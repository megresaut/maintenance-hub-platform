package drafts

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

const draftCols = `d.id, d.org_id, d.source, d.source_ref, d.entity_type, d.extracted_data,
	d.ai_confidence, d.ai_reasoning, d.ai_model, d.raw_ai_response,
	d.status, d.parent_draft_id, d.reviewed_by, d.reviewed_at,
	d.created_entity_id, d.created_at, d.updated_at`

func scanDraft(row pgx.Row) (*AIDraft, error) {
	var d AIDraft
	err := row.Scan(
		&d.ID, &d.OrgID, &d.Source, &d.SourceRef, &d.EntityType, &d.ExtractedData,
		&d.AIConfidence, &d.AIReasoning, &d.AIModel, &d.RawAIResponse,
		&d.Status, &d.ParentDraftID, &d.ReviewedBy, &d.ReviewedAt,
		&d.CreatedEntityID, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *Repo) Create(ctx context.Context, params CreateDraftParams) (*AIDraft, error) {
	q := `INSERT INTO ai_drafts (org_id, source, source_ref, entity_type, extracted_data,
	        ai_confidence, ai_reasoning, ai_model, raw_ai_response, parent_draft_id)
	      VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9::jsonb, $10)
	      RETURNING ` + strippedCols()

	d, err := scanDraft(r.db.QueryRow(ctx, q,
		params.OrgID, params.Source, params.SourceRef, params.EntityType,
		string(params.ExtractedData), params.AIConfidence, params.AIReasoning, params.AIModel,
		nullableJSON(params.RawAIResponse), params.ParentDraftID,
	))
	if err != nil {
		return nil, fmt.Errorf("create draft: %w", err)
	}
	return d, nil
}

func (r *Repo) GetByID(ctx context.Context, orgID, id int64) (*AIDraft, error) {
	q := `SELECT ` + draftCols + `, u.name
	      FROM ai_drafts d
	      LEFT JOIN org_users u ON u.id = d.reviewed_by
	      WHERE d.id = $1 AND d.org_id = $2`

	var d AIDraft
	err := r.db.QueryRow(ctx, q, id, orgID).Scan(
		&d.ID, &d.OrgID, &d.Source, &d.SourceRef, &d.EntityType, &d.ExtractedData,
		&d.AIConfidence, &d.AIReasoning, &d.AIModel, &d.RawAIResponse,
		&d.Status, &d.ParentDraftID, &d.ReviewedBy, &d.ReviewedAt,
		&d.CreatedEntityID, &d.CreatedAt, &d.UpdatedAt,
		&d.ReviewedByName,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get draft by id: %w", err)
	}
	return &d, nil
}

func (r *Repo) ListByStatus(ctx context.Context, orgID int64, status string, limit int) ([]*AIDraft, error) {
	q := `SELECT ` + draftCols + `
	      FROM ai_drafts d
	      WHERE d.org_id = $1 AND ($2 = '' OR d.status = $2)
	      ORDER BY d.created_at DESC
	      LIMIT $3`

	rows, err := r.db.Query(ctx, q, orgID, status, limit)
	if err != nil {
		return nil, fmt.Errorf("list drafts: %w", err)
	}
	defer rows.Close()

	result := make([]*AIDraft, 0)
	for rows.Next() {
		d, err := scanDraft(rows)
		if err != nil {
			return nil, fmt.Errorf("scan draft: %w", err)
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (r *Repo) UpdateStatus(ctx context.Context, orgID, id int64, status DraftStatus, reviewedBy *int64) error {
	now := time.Now()
	_, err := r.db.Exec(ctx,
		`UPDATE ai_drafts SET status = $1, reviewed_by = $2, reviewed_at = $3, updated_at = now()
		 WHERE id = $4 AND org_id = $5`,
		string(status), reviewedBy, &now, id, orgID)
	if err != nil {
		return fmt.Errorf("update draft status: %w", err)
	}
	return nil
}

func (r *Repo) SetCreatedEntityID(ctx context.Context, orgID, id, entityID int64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE ai_drafts SET created_entity_id = $1, updated_at = now() WHERE id = $2 AND org_id = $3`,
		entityID, id, orgID)
	if err != nil {
		return fmt.Errorf("set created entity id: %w", err)
	}
	return nil
}

func (r *Repo) GetChildDrafts(ctx context.Context, orgID, parentID int64) ([]*AIDraft, error) {
	q := `SELECT ` + draftCols + `
	      FROM ai_drafts d
	      WHERE d.parent_draft_id = $1 AND d.org_id = $2
	      ORDER BY d.created_at`

	rows, err := r.db.Query(ctx, q, parentID, orgID)
	if err != nil {
		return nil, fmt.Errorf("get child drafts: %w", err)
	}
	defer rows.Close()

	result := make([]*AIDraft, 0)
	for rows.Next() {
		d, err := scanDraft(rows)
		if err != nil {
			return nil, fmt.Errorf("scan child draft: %w", err)
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (r *Repo) CountPending(ctx context.Context, orgID int64) (int, error) {
	var count int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM ai_drafts WHERE org_id = $1 AND status = 'pending'`, orgID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count pending drafts: %w", err)
	}
	return count, nil
}

func (r *Repo) GetBySourceRef(ctx context.Context, orgID int64, source, sourceRef string) (*AIDraft, error) {
	q := `SELECT ` + draftCols + `
	      FROM ai_drafts d
	      WHERE d.org_id = $1 AND d.source = $2 AND d.source_ref = $3
	      ORDER BY d.created_at DESC
	      LIMIT 1`

	d, err := scanDraft(r.db.QueryRow(ctx, q, orgID, source, sourceRef))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get draft by source ref: %w", err)
	}
	return d, nil
}

// strippedCols is draftCols without the "d." qualifiers, for RETURNING clauses.
func strippedCols() string {
	return `id, org_id, source, source_ref, entity_type, extracted_data,
	ai_confidence, ai_reasoning, ai_model, raw_ai_response,
	status, parent_draft_id, reviewed_by, reviewed_at,
	created_entity_id, created_at, updated_at`
}

func nullableJSON(data []byte) interface{} {
	if len(data) == 0 {
		return nil
	}
	return string(data)
}
