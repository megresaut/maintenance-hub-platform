package aiservice

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"maintenancehub/modules/ai"
	aiclient "maintenancehub/modules/ai/client"
)

// AIClassifier classifies inbound requests (SMS threads, calendar events)
// using Claude. Ported from ra-avm; org_id threaded through, the rule-based
// calendar fallback and feedback-injection subsystems were not ported.
type AIClassifier struct {
	client        *aiclient.Client
	contextLoader *ContextLoader

	// Rate limiting: minimum 500ms between API calls.
	rateMu   sync.Mutex
	lastCall time.Time
}

func NewAIClassifier(client *aiclient.Client, contextLoader *ContextLoader) *AIClassifier {
	return &AIClassifier{client: client, contextLoader: contextLoader}
}

func (c *AIClassifier) rateLimit() {
	c.rateMu.Lock()
	if since := time.Since(c.lastCall); since < 500*time.Millisecond {
		time.Sleep(500*time.Millisecond - since)
	}
	c.lastCall = time.Now()
	c.rateMu.Unlock()
}

// CalendarEventInput is the module-neutral shape of a calendar event to
// classify (the calendar module maps Graph events into this).
type CalendarEventInput struct {
	Subject     string
	BodyPreview string
	Location    string
	Attendees   []string // "Name <email>"
	Start       string
	End         string
}

// ClassifyCalendarEvent classifies one calendar event for an org.
func (c *AIClassifier) ClassifyCalendarEvent(ctx context.Context, orgID int64, ev CalendarEventInput) (*ai.AIClassificationResult, string, error) {
	c.rateLimit()

	fullCtx, err := c.contextLoader.Load(ctx, orgID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load context: %w", err)
	}

	eventText := strings.Join(append([]string{ev.Subject, ev.BodyPreview, ev.Location}, ev.Attendees...), " ")
	filteredCtx := c.contextLoader.FilterForEvent(fullCtx, eventText)

	log.Printf("[ai-classifier] org=%d event=%q vendors_filtered=%d/%d properties=%d",
		orgID, ev.Subject, len(filteredCtx.Vendors), len(fullCtx.Vendors), len(filteredCtx.Properties))

	systemPrompt, err := RenderCalendarPrompt(filteredCtx)
	if err != nil {
		return nil, "", fmt.Errorf("failed to render prompt: %w", err)
	}
	userPrompt := FormatCalendarEvent(ev.Subject, ev.BodyPreview, ev.Location, ev.Attendees, ev.Start, ev.End)

	result, raw, err := c.client.Classify(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, raw, err
	}
	log.Printf("[ai-classifier] org=%d event=%q type=%s confidence=%.2f reasoning=%q",
		orgID, ev.Subject, result.EventType, result.Confidence, result.Reasoning)
	return result, raw, nil
}

// SenderContext holds resolved identity about an SMS sender.
type SenderContext struct {
	SenderName  string  // display name if known
	PropertyIDs []int64 // property IDs associated with this sender
	MatchSource string  // "known_contact" | "group_subject" | "none"
}

// ClassifySMS classifies an SMS conversation thread for an org.
func (c *AIClassifier) ClassifySMS(ctx context.Context, orgID int64, messages []SMSMessage, sender *SenderContext) (*ai.AIClassificationResult, string, error) {
	c.rateLimit()

	fullCtx, err := c.contextLoader.Load(ctx, orgID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load context: %w", err)
	}

	var filteredCtx *ai.ContextData
	if sender != nil && len(sender.PropertyIDs) > 0 {
		filteredCtx = c.contextLoader.FilterToProperties(fullCtx, sender.PropertyIDs)
	} else {
		var parts []string
		for _, m := range messages {
			parts = append(parts, m.Body)
		}
		filteredCtx = c.contextLoader.FilterForEvent(fullCtx, strings.Join(parts, " "))
	}

	systemPrompt, err := RenderSMSPrompt(filteredCtx, sender)
	if err != nil {
		return nil, "", fmt.Errorf("failed to render SMS prompt: %w", err)
	}
	userPrompt := FormatSMSThread(messages)

	return c.client.ClassifySMS(ctx, systemPrompt, userPrompt)
}
