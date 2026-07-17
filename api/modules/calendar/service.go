package calendar

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"maintenancehub/modules/calendar/outlook"
)

// syncPaused returns true when CALENDAR_SYNC_PAUSED is set to true/1.
// When paused, no Outlook API calls or webhook processing run (the webhook
// endpoint still returns 200 so Graph does not retry forever).
func syncPaused() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("CALENDAR_SYNC_PAUSED")))
	return v == "true" || v == "1"
}

// cachedClient pairs a Graph client with the connection fingerprint it was
// built from, so a changed org connection invalidates the cached client
// (and with it the cached per-org access token).
type cachedClient struct {
	fingerprint string
	client      *outlook.GraphCalendarClient
}

// Service implements the Outlook bidirectional calendar sync, org-scoped.
type Service struct {
	repo       *Repo
	drafts     DraftCreator // nil = classification disabled
	webhookURL string       // public URL Graph posts notifications to

	mu      sync.Mutex
	clients map[int64]*cachedClient // orgID -> Graph client (per-org token cache)

	inFlight sync.Map // "orgID:outlookEventID" currently being processed
}

// NewService constructs the calendar sync service.
//   - pool: shared pgx pool
//   - webhookURL: the public notification URL registered with Graph
//     (e.g. https://api.example.com/api/calendar/webhook)
//   - drafts: optional AI classification handoff; nil disables classification
//     (inbound events are still mirrored into calendar_events).
func NewService(pool *pgxpool.Pool, webhookURL string, drafts DraftCreator) *Service {
	return &Service{
		repo:       NewRepo(pool),
		drafts:     drafts,
		webhookURL: webhookURL,
		clients:    make(map[int64]*cachedClient),
	}
}

// ================================================================
// PER-ORG CONNECTION + GRAPH CLIENT CACHE
// ================================================================

func connectionFingerprint(c *Connection) string {
	return strings.Join([]string{c.TenantID, c.ClientID, c.ClientSecret, c.MailboxUPN, c.CalendarID}, "\x00")
}

// graphFor returns a Graph client for the org's connection, building and
// caching one (with its in-memory token) as needed.
func (s *Service) graphFor(ctx context.Context, orgID int64) (*outlook.GraphCalendarClient, *Connection, error) {
	conn, err := s.repo.GetConnection(ctx, orgID)
	if err != nil {
		return nil, nil, fmt.Errorf("load org calendar connection: %w", err)
	}
	if conn == nil {
		return nil, nil, fmt.Errorf("org %d has no Outlook calendar connection configured", orgID)
	}
	if !conn.Enabled {
		return nil, nil, fmt.Errorf("org %d calendar connection is disabled", orgID)
	}

	fp := connectionFingerprint(conn)
	s.mu.Lock()
	defer s.mu.Unlock()
	if cc, ok := s.clients[orgID]; ok && cc.fingerprint == fp {
		return cc.client, conn, nil
	}
	client := outlook.NewGraphCalendarClient(conn.TenantID, conn.ClientID, conn.ClientSecret, conn.MailboxUPN, conn.CalendarID)
	s.clients[orgID] = &cachedClient{fingerprint: fp, client: client}
	return client, conn, nil
}

// invalidateClient drops the cached Graph client (and token) for an org.
func (s *Service) invalidateClient(orgID int64) {
	s.mu.Lock()
	delete(s.clients, orgID)
	s.mu.Unlock()
}

// GetConnection returns the org's connection (nil if unconfigured).
func (s *Service) GetConnection(ctx context.Context, orgID int64) (*Connection, error) {
	return s.repo.GetConnection(ctx, orgID)
}

// SaveConnection upserts the org's connection and invalidates the cached client.
func (s *Service) SaveConnection(ctx context.Context, c *Connection) (*Connection, error) {
	saved, err := s.repo.UpsertConnection(ctx, c)
	if err != nil {
		return nil, err
	}
	s.invalidateClient(c.OrgID)
	return saved, nil
}

// DeleteConnection removes the org's connection and invalidates the cached client.
func (s *Service) DeleteConnection(ctx context.Context, orgID int64) (bool, error) {
	ok, err := s.repo.DeleteConnection(ctx, orgID)
	if err != nil {
		return false, err
	}
	s.invalidateClient(orgID)
	return ok, nil
}

