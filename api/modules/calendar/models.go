package calendar

import (
	"context"
	"time"
)

// DraftCreator is the narrow handoff interface to the AI/drafts module.
// The calendar module deliberately does NOT import maintenancehub/modules/ai;
// the integrator passes an implementation into NewService. When nil,
// classification is disabled and inbound events are only mirrored into
// calendar_events (with a log line noting classification was skipped).
type DraftCreator interface {
	ClassifyAndDraft(ctx context.Context, orgID int64, source string, sourceRef string, content string) error
}

// Sync direction / status / source constants.
const (
	DirectionOutlookToLocal = "outlook_to_local"
	DirectionLocalToOutlook = "local_to_outlook"

	SyncStatusActive    = "active"
	SyncStatusCancelled = "cancelled"
	SyncStatusDeleted   = "deleted"

	SourceOutlook = "outlook"
	SourceLocal   = "local"
)

// Connection is a per-org Outlook (Microsoft Graph) connection using the
// client-credentials flow. One row per org.
type Connection struct {
	ID           int64     `json:"id"`
	OrgID        int64     `json:"org_id"`
	TenantID     string    `json:"tenant_id"`
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"-"` // never serialized
	MailboxUPN   string    `json:"mailbox_upn"`
	CalendarID   string    `json:"calendar_id"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
}

// Event is an org-scoped calendar event mirrored from (or pushed to) Outlook.
// Outlook linkage (outlook_event_id/change_key/direction/status) is folded
// into the event row itself — one Outlook event maps to at most one local row
// per org.
type Event struct {
	ID                int64     `json:"id"`
	OrgID             int64     `json:"org_id"`
	Title             string    `json:"title"`
	Description       string    `json:"description"`
	Location          string    `json:"location"`
	StartAt           time.Time `json:"start_at"`
	EndAt             time.Time `json:"end_at"`
	AllDay            bool      `json:"all_day"`
	ShowAs            string    `json:"show_as"`
	IsPrivate         bool      `json:"is_private"`
	OrganizerName     string    `json:"organizer_name"`
	Source            string    `json:"source"` // outlook | local
	OutlookEventID    *string   `json:"outlook_event_id,omitempty"`
	OutlookChangeKey  *string   `json:"outlook_change_key,omitempty"`
	LastSyncDirection *string   `json:"last_sync_direction,omitempty"`
	SyncStatus        string    `json:"sync_status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Subscription is an org-scoped Graph webhook subscription record. The
// graph_subscription_id → org_id mapping is how inbound (unauthenticated)
// webhook notifications are resolved to an org.
type Subscription struct {
	ID                  int64     `json:"id"`
	OrgID               int64     `json:"org_id"`
	GraphSubscriptionID string    `json:"graph_subscription_id"`
	Resource            string    `json:"resource"`
	Expiration          time.Time `json:"expiration"`
	ClientState         string    `json:"-"`
	Active              bool      `json:"active"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// SyncState stores the per-org Graph delta checkpoint for a resource.
type SyncState struct {
	ID            int64     `json:"id"`
	OrgID         int64     `json:"org_id"`
	Resource      string    `json:"resource"`
	DeltaLink     string    `json:"delta_link"`
	LastCheckedAt time.Time `json:"last_checked_at"`
}
