package calendar

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repo is the Postgres repository for the calendar module. Every query is
// org-scoped except GetSubscriptionByGraphID, which is the lookup that
// resolves an inbound webhook notification to its org.
type Repo struct{ db *pgxpool.Pool }

func NewRepo(db *pgxpool.Pool) *Repo { return &Repo{db: db} }

// ---------- org_calendar_connections ----------

const connectionCols = `id, org_id, tenant_id, client_id, client_secret, mailbox_upn, calendar_id, enabled, created_at`

func scanConnection(row pgx.Row) (*Connection, error) {
	var c Connection
	err := row.Scan(&c.ID, &c.OrgID, &c.TenantID, &c.ClientID, &c.ClientSecret,
		&c.MailboxUPN, &c.CalendarID, &c.Enabled, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan connection: %w", err)
	}
	return &c, nil
}

// GetConnection returns the org's Outlook connection, or (nil, nil) if not configured.
func (r *Repo) GetConnection(ctx context.Context, orgID int64) (*Connection, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+connectionCols+` FROM org_calendar_connections WHERE org_id = $1`, orgID)
	return scanConnection(row)
}

// UpsertConnection creates or replaces the org's Outlook connection.
func (r *Repo) UpsertConnection(ctx context.Context, c *Connection) (*Connection, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO org_calendar_connections
			(org_id, tenant_id, client_id, client_secret, mailbox_upn, calendar_id, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (org_id) DO UPDATE SET
			tenant_id     = EXCLUDED.tenant_id,
			client_id     = EXCLUDED.client_id,
			client_secret = EXCLUDED.client_secret,
			mailbox_upn   = EXCLUDED.mailbox_upn,
			calendar_id   = EXCLUDED.calendar_id,
			enabled       = EXCLUDED.enabled
		RETURNING `+connectionCols,
		c.OrgID, c.TenantID, c.ClientID, c.ClientSecret, c.MailboxUPN, c.CalendarID, c.Enabled)
	return scanConnection(row)
}

func (r *Repo) DeleteConnection(ctx context.Context, orgID int64) (bool, error) {
	ct, err := r.db.Exec(ctx, `DELETE FROM org_calendar_connections WHERE org_id = $1`, orgID)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

// ---------- calendar_events ----------

const eventCols = `id, org_id, title, description, location, start_at, end_at, all_day,
	show_as, is_private, organizer_name, source, outlook_event_id, outlook_change_key,
	last_sync_direction, sync_status, created_at, updated_at`

func scanEvent(row pgx.Row) (*Event, error) {
	var e Event
	err := row.Scan(&e.ID, &e.OrgID, &e.Title, &e.Description, &e.Location,
		&e.StartAt, &e.EndAt, &e.AllDay, &e.ShowAs, &e.IsPrivate, &e.OrganizerName,
		&e.Source, &e.OutlookEventID, &e.OutlookChangeKey, &e.LastSyncDirection,
		&e.SyncStatus, &e.CreatedAt, &e.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan event: %w", err)
	}
	return &e, nil
}

// UpsertEventFromOutlook inserts or updates the org's local mirror of an
// Outlook event, keyed on (org_id, outlook_event_id). Returns the row and
// whether it was newly inserted.
func (r *Repo) UpsertEventFromOutlook(ctx context.Context, e *Event) (*Event, bool, error) {
	if e.OutlookEventID == nil || *e.OutlookEventID == "" {
		return nil, false, fmt.Errorf("upsert from outlook requires outlook_event_id")
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO calendar_events
			(org_id, title, description, location, start_at, end_at, all_day, show_as,
			 is_private, organizer_name, source, outlook_event_id, outlook_change_key,
			 last_sync_direction, sync_status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'outlook', $11, $12, $13, 'active')
		ON CONFLICT (org_id, outlook_event_id) WHERE outlook_event_id IS NOT NULL
		DO UPDATE SET
			title               = EXCLUDED.title,
			description         = EXCLUDED.description,
			location            = EXCLUDED.location,
			start_at            = EXCLUDED.start_at,
			end_at              = EXCLUDED.end_at,
			all_day             = EXCLUDED.all_day,
			show_as             = EXCLUDED.show_as,
			is_private          = EXCLUDED.is_private,
			organizer_name      = EXCLUDED.organizer_name,
			outlook_change_key  = EXCLUDED.outlook_change_key,
			last_sync_direction = EXCLUDED.last_sync_direction,
			sync_status         = 'active',
			updated_at          = now()
		RETURNING `+eventCols+`, (xmax = 0) AS inserted`,
		e.OrgID, e.Title, e.Description, e.Location, e.StartAt, e.EndAt, e.AllDay,
		e.ShowAs, e.IsPrivate, e.OrganizerName, e.OutlookEventID, e.OutlookChangeKey,
		e.LastSyncDirection)

	var out Event
	var inserted bool
	err := row.Scan(&out.ID, &out.OrgID, &out.Title, &out.Description, &out.Location,
		&out.StartAt, &out.EndAt, &out.AllDay, &out.ShowAs, &out.IsPrivate, &out.OrganizerName,
		&out.Source, &out.OutlookEventID, &out.OutlookChangeKey, &out.LastSyncDirection,
		&out.SyncStatus, &out.CreatedAt, &out.UpdatedAt, &inserted)
	if err != nil {
		return nil, false, fmt.Errorf("upsert event from outlook: %w", err)
	}
	return &out, inserted, nil
}

