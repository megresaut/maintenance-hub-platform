package outreach

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"maintenancehub/comms"
	aiclient "maintenancehub/modules/ai/client"
	"maintenancehub/modules/activity"
	"maintenancehub/modules/vendors"
)

// Service implements vendor outreach: shortlist vendors for a work order,
// send each a quote/availability request by SMS and/or email, capture
// inbound replies back onto the ticket thread, and dispatch.
type Service struct {
	db      *pgxpool.Pool
	repo    *Repo
	vendors *vendors.Repo
	twilio  *comms.TwilioClient
	smtp    *comms.SMTPClient
	ai      *aiclient.Client
	rec     *activity.Recorder
}

func NewService(db *pgxpool.Pool, repo *Repo, vrepo *vendors.Repo, twilio *comms.TwilioClient, smtp *comms.SMTPClient, ai *aiclient.Client, rec *activity.Recorder) *Service {
	return &Service{db: db, repo: repo, vendors: vrepo, twilio: twilio, smtp: smtp, ai: ai, rec: rec}
}

// workOrderInfo is the slice of the work_orders row outreach needs.
type workOrderInfo struct {
	ID           int64
	Name         string
	Description  string
	PropertyID   *int64
	PropertyName string
	PropertyAddr string
	Category     string
	Status       string
	MediaURLs    []string
}

func (s *Service) loadWorkOrder(ctx context.Context, orgID, id int64) (*workOrderInfo, error) {
	var wo workOrderInfo
	var media []byte
	err := s.db.QueryRow(ctx, `
		SELECT w.id, w.name, COALESCE(w.work_description, ''), w.property_id,
		       COALESCE(p.name, ''), COALESCE(p.address, ''),
		       COALESCE(w.category, ''), w.status, COALESCE(w.media_urls, '[]'::jsonb)
		FROM work_orders w
		LEFT JOIN properties p ON p.id = w.property_id
		WHERE w.id = $1 AND w.org_id = $2`, id, orgID,
	).Scan(&wo.ID, &wo.Name, &wo.Description, &wo.PropertyID, &wo.PropertyName,
		&wo.PropertyAddr, &wo.Category, &wo.Status, &media)
	if err != nil {
		return nil, fmt.Errorf("work order %d not found: %w", id, err)
	}
	_ = json.Unmarshal(media, &wo.MediaURLs)
	return &wo, nil
}

type orgInfo struct {
	Name         string
	TwilioNumber string
	EmailFrom    string
}

func (s *Service) loadOrg(ctx context.Context, orgID int64) (*orgInfo, error) {
	var o orgInfo
	var num, email *string
	err := s.db.QueryRow(ctx, `
		SELECT name, twilio_phone_number, outreach_email_from
		FROM organizations WHERE id = $1`, orgID).Scan(&o.Name, &num, &email)
	if err != nil {
		return nil, err
	}
	if num != nil {
		o.TwilioNumber = *num
	}
	if email != nil {
		o.EmailFrom = *email
	}
	return &o, nil
}

// Shortlist returns the vendors outreach would contact for a work order:
// the property+trade preferred list first, else the local directory lookup
// by trade + service-area overlap with the property address.
func (s *Service) Shortlist(ctx context.Context, orgID, workOrderID int64, category string, max int) ([]*vendors.Vendor, string, error) {
	wo, err := s.loadWorkOrder(ctx, orgID, workOrderID)
	if err != nil {
		return nil, "", err
	}
	if category == "" {
		category = wo.Category
	}
	if category == "" {
		return nil, "", fmt.Errorf("work order has no trade category — pass one explicitly")
	}
	if max <= 0 {
		max = 3
	}

	source := "preferred_list"
	var list []*vendors.Vendor
	if wo.PropertyID != nil {
		list, err = s.vendors.ListPreferred(ctx, orgID, *wo.PropertyID, category)
		if err != nil {
			return nil, "", err
		}
	}
	if len(list) == 0 {
		// Fallback: local directory lookup by trade; vendors whose
		// service-area tags appear in the property address rank first.
		source = "local_lookup"
		list, err = s.vendors.LookupLocal(ctx, orgID, category, nil)
		if err != nil {
			return nil, "", err
		}
		addr := strings.ToLower(wo.PropertyAddr + " " + wo.PropertyName)
		var matched, rest []*vendors.Vendor
		for _, v := range list {
			hit := false
			for _, tag := range v.ServiceAreaTags {
				if tag != "" && strings.Contains(addr, strings.ToLower(tag)) {
					hit = true
					break
				}
			}
			if hit {
				matched = append(matched, v)
			} else {
				rest = append(rest, v)
			}
		}
		list = append(matched, rest...)
	}

	if len(list) > max {
		list = list[:max]
	}
	return list, source, nil
}