// ListEvents exposes the org's mirrored/local events.
func (s *Service) ListEvents(ctx context.Context, orgID int64, from, to *time.Time) ([]Event, error) {
	return s.repo.ListEvents(ctx, orgID, from, to)
}

// ================================================================
// OUTLOOK → LOCAL
// ================================================================

// SyncEventFromOutlook fetches one Outlook event and mirrors it into the
// org's calendar_events table. Newly imported events are handed to the
// DraftCreator for AI classification (when configured).
func (s *Service) SyncEventFromOutlook(ctx context.Context, orgID int64, outlookEventID string) error {
	if syncPaused() {
		log.Printf("[calendar] sync paused (CALENDAR_SYNC_PAUSED); skipping SyncEventFromOutlook for org %d event %s", orgID, outlookEventID)
		return nil
	}

	// Prevent duplicate processing from concurrent webhook retries for the same event.
	key := fmt.Sprintf("%d:%s", orgID, outlookEventID)
	if _, loaded := s.inFlight.LoadOrStore(key, struct{}{}); loaded {
		log.Printf("[calendar] skipping org %d event %s — already in-flight", orgID, outlookEventID)
		return nil
	}
	defer s.inFlight.Delete(key)

	graph, _, err := s.graphFor(ctx, orgID)
	if err != nil {
		return err
	}

	ev, err := graph.GetEvent(ctx, outlookEventID)
	if err != nil {
		if outlook.IsNotFoundError(err) {
			log.Printf("[calendar] org %d: Outlook event %s no longer exists (404) — treating as deleted", orgID, outlookEventID)
			return s.handleCancelledEvent(ctx, orgID, outlookEventID)
		}
		return fmt.Errorf("fetch event %s: %w", outlookEventID, err)
	}

	if ev.IsCancelled {
		return s.handleCancelledEvent(ctx, orgID, outlookEventID)
	}

	// Loop prevention: if this notification is the echo of our own outbound
	// write (same changeKey we stored after local_to_outlook sync), skip it.
	existing, err := s.repo.GetEventByOutlookID(ctx, orgID, outlookEventID)
	if err != nil {
		return fmt.Errorf("lookup local event: %w", err)
	}
	if existing != nil && shouldSkipInbound(existing, ev.ChangeKey) {
		log.Printf("[calendar] org %d: skipping inbound update for event %d (loop prevention)", orgID, existing.ID)
		return nil
	}

	local := eventFromOutlook(orgID, ev)
	saved, inserted, err := s.repo.UpsertEventFromOutlook(ctx, local)
	if err != nil {
		return fmt.Errorf("upsert event from outlook: %w", err)
	}
	log.Printf("[calendar] org %d: mirrored Outlook event %s as calendar_event %d (inserted=%v)", orgID, ev.ID, saved.ID, inserted)

	// AI classification handoff — only on first import so a stream of update
	// notifications doesn't create duplicate drafts.
	if inserted {
		if s.drafts == nil {
			log.Printf("[calendar] org %d: classification disabled (no DraftCreator) — event %s imported only", orgID, ev.ID)
		} else if !saved.IsPrivate {
			content := buildClassificationContent(ev)
			if err := s.drafts.ClassifyAndDraft(ctx, orgID, "calendar", ev.ID, content); err != nil {
				log.Printf("[calendar] org %d: ClassifyAndDraft failed for event %s: %v", orgID, ev.ID, err)
			}
		}
	}
	return nil
}

// eventFromOutlook maps a Graph event to the local org-scoped row.
// Private/confidential events are stored as "Busy" with no description.
func eventFromOutlook(orgID int64, ev *outlook.CalendarEvent) *Event {
	isPrivate := ev.Sensitivity == "private" || ev.Sensitivity == "confidential"

	title := ev.Subject
	if isPrivate {
		title = "Busy"
	}
	description := ""
	if ev.BodyPreview != "" && !isPrivate {
		description = ev.BodyPreview
	}
	showAs := ev.ShowAs
	if showAs == "" {
		showAs = "busy"
	}

	now := time.Now().UTC()
	startAt := parseDateTimeTZ(ev.Start)
	endAt := parseDateTimeTZ(ev.End)
	if startAt == nil {
		startAt = &now
	}
	if endAt == nil {
		e := startAt.Add(time.Hour)
		endAt = &e
	}

	outlookID := ev.ID
	changeKey := ev.ChangeKey
	dir := DirectionOutlookToLocal
	return &Event{
		OrgID:             orgID,
		Title:             title,
		Description:       description,
		Location:          ev.Location.DisplayName,
		StartAt:           *startAt,
		EndAt:             *endAt,
		AllDay:            ev.IsAllDay,
		ShowAs:            showAs,
		IsPrivate:         isPrivate,
		OrganizerName:     ev.Organizer.EmailAddress.Name,
		Source:            SourceOutlook,
		OutlookEventID:    &outlookID,
		OutlookChangeKey:  &changeKey,
		LastSyncDirection: &dir,
		SyncStatus:        SyncStatusActive,
	}
}

