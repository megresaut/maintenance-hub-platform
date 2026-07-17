package outreach

import (
	"context"
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

const reqCols = `r.id, r.org_id, r.work_order_id, r.vendor_id, r.channel, r.to_address,
	r.message_body, r.provider_ref, r.status, r.error, r.sent_at, r.created_at`

func scanRequest(row pgx.Row) (*Request, error) {
	var q Request
	err := row.Scan(&q.ID, &q.OrgID, &q.WorkOrderID, &q.VendorID, &q.Channel, &q.ToAddress,
		&q.MessageBody, &q.ProviderRef, &q.Status, &q.Error, &q.SentAt, &q.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &q, nil
}

func (r *Repo) CreateRequest(ctx context.Context, req *Request) (*Request, error) {
	q := `INSERT INTO vendor_outreach_requests
	        (org_id, work_order_id, vendor_id, channel, to_address, message_body, status)
	      VALUES ($1, $2, $3, $4, $5, $6, $7)
	      RETURNING id, created_at`
	err := r.db.QueryRow(ctx, q, req.OrgID, req.WorkOrderID, req.VendorID, req.Channel,
		req.ToAddress, req.MessageBody, req.Status).Scan(&req.ID, &req.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create outreach request: %w", err)
	}
	return req, nil
}

func (r *Repo) MarkSent(ctx context.Context, id int64, providerRef string) error {
	now := time.Now()
	_, err := r.db.Exec(ctx, `
		UPDATE vendor_outreach_requests
		SET status = 'sent', provider_ref = NULLIF($1,''), sent_at = $2
		WHERE id = $3`, providerRef, now, id)
	return err
}

func (r *Repo) MarkFailed(ctx context.Context, id int64, errMsg string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE vendor_outreach_requests SET status = 'failed', error = $1 WHERE id = $2`,
		errMsg, id)
	return err
}

func (r *Repo) SetStatus(ctx context.Context, orgID, id int64, status RequestStatus) error {
	_, err := r.db.Exec(ctx, `
		UPDATE vendor_outreach_requests SET status = $1 WHERE id = $2 AND org_id = $3`,
		string(status), id, orgID)
	return err
}

// ListForWorkOrder returns all outreach requests for a work order with
// vendor contact info and replies attached — the side-by-side comparison data.
func (r *Repo) ListForWorkOrder(ctx context.Context, orgID, workOrderID int64) ([]*Request, error) {
	q := `SELECT ` + reqCols + `, v.name, v.phone, v.primary_email
	      FROM vendor_outreach_requests r
	      JOIN vendors v ON v.id = r.vendor_id
	      WHERE r.org_id = $1 AND r.work_order_id = $2
	      ORDER BY r.created_at`
	rows, err := r.db.Query(ctx, q, orgID, workOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*Request{}
	for rows.Next() {
		var req Request
		if err := rows.Scan(&req.ID, &req.OrgID, &req.WorkOrderID, &req.VendorID, &req.Channel,
			&req.ToAddress, &req.MessageBody, &req.ProviderRef, &req.Status, &req.Error,
			&req.SentAt, &req.CreatedAt, &req.VendorName, &req.VendorPhone, &req.VendorEmail); err != nil {
			return nil, err
		}
		out = append(out, &req)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, req := range out {
		replies, err := r.ListReplies(ctx, req.ID)
		if err != nil {
			return nil, err
		}
		req.Replies = replies
	}
	return out, nil
}

func (r *Repo) ListReplies(ctx context.Context, requestID int64) ([]*Reply, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, outreach_request_id, body, media_urls, parsed_quote_cents,
		       parsed_availability, raw_source_ref, received_at, created_at
		FROM vendor_outreach_replies
		WHERE outreach_request_id = $1
		ORDER BY received_at`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Reply{}
	for rows.Next() {
		var rep Reply
		if err := rows.Scan(&rep.ID, &rep.OutreachRequestID, &rep.Body, &rep.MediaURLs,
			&rep.ParsedQuoteCents, &rep.ParsedAvailability, &rep.RawSourceRef,
			&rep.ReceivedAt, &rep.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &rep)
	}
	return out, rows.Err()
}

func (r *Repo) GetRequest(ctx context.Context, orgID, id int64) (*Request, error) {
	req, err := scanRequest(r.db.QueryRow(ctx,
		`SELECT `+reqCols+` FROM vendor_outreach_requests r WHERE r.id = $1 AND r.org_id = $2`,
		id, orgID))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return req, err
}

func (r *Repo) GetReply(ctx context.Context, orgID, replyID int64) (*Reply, *Request, error) {
	var rep Reply
	var req Request
	err := r.db.QueryRow(ctx, `
		SELECT rep.id, rep.outreach_request_id, rep.body, rep.media_urls, rep.parsed_quote_cents,
		       rep.parsed_availability, rep.raw_source_ref, rep.received_at, rep.created_at,
		       `+reqCols+`
		FROM vendor_outreach_replies rep
		JOIN vendor_outreach_requests r ON r.id = rep.outreach_request_id
		WHERE rep.id = $1 AND r.org_id = $2`, replyID, orgID,
	).Scan(&rep.ID, &rep.OutreachRequestID, &rep.Body, &rep.MediaURLs, &rep.ParsedQuoteCents,
		&rep.ParsedAvailability, &rep.RawSourceRef, &rep.ReceivedAt, &rep.CreatedAt,
		&req.ID, &req.OrgID, &req.WorkOrderID, &req.VendorID, &req.Channel, &req.ToAddress,
		&req.MessageBody, &req.ProviderRef, &req.Status, &req.Error, &req.SentAt, &req.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return &rep, &req, nil
}

// FindOpenRequestByVendorPhone matches an inbound sender to the most recent
// active outreach request whose vendor has that phone (last-10-digit match).
func (r *Repo) FindOpenRequestByVendorPhone(ctx context.Context, orgID int64, phone string) (*Request, error) {
	q := `SELECT ` + reqCols + `
	      FROM vendor_outreach_requests r
	      JOIN vendors v ON v.id = r.vendor_id
	      WHERE r.org_id = $1
	        AND r.status IN ('sent', 'replied')
	        AND (
	          RIGHT(regexp_replace(COALESCE(v.phone,''), '[^0-9]', '', 'g'), 10)
	            = RIGHT(regexp_replace($2, '[^0-9]', '', 'g'), 10)
	          OR RIGHT(regexp_replace(COALESCE(v.alt_phone,''), '[^0-9]', '', 'g'), 10)
	            = RIGHT(regexp_replace($2, '[^0-9]', '', 'g'), 10)
	        )
	      ORDER BY r.sent_at DESC NULLS LAST
	      LIMIT 1`
	req, err := scanRequest(r.db.QueryRow(ctx, q, orgID, phone))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return req, err
}

// FindOpenRequestByVendorEmail is the email-channel equivalent.
func (r *Repo) FindOpenRequestByVendorEmail(ctx context.Context, orgID int64, email string) (*Request, error) {
	q := `SELECT ` + reqCols + `
	      FROM vendor_outreach_requests r
	      JOIN vendors v ON v.id = r.vendor_id
	      WHERE r.org_id = $1
	        AND r.status IN ('sent', 'replied')
	        AND (LOWER(COALESCE(v.primary_email,'')) = LOWER($2)
	             OR LOWER(COALESCE(v.alt_email,'')) = LOWER($2))
	      ORDER BY r.sent_at DESC NULLS LAST
	      LIMIT 1`
	req, err := scanRequest(r.db.QueryRow(ctx, q, orgID, email))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return req, err
}

// FindOpenRequestByWORef matches "WO-<id>" reference tokens in a reply body
// to an active request from that vendor contact for that work order.
func (r *Repo) FindOpenRequestByWORef(ctx context.Context, orgID, workOrderID int64, contact string) (*Request, error) {
	q := `SELECT ` + reqCols + `
	      FROM vendor_outreach_requests r
	      WHERE r.org_id = $1 AND r.work_order_id = $2
	        AND r.status IN ('sent', 'replied')
	        AND (RIGHT(regexp_replace(r.to_address, '[^0-9]', '', 'g'), 10)
	               = RIGHT(regexp_replace($3, '[^0-9]', '', 'g'), 10)
	             OR LOWER(r.to_address) = LOWER($3))
	      ORDER BY r.sent_at DESC NULLS LAST
	      LIMIT 1`
	req, err := scanRequest(r.db.QueryRow(ctx, q, orgID, workOrderID, contact))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return req, err
}

func (r *Repo) InsertReply(ctx context.Context, rep *Reply) (*Reply, error) {
	media := rep.MediaURLs
	if len(media) == 0 {
		media = []byte(`[]`)
	}
	err := r.db.QueryRow(ctx, `
		INSERT INTO vendor_outreach_replies
			(outreach_request_id, body, media_urls, parsed_quote_cents, parsed_availability, raw_source_ref)
		VALUES ($1, $2, $3::jsonb, $4, $5, $6)
		RETURNING id, received_at, created_at`,
		rep.OutreachRequestID, rep.Body, string(media), rep.ParsedQuoteCents,
		rep.ParsedAvailability, rep.RawSourceRef,
	).Scan(&rep.ID, &rep.ReceivedAt, &rep.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert outreach reply: %w", err)
	}
	return rep, nil
}

func (r *Repo) UpdateReplyParse(ctx context.Context, replyID int64, quoteCents *int64, availability *string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE vendor_outreach_replies
		SET parsed_quote_cents = $1, parsed_availability = $2
		WHERE id = $3`, quoteCents, availability, replyID)
	return err
}
