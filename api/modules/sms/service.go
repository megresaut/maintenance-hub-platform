package sms

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"maintenancehub/comms"
	"maintenancehub/modules/ai"
	"maintenancehub/modules/ai/drafts"
	aiservice "maintenancehub/modules/ai/service"
)

// classificationGap is how long a conversation sits quiet before its
// unprocessed messages get classified (configurable for demos; the source
// hardcoded 5 minutes). classificationThreshold triggers immediately.
var (
	classificationGap       = envDuration("SMS_CLASSIFY_GAP_SECONDS", 90*time.Second)
	classificationThreshold = 5
)

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return fallback
}

// VendorReplyRouter lets the vendor-outreach module intercept inbound SMS
// from vendors before tenant-intake classification runs. Returns true if the
// message was consumed as an outreach reply.
type VendorReplyRouter interface {
	TryRouteVendorReply(ctx context.Context, orgID int64, fromPhone, body string, mediaURLs []string) (bool, error)
}

// Service is the SMS intake pipeline. Ported from ra-avm: inbound webhook →
// conversation storage → shouldClassify → AI classification → drafts →
// confirmation SMS. Changes: org-scoped (the inbound To number resolves the
// org), owner-phone sender lookup replaced by property matching (no owners
// table in this product), and a vendor-reply routing hook for outreach.
type Service struct {
	repo        *Repo
	classifier  *aiservice.AIClassifier
	draftSvc    *drafts.Service
	twilio      *comms.TwilioClient
	db          *pgxpool.Pool
	replyRouter VendorReplyRouter
}

type Deps struct {
	Repo       *Repo
	Classifier *aiservice.AIClassifier
	DraftSvc   *drafts.Service
	Twilio     *comms.TwilioClient
	DB         *pgxpool.Pool
}

func NewService(d Deps) *Service {
	return &Service{
		repo:       d.Repo,
		classifier: d.Classifier,
		draftSvc:   d.DraftSvc,
		twilio:     d.Twilio,
		db:         d.DB,
	}
}

// SetReplyRouter wires the vendor-outreach reply interceptor (set after
// construction to avoid a module cycle).
func (s *Service) SetReplyRouter(rr VendorReplyRouter) {
	s.replyRouter = rr
}

// ResolveOrgByNumber maps an inbound To number to the owning org.
func (s *Service) ResolveOrgByNumber(ctx context.Context, to string) (int64, string, error) {
	var orgID int64
	var orgNumber string
	err := s.db.QueryRow(ctx, `
		SELECT id, twilio_phone_number FROM organizations
		WHERE RIGHT(regexp_replace(COALESCE(twilio_phone_number,''), '[^0-9]', '', 'g'), 10)
		    = RIGHT(regexp_replace($1, '[^0-9]', '', 'g'), 10)
		LIMIT 1`, to).Scan(&orgID, &orgNumber)
	if err != nil {
		return 0, "", fmt.Errorf("no org configured for number %s: %w", to, err)
	}
	return orgID, orgNumber, nil
}

func (s *Service) HandleInboundMessage(ctx context.Context, from, to, body, messageSid, mmsSubject string, mediaURLs []string) error {
	orgID, _, err := s.ResolveOrgByNumber(ctx, to)
	if err != nil {
		return err
	}

	// Vendor outreach replies take precedence over tenant intake: if the
	// sender is a vendor with an active outreach request, the message belongs
	// to that ticket's thread, not the classification queue.
	if s.replyRouter != nil {
		routed, err := s.replyRouter.TryRouteVendorReply(ctx, orgID, from, body, mediaURLs)
		if err != nil {
			log.Printf("[sms] vendor reply routing error: %v", err)
		}
		if routed {
			log.Printf("[sms] inbound from %s consumed as vendor outreach reply", from)
			return nil
		}
	}

	// Group conversation first, else 1:1.
	conv, err := s.repo.FindGroupByMemberPhone(ctx, orgID, from)
	if err != nil {
		log.Printf("[sms] group lookup error: %v", err)
	}
	if conv == nil {
		conv, err = s.repo.UpsertConversation(ctx, orgID, from, mmsSubject)
		if err != nil {
			return fmt.Errorf("upsert conversation: %w", err)
		}
	}

	mediaJSON, _ := json.Marshal(mediaURLs)
	msg := &Message{
		ConversationID: conv.ID,
		TwilioSID:      &messageSid,
		Direction:      "inbound",
		FromNumber:     from,
		ToNumber:       to,
		Body:           body,
		MediaURLs:      mediaJSON,
	}
	if _, err := s.repo.InsertMessage(ctx, msg); err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	if err := s.repo.UpdateLastMessageAt(ctx, conv.ID); err != nil {
		log.Printf("[sms] failed to update last_message_at: %v", err)
	}

	if s.shouldClassify(ctx, conv) {
		go s.classifyConversation(context.Background(), conv, from)
	}
	return nil
}