// Trigger shortlists and sends outreach messages for a work order. Returns
// the created request records.
func (s *Service) Trigger(ctx context.Context, orgID, workOrderID, actorID int64, in TriggerRequest) ([]*Request, error) {
	wo, err := s.loadWorkOrder(ctx, orgID, workOrderID)
	if err != nil {
		return nil, err
	}
	org, err := s.loadOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}

	var shortlist []*vendors.Vendor
	var source string
	if len(in.VendorIDs) > 0 {
		source = "manual_selection"
		for _, vid := range in.VendorIDs {
			v, err := s.vendors.GetByID(ctx, orgID, vid)
			if err != nil || v == nil {
				return nil, fmt.Errorf("vendor %d not found", vid)
			}
			shortlist = append(shortlist, v)
		}
	} else {
		shortlist, source, err = s.Shortlist(ctx, orgID, workOrderID, in.Category, in.MaxVendors)
		if err != nil {
			return nil, err
		}
	}
	if len(shortlist) == 0 {
		return nil, fmt.Errorf("no vendors found for trade %q — add vendors or a preferred list first", in.Category)
	}

	wantSMS, wantEmail := channelPrefs(in.Channels)

	var out []*Request
	for _, v := range shortlist {
		sent := false
		if wantSMS && v.Phone != nil && *v.Phone != "" && org.TwilioNumber != "" {
			req, err := s.sendOne(ctx, orgID, org, wo, v, "sms", *v.Phone, in.Note)
			if err != nil {
				return nil, err
			}
			out = append(out, req)
			sent = req.Status == StatusSent
		}
		if wantEmail && v.PrimaryEmail != nil && *v.PrimaryEmail != "" && org.EmailFrom != "" {
			// Email complements SMS only when SMS wasn't possible, unless
			// email was explicitly requested.
			if !sent || contains(in.Channels, "email") {
				req, err := s.sendOne(ctx, orgID, org, wo, v, "email", *v.PrimaryEmail, in.Note)
				if err != nil {
					return nil, err
				}
				out = append(out, req)
			}
		}
		if (v.Phone == nil || *v.Phone == "") && (v.PrimaryEmail == nil || *v.PrimaryEmail == "") {
			log.Printf("[outreach] vendor %d (%s) has no phone or email — skipped", v.ID, v.Name)
		}
	}

	if len(out) > 0 {
		// Move the work order to 'sent' (sourcing) once outreach is out the door.
		_, err := s.db.Exec(ctx, `
			UPDATE work_orders SET status = 'sent'
			WHERE id = $1 AND org_id = $2 AND status = 'new'`,
			workOrderID, orgID)
		if err != nil {
			log.Printf("[outreach] failed to set work order %d to sent: %v", workOrderID, err)
		}

		names := make([]string, 0, len(out))
		for _, r := range out {
			if r.Status == StatusSent {
				names = append(names, fmt.Sprintf("%s via %s", vendorName(shortlist, r.VendorID), r.Channel))
			}
		}
		meta, _ := json.Marshal(map[string]any{"source": source, "count": len(names)})
		s.rec.Record(ctx, activity.Entry{
			OrgID: orgID, TicketType: "work_order", TicketID: workOrderID,
			Kind: activity.KindOutreachSent, ActorType: "user", ActorID: &actorID,
			Body:     fmt.Sprintf("Outreach sent to %d vendor(s) (%s): %s", len(names), source, strings.Join(names, ", ")),
			Metadata: meta,
		})
	}

	return out, nil
}