// buildClassificationContent assembles the text handed to the DraftCreator.
func buildClassificationContent(ev *outlook.CalendarEvent) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Subject: %s\n", ev.Subject)
	if ev.Location.DisplayName != "" {
		fmt.Fprintf(&b, "Location: %s\n", ev.Location.DisplayName)
	}
	fmt.Fprintf(&b, "Start: %s (%s)\n", ev.Start.DateTime, ev.Start.TimeZone)
	fmt.Fprintf(&b, "End: %s (%s)\n", ev.End.DateTime, ev.End.TimeZone)
	if ev.Organizer.EmailAddress.Address != "" || ev.Organizer.EmailAddress.Name != "" {
		fmt.Fprintf(&b, "Organizer: %s <%s>\n", ev.Organizer.EmailAddress.Name, ev.Organizer.EmailAddress.Address)
	}
	for _, a := range ev.Attendees {
		fmt.Fprintf(&b, "Attendee: %s <%s>\n", a.EmailAddress.Name, a.EmailAddress.Address)
	}
	if ev.BodyPreview != "" {
		fmt.Fprintf(&b, "Body:\n%s\n", ev.BodyPreview)
	}
	return b.String()
}

func (s *Service) handleCancelledEvent(ctx context.Context, orgID int64, outlookEventID string) error {
	changed, err := s.repo.MarkEventCancelledByOutlookID(ctx, orgID, outlookEventID)
	if err != nil {
		return fmt.Errorf("mark event cancelled: %w", err)
	}
	if changed {
		log.Printf("[calendar] org %d: marked calendar_event for Outlook event %s cancelled", orgID, outlookEventID)
	}
	return nil
}

// ================================================================
// LOCAL → OUTLOOK
// ================================================================

// SyncEventToOutlook pushes a local calendar_events row to the org's Outlook
// calendar. Creates a new Outlook event if the row isn't linked yet; patches
// the linked event otherwise.
func (s *Service) SyncEventToOutlook(ctx context.Context, orgID, eventID int64) error {
	if syncPaused() {
		log.Printf("[calendar] sync paused (CALENDAR_SYNC_PAUSED); skipping SyncEventToOutlook for org %d event %d", orgID, eventID)
		return nil
	}

	local, err := s.repo.GetEvent(ctx, orgID, eventID)
	if err != nil {
		return fmt.Errorf("load event %d: %w", eventID, err)
	}
	if local == nil {
		return fmt.Errorf("event %d not found for org %d", eventID, orgID)
	}

	graph, _, err := s.graphFor(ctx, orgID)
	if err != nil {
		return err
	}

	if local.OutlookEventID != nil && *local.OutlookEventID != "" {
		req := buildUpdateEventRequest(local)
		ev, err := graph.UpdateEvent(ctx, *local.OutlookEventID, req)
		if err != nil {
			return fmt.Errorf("update outlook event: %w", err)
		}
		if err := s.repo.UpdateEventSyncMeta(ctx, orgID, local.ID, ev.ChangeKey, DirectionLocalToOutlook); err != nil {
			log.Printf("[calendar] org %d: failed to update sync meta for event %d: %v", orgID, local.ID, err)
		}
		log.Printf("[calendar] org %d: updated Outlook event %s from calendar_event %d", orgID, *local.OutlookEventID, local.ID)
		return nil
	}

	req := buildCreateEventRequest(local)
	ev, err := graph.CreateEvent(ctx, req)
	if err != nil {
		return fmt.Errorf("create outlook event: %w", err)
	}
	if err := s.repo.SetEventOutlookLink(ctx, orgID, local.ID, ev.ID, ev.ChangeKey); err != nil {
		return fmt.Errorf("store outlook link for event %d: %w", local.ID, err)
	}
	log.Printf("[calendar] org %d: created Outlook event %s from calendar_event %d", orgID, ev.ID, local.ID)
	return nil
}