func (s *Service) shouldClassify(ctx context.Context, conv *Conversation) bool {
	messages, err := s.repo.GetUnprocessedMessages(ctx, conv.ID)
	if err != nil {
		return false
	}
	if len(messages) >= classificationThreshold {
		return true
	}
	if len(messages) > 0 && conv.LastClassifiedAt != nil {
		if time.Since(*conv.LastClassifiedAt) > classificationGap {
			return true
		}
	}
	if len(messages) > 0 && conv.LastClassifiedAt == nil {
		if time.Since(messages[0].CreatedAt) > classificationGap {
			return true
		}
	}
	return false
}

func (s *Service) classifyConversation(ctx context.Context, conv *Conversation, from string) {
	messages, err := s.repo.GetUnprocessedMessages(ctx, conv.ID)
	if err != nil {
		log.Printf("[sms] failed to get unprocessed messages: %v", err)
		return
	}
	if len(messages) == 0 {
		return
	}

	sender := s.resolveSender(ctx, conv.OrgID, conv.GroupName)

	var aiMessages []aiservice.SMSMessage
	var mediaURLs []string
	for _, m := range messages {
		aiMessages = append(aiMessages, aiservice.SMSMessage{
			Direction: m.Direction,
			From:      m.FromNumber,
			Body:      m.Body,
			Timestamp: m.CreatedAt.Format(time.RFC3339),
		})
		var urls []string
		if err := json.Unmarshal(m.MediaURLs, &urls); err == nil {
			mediaURLs = append(mediaURLs, urls...)
		}
	}

	result, rawResponse, err := s.classifier.ClassifySMS(ctx, conv.OrgID, aiMessages, sender)
	if err != nil {
		log.Printf("[sms] classification failed: %v", err)
		return
	}

	log.Printf("[sms] classified conversation %d (org %d): type=%s confidence=%.2f",
		conv.ID, conv.OrgID, result.EventType, result.Confidence)

	if err := s.repo.MarkMessagesProcessed(ctx, conv.ID); err != nil {
		log.Printf("[sms] failed to mark messages processed: %v", err)
	}
	if err := s.repo.UpdateLastClassifiedAt(ctx, conv.ID); err != nil {
		log.Printf("[sms] failed to update last_classified_at: %v", err)
	}

	switch result.EventType {
	case "work_order", "fto_with_proposed_wo", "fto_only":
		s.createDraftsFromClassification(ctx, conv.OrgID, from, result, rawResponse, mediaURLs)
	}
}

// resolveSender matches the group subject / MMS subject against property
// names. (The source also looked the phone up in an owners table — this
// product has no owners module, so that phase is dropped.)
func (s *Service) resolveSender(ctx context.Context, orgID int64, groupName *string) *aiservice.SenderContext {
	if s.db == nil || groupName == nil {
		return nil
	}
	subject := strings.TrimSpace(*groupName)
	if subject == "" {
		return nil
	}
	var propertyID int64
	err := s.db.QueryRow(ctx, `
		SELECT id FROM properties
		WHERE org_id = $1 AND (LOWER(name) ILIKE $2 OR LOWER(address) ILIKE $2)
		LIMIT 1`, orgID, "%"+strings.ToLower(subject)+"%").Scan(&propertyID)
	if err == nil {
		return &aiservice.SenderContext{
			PropertyIDs: []int64{propertyID},
			MatchSource: "group_subject",
		}
	}
	return nil
}