// CreateLocalEvent inserts a locally created event (not yet linked to Outlook).
func (r *Repo) CreateLocalEvent(ctx context.Context, e *Event) (*Event, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO calendar_events
			(org_id, title, description, location, start_at, end_at, all_day, show_as,
			 is_private, organizer_name, source, sync_status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'local', 'active')
		RETURNING `+eventCols,
		e.OrgID, e.Title, e.Description, e.Location, e.StartAt, e.EndAt, e.AllDay,
		e.ShowAs, e.IsPrivate, e.OrganizerName)
	return scanEvent(row)
}

func (r *Repo) GetEvent(ctx context.Context, orgID, id int64) (*Event, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+eventCols+` FROM calendar_events WHERE id = $1 AND org_id = $2`, id, orgID)
	return scanEvent(row)
}

func (r *Repo) GetEventByOutlookID(ctx context.Context, orgID int64, outlookEventID string) (*Event, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+eventCols+` FROM calendar_events
		 WHERE org_id = $1 AND outlook_event_id = $2`, orgID, outlookEventID)
	return scanEvent(row)
}

// ListEvents returns the org's events, optionally bounded to [from, to).
func (r *Repo) ListEvents(ctx context.Context, orgID int64, from, to *time.Time) ([]Event, error) {
	q := `SELECT ` + eventCols + ` FROM calendar_events WHERE org_id = $1`
	args := []any{orgID}
	if from != nil {
		args = append(args, *from)
		q += fmt.Sprintf(` AND end_at >= $%d`, len(args))
	}
	if to != nil {
		args = append(args, *to)
		q += fmt.Sprintf(` AND start_at < $%d`, len(args))
	}
	q += ` ORDER BY start_at`
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// SetEventOutlookLink stores the Outlook linkage after an outbound create.
func (r *Repo) SetEventOutlookLink(ctx context.Context, orgID, id int64, outlookEventID, changeKey string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE calendar_events
		SET outlook_event_id = $3, outlook_change_key = $4,
		    last_sync_direction = 'local_to_outlook', updated_at = now()
		WHERE id = $1 AND org_id = $2`, id, orgID, outlookEventID, changeKey)
	return err
}