// sendOne composes, persists, and sends a single outreach message. The
// request row is recorded first (status pending) and then marked sent or
// failed, so the audit trail survives provider errors.
func (s *Service) sendOne(ctx context.Context, orgID int64, org *orgInfo, wo *workOrderInfo, v *vendors.Vendor, channel, to, note string) (*Request, error) {
	body := composeMessage(org, wo, v, note)
	req := &Request{
		OrgID: orgID, WorkOrderID: wo.ID, VendorID: v.ID,
		Channel: channel, ToAddress: to, MessageBody: body, Status: StatusPending,
		VendorName: v.Name,
	}
	if _, err := s.repo.CreateRequest(ctx, req); err != nil {
		return nil, err
	}

	// OUTREACH_SIMULATE=1: dev flag for demos without messaging credentials —
	// the request is recorded and marked sent, but no provider is called.
	// Real Twilio/SMTP sending is the default path.
	if os.Getenv("OUTREACH_SIMULATE") == "1" {
		req.Status = StatusSent
		if err := s.repo.MarkSent(ctx, req.ID, "simulated"); err != nil {
			log.Printf("[outreach] failed to mark request %d sent: %v", req.ID, err)
		}
		log.Printf("[outreach] SIMULATED %s to vendor %d (%s) at %s for WO-%d", channel, v.ID, v.Name, to, wo.ID)
		return req, nil
	}

	var providerRef string
	var sendErr error
	switch channel {
	case "sms":
		if !s.twilio.Configured() {
			sendErr = fmt.Errorf("twilio not configured")
		} else {
			providerRef, sendErr = s.twilio.SendSMS(ctx, org.TwilioNumber, to, body)
		}
	case "email":
		if !s.smtp.Configured() {
			sendErr = fmt.Errorf("smtp not configured")
		} else {
			subject := fmt.Sprintf("Quote request: %s (Ref WO-%d)", wo.Name, wo.ID)
			sendErr = s.smtp.SendEmail(ctx, org.EmailFrom, to, subject, body)
		}
	default:
		sendErr = fmt.Errorf("unknown channel %q", channel)
	}

	if sendErr != nil {
		req.Status = StatusFailed
		msg := sendErr.Error()
		req.Error = &msg
		if err := s.repo.MarkFailed(ctx, req.ID, msg); err != nil {
			log.Printf("[outreach] failed to mark request %d failed: %v", req.ID, err)
		}
		log.Printf("[outreach] send failed (request %d, %s to %s): %v", req.ID, channel, to, sendErr)
		return req, nil
	}

	req.Status = StatusSent
	if err := s.repo.MarkSent(ctx, req.ID, providerRef); err != nil {
		log.Printf("[outreach] failed to mark request %d sent: %v", req.ID, err)
	}
	log.Printf("[outreach] sent %s to vendor %d (%s) at %s for WO-%d", channel, v.ID, v.Name, to, wo.ID)
	return req, nil
}

func vendorName(list []*vendors.Vendor, id int64) string {
	for _, v := range list {
		if v.ID == id {
			return v.Name
		}
	}
	return fmt.Sprintf("vendor %d", id)
}