func (s *Service) createDraftsFromClassification(ctx context.Context, orgID int64, replyTo string, result *ai.AIClassificationResult, rawResponse string, mediaURLs []string) {
	sourceRef := replyTo
	model := os.Getenv("AI_MODEL")
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}

	rawJSON, _ := json.Marshal(map[string]interface{}{
		"raw_response": rawResponse,
		"parsed":       result,
	})

	draftParams := func(entityType string, data any, parentID *int64) drafts.CreateDraftParams {
		b, _ := json.Marshal(data)
		p := drafts.CreateDraftParams{
			OrgID:         orgID,
			Source:        "sms",
			SourceRef:     &sourceRef,
			EntityType:    entityType,
			ExtractedData: b,
			AIConfidence:  &result.Confidence,
			AIReasoning:   &result.Reasoning,
			AIModel:       &model,
			RawAIResponse: rawJSON,
		}
		if parentID != nil {
			p.ParentDraftID = parentID
		}
		return p
	}

	taskDraft, err := s.draftSvc.CreateDraft(ctx, draftParams("task", map[string]any{
		"property_id": result.PropertyID,
		"name":        result.TaskName,
		"description": result.Description,
		"priority":    result.Priority,
		"category":    result.Category,
		"status":      "open",
		"media_urls":  mediaURLs,
	}, nil))
	if err != nil {
		log.Printf("[sms] failed to create task draft: %v", err)
		return
	}

	ftoName := result.FTOName
	if ftoName == "" {
		ftoName = result.TaskName
	}

	woData := map[string]any{
		"vendor_id":        result.VendorID,
		"property_id":      result.PropertyID,
		"name":             result.TaskName,
		"work_description": result.Description,
		"priority":         result.Priority,
		"category":         result.Category,
		"media_urls":       mediaURLs,
	}
	ftoData := map[string]any{
		"name":                  ftoName,
		"description":           result.Description,
		"property_id":           result.PropertyID,
		"field_team_member_ids": result.FieldTeamMemberIDs,
		"priority":              result.Priority,
	}

	confirm := ""
	switch result.EventType {
	case "work_order":
		if _, err := s.draftSvc.CreateDraft(ctx, draftParams("work_order", woData, &taskDraft.ID)); err != nil {
			log.Printf("[sms] failed to create work_order draft: %v", err)
		}
		if _, err := s.draftSvc.CreateDraft(ctx, draftParams("fto", ftoData, &taskDraft.ID)); err != nil {
			log.Printf("[sms] failed to create fto draft: %v", err)
		}
		confirm = fmt.Sprintf("Maintenance detected: %s. Draft work order created for review.", result.TaskName)

	case "fto_with_proposed_wo":
		if _, err := s.draftSvc.CreateDraft(ctx, draftParams("fto", ftoData, &taskDraft.ID)); err != nil {
			log.Printf("[sms] failed to create fto draft: %v", err)
		}
		woData["vendor_id"] = nil
		if _, err := s.draftSvc.CreateDraft(ctx, draftParams("work_order", woData, &taskDraft.ID)); err != nil {
			log.Printf("[sms] failed to create proposed work_order draft: %v", err)
		}
		confirm = fmt.Sprintf("Maintenance detected: %s. Draft created — vendor selection needed.", result.TaskName)

	case "fto_only":
		if _, err := s.draftSvc.CreateDraft(ctx, draftParams("fto", ftoData, &taskDraft.ID)); err != nil {
			log.Printf("[sms] failed to create fto draft: %v", err)
		}
		confirm = fmt.Sprintf("Field task detected: %s. Draft created for review.", result.TaskName)
	}

	if confirm != "" && s.twilio.Configured() && !strings.HasPrefix(replyTo, "group:") {
		fromNumber := s.orgNumber(ctx, orgID)
		if fromNumber != "" {
			if _, err := s.twilio.SendSMS(ctx, fromNumber, replyTo, confirm); err != nil {
				log.Printf("[sms] confirmation SMS failed: %v", err)
			}
		}
	}
}

func (s *Service) orgNumber(ctx context.Context, orgID int64) string {
	var n *string
	if err := s.db.QueryRow(ctx,
		`SELECT twilio_phone_number FROM organizations WHERE id = $1`, orgID).Scan(&n); err != nil || n == nil {
		return ""
	}
	return *n
}

// SendSMS sends an outbound SMS from the org's number and records it in the
// conversation history.
func (s *Service) SendSMS(ctx context.Context, orgID int64, to, body string) error {
	fromNumber := s.orgNumber(ctx, orgID)
	if fromNumber == "" {
		return fmt.Errorf("org %d has no twilio_phone_number configured", orgID)
	}
	sid, err := s.twilio.SendSMS(ctx, fromNumber, to, body)
	if err != nil {
		return err
	}
	conv, err := s.repo.UpsertConversation(ctx, orgID, to, "")
	if err == nil {
		_, _ = s.repo.InsertMessage(ctx, &Message{
			ConversationID: conv.ID,
			TwilioSID:      &sid,
			Direction:      "outbound",
			FromNumber:     fromNumber,
			ToNumber:       to,
			Body:           body,
			Processed:      true,
		})
		_ = s.repo.UpdateLastMessageAt(ctx, conv.ID)
	}
	return nil
}

// StartPoller classifies conversations whose unprocessed messages have aged
// past the classification gap.
func (s *Service) StartPoller(ctx context.Context) {
	pollInterval := envDuration("SMS_POLL_INTERVAL_SECONDS", 30*time.Second)
	go func() {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		log.Printf("[sms] background poller started (interval=%s, gap=%s)", pollInterval, classificationGap)
		for {
			select {
			case <-ctx.Done():
				log.Printf("[sms] background poller stopped")
				return
			case <-ticker.C:
				s.pollStaleConversations(ctx)
			}
		}
	}()
}

func (s *Service) pollStaleConversations(ctx context.Context) {
	cutoff := time.Now().Add(-classificationGap)
	convs, err := s.repo.GetStaleConversations(ctx, cutoff)
	if err != nil {
		log.Printf("[sms] poller: failed to query stale conversations: %v", err)
		return
	}
	if len(convs) == 0 {
		return
	}
	log.Printf("[sms] poller: found %d stale conversation(s), classifying...", len(convs))
	for _, conv := range convs {
		go s.classifyConversation(context.Background(), conv, conv.PhoneNumber)
	}
}