// DeleteEventFromOutlook deletes the Outlook event linked to a local row and
// marks the row deleted. A Graph 404 (already gone) is not an error.
func (s *Service) DeleteEventFromOutlook(ctx context.Context, orgID, eventID int64) error {
	if syncPaused() {
		log.Printf("[calendar] sync paused (CALENDAR_SYNC_PAUSED); skipping DeleteEventFromOutlook for org %d event %d", orgID, eventID)
		return nil
	}

	local, err := s.repo.GetEvent(ctx, orgID, eventID)
	if err != nil {
		return fmt.Errorf("load event %d: %w", eventID, err)
	}
	if local == nil {
		return fmt.Errorf("event %d not found for org %d", eventID, orgID)
	}

	if local.OutlookEventID != nil && *local.OutlookEventID != "" {
		graph, _, err := s.graphFor(ctx, orgID)
		if err != nil {
			return err
		}
		if err := graph.DeleteEvent(ctx, *local.OutlookEventID); err != nil {
			log.Printf("[calendar] org %d: failed to delete Outlook event %s: %v", orgID, *local.OutlookEventID, err)
		}
	}
	if err := s.repo.SetEventStatus(ctx, orgID, local.ID, SyncStatusDeleted); err != nil {
		return fmt.Errorf("mark event %d deleted: %w", local.ID, err)
	}
	log.Printf("[calendar] org %d: deleted calendar_event %d (outlook link removed)", orgID, local.ID)
	return nil
}

// Outbound event bodies are stamped so inbound classification can ignore them.
const platformGeneratedMarker = "[Platform Generated]"

func buildCreateEventRequest(e *Event) outlook.CreateEventRequest {
	req := outlook.CreateEventRequest{
		Subject: e.Title,
		Body: outlook.EventBody{
			ContentType: "text",
			Content:     fmt.Sprintf("%s\n\n%s", e.Description, platformGeneratedMarker),
		},
		Start:    toGraphDateTime(e.StartAt),
		End:      toGraphDateTime(e.EndAt),
		IsAllDay: e.AllDay,
	}
	if e.Location != "" {
		req.Location = &outlook.Location{DisplayName: e.Location}
	}
	return req
}

func buildUpdateEventRequest(e *Event) outlook.UpdateEventRequest {
	subject := e.Title
	body := outlook.EventBody{
		ContentType: "text",
		Content:     fmt.Sprintf("%s\n\n%s", e.Description, platformGeneratedMarker),
	}
	start := toGraphDateTime(e.StartAt)
	end := toGraphDateTime(e.EndAt)
	allDay := e.AllDay
	req := outlook.UpdateEventRequest{
		Subject:  &subject,
		Body:     &body,
		Start:    &start,
		End:      &end,
		IsAllDay: &allDay,
	}
	if e.Location != "" {
		req.Location = &outlook.Location{DisplayName: e.Location}
	}
	return req
}

// toGraphDateTime renders a time in UTC for Graph. (The AVMRE source
// hardcoded "Eastern Standard Time"; UTC is org-neutral.)
func toGraphDateTime(t time.Time) outlook.DateTimeTZ {
	return outlook.DateTimeTZ{
		DateTime: t.UTC().Format("2006-01-02T15:04:05"),
		TimeZone: "UTC",
	}
}

// ================================================================
// DELTA SYNC
// ================================================================

