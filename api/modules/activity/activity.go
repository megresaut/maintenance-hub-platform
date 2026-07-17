package activity

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Entry is one row in a ticket's unified activity feed: status changes,
// notes, outreach sent/received, dispatch. ticket_type distinguishes
// work_order / fto / task.
type Entry struct {
	ID         int64           `json:"id"`
	OrgID      int64           `json:"org_id"`
	TicketType string          `json:"ticket_type"`
	TicketID   int64           `json:"ticket_id"`
	Kind       string          `json:"kind"`
	ActorType  string          `json:"actor_type"` // user | system | vendor | ai
	ActorID    *int64          `json:"actor_id,omitempty"`
	ActorName  string          `json:"actor_name,omitempty"`
	Body       string          `json:"body"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

// Kinds
const (
	KindCreated          = "created"
	KindStatusChange     = "status_change"
	KindNote             = "note"
	KindOutreachSent     = "outreach_sent"
	KindOutreachReply    = "outreach_reply"
	KindDispatched       = "dispatched"
	KindAssigned         = "assigned"
	KindDraftApproved    = "draft_approved"
)

type Recorder struct {
	db *pgxpool.Pool
}

func NewRecorder(db *pgxpool.Pool) *Recorder {
	return &Recorder{db: db}
}

// Record appends an entry to the feed. Failures are logged, not returned —
// activity logging must never break the main operation.
func (rec *Recorder) Record(ctx context.Context, e Entry) {
	meta := e.Metadata
	if meta == nil {
		meta = json.RawMessage(`{}`)
	}
	_, err := rec.db.Exec(ctx, `
		INSERT INTO ticket_activity
			(org_id, ticket_type, ticket_id, kind, actor_type, actor_id, actor_name, body, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		e.OrgID, e.TicketType, e.TicketID, e.Kind, e.ActorType, e.ActorID, e.ActorName, e.Body, meta)
	if err != nil {
		log.Printf("activity: failed to record %s on %s/%d: %v", e.Kind, e.TicketType, e.TicketID, err)
	}
}

// List returns the feed for one ticket, oldest first.
func (rec *Recorder) List(ctx context.Context, orgID int64, ticketType string, ticketID int64) ([]Entry, error) {
	rows, err := rec.db.Query(ctx, `
		SELECT id, org_id, ticket_type, ticket_id, kind, actor_type, actor_id,
		       COALESCE(actor_name, ''), body, metadata, created_at
		FROM ticket_activity
		WHERE org_id = $1 AND ticket_type = $2 AND ticket_id = $3
		ORDER BY created_at ASC, id ASC`, orgID, ticketType, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.OrgID, &e.TicketType, &e.TicketID, &e.Kind,
			&e.ActorType, &e.ActorID, &e.ActorName, &e.Body, &e.Metadata, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