func channelPrefs(channels []string) (sms, email bool) {
	if len(channels) == 0 {
		return true, true
	}
	return contains(channels, "sms"), contains(channels, "email")
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// composeMessage builds the outreach text referencing the ticket details:
// description, property, photos — asking for availability and a quote.
func composeMessage(org *orgInfo, wo *workOrderInfo, v *vendors.Vendor, note string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Hi %s, this is %s. ", v.Name, org.Name)
	trade := wo.Category
	if trade == "" {
		trade = "maintenance"
	}
	prop := wo.PropertyAddr
	if prop == "" {
		prop = wo.PropertyName
	}
	fmt.Fprintf(&b, "We need %s work at %s.\n\n", strings.ReplaceAll(trade, "_", " "), prop)
	fmt.Fprintf(&b, "Job: %s", wo.Name)
	if wo.Description != "" && wo.Description != wo.Name {
		fmt.Fprintf(&b, " — %s", wo.Description)
	}
	b.WriteString("\n")
	if note != "" {
		fmt.Fprintf(&b, "Note: %s\n", note)
	}
	if len(wo.MediaURLs) > 0 {
		b.WriteString("Photos: " + strings.Join(wo.MediaURLs, " ") + "\n")
	}
	fmt.Fprintf(&b, "\nCan you take this job? Please reply with your availability and a quote. Reference: WO-%d", wo.ID)
	return b.String()
}

var woRefRe = regexp.MustCompile(`(?i)\bWO-(\d+)\b`)

// TryRouteVendorReply implements sms.VendorReplyRouter: consume an inbound
// SMS as an outreach reply when the sender matches an active request.
func (s *Service) TryRouteVendorReply(ctx context.Context, orgID int64, fromPhone, body string, mediaURLs []string) (bool, error) {
	// Prefer an explicit WO-<id> reference in the body, then vendor phone.
	var req *Request
	if m := woRefRe.FindStringSubmatch(body); m != nil {
		if woID, err := strconv.ParseInt(m[1], 10, 64); err == nil {
			r, err := s.repo.FindOpenRequestByWORef(ctx, orgID, woID, fromPhone)
			if err != nil {
				return false, err
			}
			req = r
		}
	}
	if req == nil {
		r, err := s.repo.FindOpenRequestByVendorPhone(ctx, orgID, fromPhone)
		if err != nil {
			return false, err
		}
		req = r
	}
	if req == nil {
		return false, nil
	}
	return true, s.recordReply(ctx, req, body, mediaURLs, "sms:"+fromPhone)
}

// RouteEmailReply is the email-channel entry point (inbound-parse webhook).
func (s *Service) RouteEmailReply(ctx context.Context, orgID int64, fromEmail, subject, body string) (bool, error) {
	text := subject + "\n" + body
	var req *Request
	if m := woRefRe.FindStringSubmatch(text); m != nil {
		if woID, err := strconv.ParseInt(m[1], 10, 64); err == nil {
			r, err := s.repo.FindOpenRequestByWORef(ctx, orgID, woID, fromEmail)
			if err != nil {
				return false, err
			}
			req = r
		}
	}
	if req == nil {
		r, err := s.repo.FindOpenRequestByVendorEmail(ctx, orgID, fromEmail)
		if err != nil {
			return false, err
		}
		req = r
	}
	if req == nil {
		return false, nil
	}
	return true, s.recordReply(ctx, req, strings.TrimSpace(body), nil, "email:"+fromEmail)
}

func (s *Service) recordReply(ctx context.Context, req *Request, body string, mediaURLs []string, sourceRef string) error {
	media, _ := json.Marshal(mediaURLs)
	rep := &Reply{
		OutreachRequestID: req.ID,
		Body:              body,
		MediaURLs:         media,
		RawSourceRef:      &sourceRef,
	}

	// Parse quote/availability with AI — best-effort, never blocks capture.
	if s.ai != nil {
		if cents, avail, err := s.parseQuote(ctx, body); err == nil {
			rep.ParsedQuoteCents = cents
			rep.ParsedAvailability = avail
		} else {
			log.Printf("[outreach] quote parse failed for request %d: %v", req.ID, err)
		}
	}

	if _, err := s.repo.InsertReply(ctx, rep); err != nil {
		return err
	}
	if err := s.repo.SetStatus(ctx, req.OrgID, req.ID, StatusReplied); err != nil {
		log.Printf("[outreach] failed to mark request %d replied: %v", req.ID, err)
	}

	var vendorName string
	_ = s.db.QueryRow(ctx, `SELECT name FROM vendors WHERE id = $1`, req.VendorID).Scan(&vendorName)

	quoteStr := ""
	if rep.ParsedQuoteCents != nil {
		quoteStr = fmt.Sprintf(" — quoted $%.2f", float64(*rep.ParsedQuoteCents)/100)
	}
	meta, _ := json.Marshal(map[string]any{
		"outreach_request_id": req.ID,
		"reply_id":            rep.ID,
		"vendor_id":           req.VendorID,
		"parsed_quote_cents":  rep.ParsedQuoteCents,
	})
	s.rec.Record(ctx, activity.Entry{
		OrgID: req.OrgID, TicketType: "work_order", TicketID: req.WorkOrderID,
		Kind: activity.KindOutreachReply, ActorType: "vendor", ActorName: vendorName,
		Body:     fmt.Sprintf("%s replied%s: %s", vendorName, quoteStr, body),
		Metadata: meta,
	})
	return nil
}

const quoteParsePrompt = `You extract structured data from a vendor's reply to a maintenance quote request.

Respond with ONLY a JSON object (no markdown):
{
  "quote_cents": <integer amount in cents, or null if no price given>,
  "availability": "<short summary of when they can do the work, or null>",
  "declined": <true if the vendor is declining the job, else false>
}

Rules:
- Convert dollars to cents ($450 = 45000, "$1,200.50" = 120050).
- If a price range is given, use the LOW end.
- availability examples: "tomorrow 8am", "Thursday afternoon", "next week". Keep it under 10 words.
- Do not invent values.`

func (s *Service) parseQuote(ctx context.Context, body string) (*int64, *string, error) {
	raw, err := s.ai.Ask(ctx, quoteParsePrompt, "Vendor reply:\n"+body)
	if err != nil {
		return nil, nil, err
	}
	var parsed struct {
		QuoteCents   *int64  `json:"quote_cents"`
		Availability *string `json:"availability"`
		Declined     bool    `json:"declined"`
	}
	if err := json.Unmarshal([]byte(aiclient.ExtractJSON(raw)), &parsed); err != nil {
		return nil, nil, fmt.Errorf("parse quote JSON: %w (raw: %s)", err, raw)
	}
	return parsed.QuoteCents, parsed.Availability, nil
}

// Dispatch selects a vendor (and optionally the reply that won) and moves
// the work order to dispatched.
func (s *Service) Dispatch(ctx context.Context, orgID, workOrderID, actorID int64, in DispatchRequest) error {
	if in.VendorID == 0 {
		return fmt.Errorf("vendor_id is required")
	}
	v, err := s.vendors.GetByID(ctx, orgID, in.VendorID)
	if err != nil || v == nil {
		return fmt.Errorf("vendor %d not found", in.VendorID)
	}

	quote := in.QuoteCents
	if quote == nil && in.ReplyID != nil {
		rep, _, err := s.repo.GetReply(ctx, orgID, *in.ReplyID)
		if err == nil && rep != nil {
			quote = rep.ParsedQuoteCents
		}
	}

	ct, err := s.db.Exec(ctx, `
		UPDATE work_orders
		SET vendor_id = $1, status = 'dispatched', dispatched_at = now(), quote_amount_cents = $2
		WHERE id = $3 AND org_id = $4`, in.VendorID, quote, workOrderID, orgID)
	if err != nil {
		return fmt.Errorf("dispatch work order: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("work order %d not found", workOrderID)
	}

	// Mark the winning request selected.
	_, _ = s.db.Exec(ctx, `
		UPDATE vendor_outreach_requests SET status = 'selected'
		WHERE org_id = $1 AND work_order_id = $2 AND vendor_id = $3 AND status IN ('sent','replied')`,
		orgID, workOrderID, in.VendorID)

	quoteStr := ""
	if quote != nil {
		quoteStr = fmt.Sprintf(" at $%.2f", float64(*quote)/100)
	}
	meta, _ := json.Marshal(map[string]any{"vendor_id": in.VendorID, "quote_cents": quote, "reply_id": in.ReplyID})
	s.rec.Record(ctx, activity.Entry{
		OrgID: orgID, TicketType: "work_order", TicketID: workOrderID,
		Kind: activity.KindDispatched, ActorType: "user", ActorID: &actorID,
		Body:     fmt.Sprintf("Dispatched to %s%s. %s", v.Name, quoteStr, in.Note),
		Metadata: meta,
	})

	// Confirmation to the winning vendor.
	if in.NotifyVendor && v.Phone != nil && *v.Phone != "" {
		org, err := s.loadOrg(ctx, orgID)
		if err == nil && org.TwilioNumber != "" && s.twilio.Configured() {
			wo, err := s.loadWorkOrder(ctx, orgID, workOrderID)
			if err == nil {
				msg := fmt.Sprintf("Hi %s, %s here — you've got the job (Ref WO-%d: %s). We'll follow up to coordinate access. Thank you!",
					v.Name, org.Name, wo.ID, wo.Name)
				if _, err := s.twilio.SendSMS(ctx, org.TwilioNumber, *v.Phone, msg); err != nil {
					log.Printf("[outreach] dispatch notification to vendor failed: %v", err)
				}
			}
		}
	}
	return nil
}