// RunDeltaSync performs a delta reconciliation sweep for one org.
// If start/end are provided the checkpoint is bypassed and that exact range
// is queried; otherwise the stored per-org delta link is used and advanced.
func (s *Service) RunDeltaSync(ctx context.Context, orgID int64, start, end *time.Time) (int, error) {
	if syncPaused() {
		log.Printf("[calendar] sync paused (CALENDAR_SYNC_PAUSED); skipping RunDeltaSync for org %d", orgID)
		return 0, nil
	}

	graph, conn, err := s.graphFor(ctx, orgID)
	if err != nil {
		return 0, err
	}

	resource := fmt.Sprintf("users/%s/calendarView", conn.MailboxUPN)

	var deltaLink string
	if start == nil && end == nil {
		deltaLink, err = s.repo.GetDeltaLink(ctx, orgID, resource)
		if err != nil {
			return 0, err
		}
	}

	resp, err := graph.ListEventsDelta(ctx, deltaLink, start, end)
	if err != nil {
		return 0, fmt.Errorf("delta query: %w", err)
	}

	// Skip events older than 7 days so the first full scan doesn't flood
	// the drafts queue with ancient events.
	cutoff := time.Now().AddDate(0, 0, -7)

	processed := 0
	skippedOld := 0
	for _, ev := range resp.Events {
		if evStart := parseDateTimeTZ(ev.Start); evStart != nil && evStart.Before(cutoff) {
			skippedOld++
			continue
		}
		if err := s.SyncEventFromOutlook(ctx, orgID, ev.ID); err != nil {
			log.Printf("[calendar] org %d: delta sync error for event %s: %v", orgID, ev.ID, err)
			continue
		}
		processed++
	}
	if skippedOld > 0 {
		log.Printf("[calendar] org %d: delta sync skipped %d events older than 7 days", orgID, skippedOld)
	}

	// Only save the delta checkpoint on a full (unfiltered) sync.
	if start == nil && end == nil && resp.DeltaLink != "" {
		if err := s.repo.UpsertDeltaLink(ctx, orgID, resource, resp.DeltaLink); err != nil {
			log.Printf("[calendar] org %d: failed to save delta checkpoint: %v", orgID, err)
		}
	}

	log.Printf("[calendar] org %d: delta sync complete: %d events processed", orgID, processed)
	return processed, nil
}

// ================================================================
// SUBSCRIPTION MANAGEMENT
// ================================================================

// EnsureSubscription creates or renews the org's Graph webhook subscription.
func (s *Service) EnsureSubscription(ctx context.Context, orgID int64) error {
	if syncPaused() {
		log.Printf("[calendar] sync paused (CALENDAR_SYNC_PAUSED); skipping EnsureSubscription for org %d", orgID)
		return nil
	}
	if s.webhookURL == "" {
		return fmt.Errorf("CALENDAR_WEBHOOK_URL is not configured; cannot create Graph subscription")
	}

	graph, conn, err := s.graphFor(ctx, orgID)
	if err != nil {
		return err
	}

	active, err := s.repo.ListActiveSubscriptions(ctx, orgID)
	if err != nil {
		return fmt.Errorf("list active subscriptions: %w", err)
	}

	// Subscribe to a specific calendar when CalendarID is set; otherwise the default calendar.
	var resource string
	if conn.CalendarID != "" {
		resource = fmt.Sprintf("users/%s/calendars/%s/events", conn.MailboxUPN, conn.CalendarID)
	} else {
		resource = fmt.Sprintf("users/%s/events", conn.MailboxUPN)
	}

	for _, sub := range active {
		if sub.Resource != resource {
			continue
		}
		if time.Until(sub.Expiration) > 12*time.Hour {
			log.Printf("[calendar] org %d: subscription %s still valid until %s", orgID, sub.GraphSubscriptionID, sub.Expiration)
			return nil
		}

		newExpiry := time.Now().Add(outlook.MaxSubscriptionLifetime)
		renewed, err := graph.RenewSubscription(ctx, sub.GraphSubscriptionID, newExpiry)
		if err != nil {
			log.Printf("[calendar] org %d: failed to renew subscription %s, creating new: %v", orgID, sub.GraphSubscriptionID, err)
			if deactErr := s.repo.DeactivateSubscription(ctx, orgID, sub.ID); deactErr != nil {
				log.Printf("[calendar] org %d: failed to deactivate old subscription: %v", orgID, deactErr)
			}
			break
		}

		parsedExpiry, _ := time.Parse(time.RFC3339, renewed.ExpirationDateTime)
		if err := s.repo.UpdateSubscriptionExpiration(ctx, orgID, sub.ID, parsedExpiry); err != nil {
			return fmt.Errorf("update subscription expiration: %w", err)
		}
		log.Printf("[calendar] org %d: renewed subscription %s until %s", orgID, sub.GraphSubscriptionID, parsedExpiry)
		return nil
	}

	return s.createNewSubscription(ctx, graph, orgID, resource)
}

