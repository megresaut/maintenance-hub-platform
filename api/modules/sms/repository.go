package sms

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct {
	db *pgxpool.Pool
}

func NewRepo(db *pgxpool.Pool) *Repo {
	return &Repo{db: db}
}

const convCols = `id, org_id, phone_number, twilio_sid, status, last_message_at,
	last_classified_at, is_group, group_name, participants, created_at, updated_at`

func scanConv(row pgx.Row) (*Conversation, error) {
	var c Conversation
	err := row.Scan(&c.ID, &c.OrgID, &c.PhoneNumber, &c.TwilioSID, &c.Status,
		&c.LastMessageAt, &c.LastClassifiedAt, &c.IsGroup, &c.GroupName,
		&c.Participants, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpsertConversation finds or creates the 1:1 conversation for a phone number.
func (r *Repo) UpsertConversation(ctx context.Context, orgID int64, phoneNumber, mmsSubject string) (*Conversation, error) {
	q := `INSERT INTO sms_conversations (org_id, phone_number, group_name)
	      VALUES ($1, $2, NULLIF($3, ''))
	      ON CONFLICT (org_id, phone_number)
	      DO UPDATE SET group_name = COALESCE(NULLIF($3, ''), sms_conversations.group_name),
	                    updated_at = now()
	      RETURNING ` + convCols
	c, err := scanConv(r.db.QueryRow(ctx, q, orgID, phoneNumber, mmsSubject))
	if err != nil {
		return nil, fmt.Errorf("upsert conversation: %w", err)
	}
	return c, nil
}

func (r *Repo) GetConversationByID(ctx context.Context, orgID, id int64) (*Conversation, error) {
	c, err := scanConv(r.db.QueryRow(ctx,
		`SELECT `+convCols+` FROM sms_conversations WHERE id = $1 AND org_id = $2`, id, orgID))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return c, err
}

// FindGroupByMemberPhone returns the group conversation containing a phone
// number, if any (compared on last-10-digits).
func (r *Repo) FindGroupByMemberPhone(ctx context.Context, orgID int64, phoneNumber string) (*Conversation, error) {
	q := `SELECT ` + convCols + `
	      FROM sms_conversations
	      WHERE org_id = $1 AND is_group = true
	        AND EXISTS (
	          SELECT 1 FROM unnest(participants) p
	          WHERE RIGHT(regexp_replace(p, '[^0-9]', '', 'g'), 10)
	              = RIGHT(regexp_replace($2, '[^0-9]', '', 'g'), 10)
	        )
	      LIMIT 1`
	c, err := scanConv(r.db.QueryRow(ctx, q, orgID, phoneNumber))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return c, err
}

func (r *Repo) CreateGroupConversation(ctx context.Context, orgID int64, name string, participants []string) (*Conversation, error) {
	q := `INSERT INTO sms_conversations (org_id, phone_number, is_group, group_name, participants)
	      VALUES ($1, $2, true, $3, $4)
	      RETURNING ` + convCols
	c, err := scanConv(r.db.QueryRow(ctx, q, orgID, "group:"+name, name, participants))
	if err != nil {
		return nil, fmt.Errorf("create group conversation: %w", err)
	}
	return c, nil
}

func (r *Repo) ListConversations(ctx context.Context, orgID int64) ([]*Conversation, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+convCols+` FROM sms_conversations WHERE org_id = $1
		 ORDER BY last_message_at DESC NULLS LAST`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Conversation{}
	for rows.Next() {
		c, err := scanConv(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repo) UpdateLastMessageAt(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE sms_conversations SET last_message_at = now(), updated_at = now() WHERE id = $1`, id)
	return err
}

func (r *Repo) UpdateLastClassifiedAt(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE sms_conversations SET last_classified_at = now(), updated_at = now() WHERE id = $1`, id)
	return err
}

// GetStaleConversations returns conversations with unprocessed messages
// older than cutoff (for the background classification poller).
func (r *Repo) GetStaleConversations(ctx context.Context, cutoff time.Time) ([]*Conversation, error) {
	q := `SELECT DISTINCT ` + qualCols("c") + `
	      FROM sms_conversations c
	      JOIN sms_messages m ON m.conversation_id = c.id
	      WHERE m.processed = false AND m.direction = 'inbound' AND m.created_at < $1`
	rows, err := r.db.Query(ctx, q, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Conversation{}
	for rows.Next() {
		c, err := scanConv(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repo) InsertMessage(ctx context.Context, msg *Message) (*Message, error) {
	media := msg.MediaURLs
	if len(media) == 0 {
		media = json.RawMessage(`[]`)
	}
	q := `INSERT INTO sms_messages (conversation_id, twilio_sid, direction, from_number, to_number, body, media_urls)
	      VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
	      RETURNING id, created_at`
	err := r.db.QueryRow(ctx, q,
		msg.ConversationID, msg.TwilioSID, msg.Direction, msg.FromNumber, msg.ToNumber,
		msg.Body, string(media),
	).Scan(&msg.ID, &msg.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert message: %w", err)
	}
	return msg, nil
}

func (r *Repo) GetUnprocessedMessages(ctx context.Context, conversationID int64) ([]*Message, error) {
	return r.queryMessages(ctx, `
		SELECT id, conversation_id, twilio_sid, direction, from_number, to_number, body, media_urls, processed, created_at
		FROM sms_messages
		WHERE conversation_id = $1 AND processed = false AND direction = 'inbound'
		ORDER BY created_at`, conversationID)
}

func (r *Repo) GetRecentMessages(ctx context.Context, conversationID int64, limit int) ([]*Message, error) {
	return r.queryMessages(ctx, `
		SELECT id, conversation_id, twilio_sid, direction, from_number, to_number, body, media_urls, processed, created_at
		FROM sms_messages
		WHERE conversation_id = $1
		ORDER BY created_at DESC
		LIMIT $2`, conversationID, limit)
}

func (r *Repo) MarkMessagesProcessed(ctx context.Context, conversationID int64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE sms_messages SET processed = true WHERE conversation_id = $1 AND processed = false`, conversationID)
	return err
}

func (r *Repo) queryMessages(ctx context.Context, q string, args ...any) ([]*Message, error) {
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.TwilioSID, &m.Direction,
			&m.FromNumber, &m.ToNumber, &m.Body, &m.MediaURLs, &m.Processed, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

func qualCols(alias string) string {
	return alias + `.id, ` + alias + `.org_id, ` + alias + `.phone_number, ` + alias + `.twilio_sid, ` +
		alias + `.status, ` + alias + `.last_message_at, ` + alias + `.last_classified_at, ` +
		alias + `.is_group, ` + alias + `.group_name, ` + alias + `.participants, ` +
		alias + `.created_at, ` + alias + `.updated_at`
}