// UpdateEventSyncMeta refreshes change key + direction after an outbound update.
func (r *Repo) UpdateEventSyncMeta(ctx context.Context, orgID, id int64, changeKey, direction string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE calendar_events
		SET outlook_change_key = $3, last_sync_direction = $4, updated_at = now()
		WHERE id = $1 AND org_id = $2`, id, orgID, changeKey, direction)
	return err
}

func (r *Repo) SetEventStatus(ctx context.Context, orgID, id int64, status string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE calendar_events SET sync_status = $3, updated_at = now()
		WHERE id = $1 AND org_id = $2`, id, orgID, status)
	return err
}

// MarkEventCancelledByOutlookID marks the local mirror of a cancelled/deleted
// Outlook event. Returns true if a row was updated.
func (r *Repo) MarkEventCancelledByOutlookID(ctx context.Context, orgID int64, outlookEventID string) (bool, error) {
	ct, err := r.db.Exec(ctx, `
		UPDATE calendar_events SET sync_status = 'cancelled', updated_at = now()
		WHERE org_id = $1 AND outlook_event_id = $2 AND sync_status = 'active'`,
		orgID, outlookEventID)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

// ---------- calendar_subscriptions ----------

const subscriptionCols = `id, org_id, graph_subscription_id, resource, expiration, client_state, active, created_at, updated_at`

func scanSubscription(row pgx.Row) (*Subscription, error) {
	var s Subscription
	err := row.Scan(&s.ID, &s.OrgID, &s.GraphSubscriptionID, &s.Resource,
		&s.Expiration, &s.ClientState, &s.Active, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan subscription: %w", err)
	}
	return &s, nil
}

func (r *Repo) CreateSubscription(ctx context.Context, s *Subscription) (*Subscription, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO calendar_subscriptions
			(org_id, graph_subscription_id, resource, expiration, client_state, active)
		VALUES ($1, $2, $3, $4, $5, true)
		RETURNING `+subscriptionCols,
		s.OrgID, s.GraphSubscriptionID, s.Resource, s.Expiration, s.ClientState)
	return scanSubscription(row)
}

// GetSubscriptionByGraphID is intentionally NOT org-filtered: it is the
// webhook receiver's way of resolving which org a Graph notification belongs to.
func (r *Repo) GetSubscriptionByGraphID(ctx context.Context, graphSubID string) (*Subscription, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+subscriptionCols+` FROM calendar_subscriptions WHERE graph_subscription_id = $1`,
		graphSubID)
	return scanSubscription(row)
}

func (r *Repo) ListActiveSubscriptions(ctx context.Context, orgID int64) ([]*Subscription, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+subscriptionCols+` FROM calendar_subscriptions
		 WHERE org_id = $1 AND active = true ORDER BY expiration`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list active subscriptions: %w", err)
	}
	defer rows.Close()
	var subs []*Subscription
	for rows.Next() {
		s, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

func (r *Repo) UpdateSubscriptionExpiration(ctx context.Context, orgID, id int64, expiration time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE calendar_subscriptions SET expiration = $3, updated_at = now()
		WHERE id = $1 AND org_id = $2`, id, orgID, expiration)
	return err
}

func (r *Repo) DeactivateSubscription(ctx context.Context, orgID, id int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE calendar_subscriptions SET active = false, updated_at = now()
		WHERE id = $1 AND org_id = $2`, id, orgID)
	return err
}

// ---------- calendar_sync_state (delta checkpoints) ----------

func (r *Repo) GetDeltaLink(ctx context.Context, orgID int64, resource string) (string, error) {
	var link string
	err := r.db.QueryRow(ctx, `
		SELECT delta_link FROM calendar_sync_state
		WHERE org_id = $1 AND resource = $2`, orgID, resource).Scan(&link)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get delta link: %w", err)
	}
	return link, nil
}

func (r *Repo) UpsertDeltaLink(ctx context.Context, orgID int64, resource, deltaLink string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO calendar_sync_state (org_id, resource, delta_link, last_checked_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (org_id, resource)
		DO UPDATE SET delta_link = $3, last_checked_at = now()`,
		orgID, resource, deltaLink)
	return err
}