func (s *Service) createNewSubscription(ctx context.Context, graph *outlook.GraphCalendarClient, orgID int64, resource string) error {
	clientState := fmt.Sprintf("mh-cal-%d-%d", orgID, time.Now().UnixNano())
	expiry := time.Now().Add(outlook.MaxSubscriptionLifetime)

	graphSub, err := graph.CreateSubscription(ctx, outlook.CreateSubscriptionRequest{
		ChangeType:         "created,updated,deleted",
		NotificationURL:    s.webhookURL,
		Resource:           resource,
		ExpirationDateTime: expiry.UTC().Format(time.RFC3339),
		ClientState:        clientState,
	})
	if err != nil {
		return fmt.Errorf("create graph subscription: %w", err)
	}

	parsedExpiry, _ := time.Parse(time.RFC3339, graphSub.ExpirationDateTime)

	_, err = s.repo.CreateSubscription(ctx, &Subscription{
		OrgID:               orgID,
		GraphSubscriptionID: graphSub.ID,
		Resource:            resource,
		Expiration:          parsedExpiry,
		ClientState:         clientState,
	})
	if err != nil {
		return fmt.Errorf("save subscription record: %w", err)
	}

	log.Printf("[calendar] org %d: created subscription %s for %s (expires %s)", orgID, graphSub.ID, resource, parsedExpiry)
	return nil
}

// ================================================================
// WEBHOOK NOTIFICATION HANDLER
// ================================================================

// HandleWebhookNotification validates and dispatches a single Graph
// notification. The org is resolved via the stored subscription id → org_id
// mapping (the webhook endpoint itself is unauthenticated).
func (s *Service) HandleWebhookNotification(ctx context.Context, subscriptionID, clientState, resource, changeType string) error {
	if syncPaused() {
		log.Printf("[calendar] sync paused (CALENDAR_SYNC_PAUSED); skipping webhook notification for subscription %s", subscriptionID)
		return nil
	}
	sub, err := s.repo.GetSubscriptionByGraphID(ctx, subscriptionID)
	if err != nil {
		return fmt.Errorf("lookup subscription: %w", err)
	}
	if sub == nil {
		return fmt.Errorf("unknown subscription: %s", subscriptionID)
	}
	if sub.ClientState != clientState {
		return fmt.Errorf("client state mismatch for subscription %s", subscriptionID)
	}

	eventID := outlook.ExtractEventID(resource)
	if eventID == "" {
		log.Printf("[calendar] could not extract event ID from resource: %s", resource)
		return nil
	}

	if changeType == "deleted" {
		return s.handleCancelledEvent(ctx, sub.OrgID, eventID)
	}

	return s.SyncEventFromOutlook(ctx, sub.OrgID, eventID)
}

// ================================================================
// LOOP PREVENTION / HELPERS
// ================================================================

// shouldSkipInbound returns true if the inbound change was likely caused by
// our own outbound sync (same changeKey we stored after a local_to_outlook
// write). Prevents infinite update loops.
func shouldSkipInbound(e *Event, incomingChangeKey string) bool {
	if e.LastSyncDirection == nil || *e.LastSyncDirection != DirectionLocalToOutlook {
		return false
	}
	if e.OutlookChangeKey == nil {
		return false
	}
	return *e.OutlookChangeKey == incomingChangeKey
}

// windowsToIANA maps common Microsoft/Windows timezone names to IANA names.
var windowsToIANA = map[string]string{
	"Eastern Standard Time":   "America/New_York",
	"Central Standard Time":   "America/Chicago",
	"Mountain Standard Time":  "America/Denver",
	"Pacific Standard Time":   "America/Los_Angeles",
	"UTC":                     "UTC",
	"GMT Standard Time":       "Europe/London",
	"Romance Standard Time":   "Europe/Paris",
	"W. Europe Standard Time": "Europe/Berlin",
	"Tokyo Standard Time":     "Asia/Tokyo",
}

func parseDateTimeTZ(dt outlook.DateTimeTZ) *time.Time {
	if dt.DateTime == "" {
		return nil
	}

	// Resolve the timezone location from the Graph API timezone field
	var loc *time.Location
	if dt.TimeZone != "" {
		// Try IANA name directly first
		if l, err := time.LoadLocation(dt.TimeZone); err == nil {
			loc = l
		} else if iana, ok := windowsToIANA[dt.TimeZone]; ok {
			// Fall back to Windows-to-IANA mapping
			if l, err := time.LoadLocation(iana); err == nil {
				loc = l
			}
		}
	}

	layouts := []string{
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05.0000000",
		time.RFC3339,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, dt.DateTime); err == nil {
			if loc != nil && t.Location() == time.UTC {
				// The parsed time has no timezone info — apply the Graph timezone
				t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), loc)
			}
			return &t
		}
	}
	return nil
}
