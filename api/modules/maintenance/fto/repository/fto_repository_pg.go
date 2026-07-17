package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"maintenancehub/modules/maintenance/fto"
)

type FTORepositoryPG struct {
	db *pgxpool.Pool
}

func NewFTORepositoryPG(db *pgxpool.Pool) *FTORepositoryPG {
	return &FTORepositoryPG{db: db}
}

// parseTimePtr converts an RFC3339 (or date-only) string pointer to a
// *time.Time; invalid/empty strings become nil (faithful to source
// behaviour of tolerant date parsing on create/update DTOs).
func parseTimePtr(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, *s); err == nil {
			return &t
		}
	}
	return nil
}

const ftoColumns = `
	f.id, f.org_id, f.name, f.description,
	f.property_id, p.name AS property_name, p.address AS property_address,
	f.task_id, t.name AS task_name,
	f.unit_id, f.location_id,
	f.field_team_member_ids, f.pm_assignee_ids,
	f.status, f.priority, f.due_date, f.event_start_at, f.event_end_at,
	f.pm_notes, f.resolution_summary, f.issue_resolved,
	f.approval_status, f.form_submission_status,
	f.dispatched_at, f.completed_at, f.completed_by,
	f.created_by, u.name AS created_by_name,
	f.is_visible_to_client, f.source, f.created_at, f.updated_at`

const ftoJoins = `
	LEFT JOIN properties p ON p.id = f.property_id AND p.org_id = f.org_id
	LEFT JOIN tasks t ON t.id = f.task_id AND t.org_id = f.org_id
	LEFT JOIN org_users u ON u.id = f.created_by AND u.org_id = f.org_id`

func scanFTO(row pgx.Row) (*fto.FieldTeamOrder, error) {
	var out fto.FieldTeamOrder
	err := row.Scan(
		&out.ID, &out.OrgID, &out.Name, &out.Description,
		&out.PropertyID, &out.PropertyName, &out.PropertyAddress,
		&out.TaskID, &out.TaskName,
		&out.UnitID, &out.LocationID,
		&out.FieldTeamMemberIDs, &out.PMAssigneeIDs,
		&out.Status, &out.Priority, &out.DueDate, &out.EventStartAt, &out.EventEndAt,
		&out.PMNotes, &out.ResolutionSummary, &out.IssueResolved,
		&out.ApprovalStatus, &out.FormStatus,
		&out.DispatchedAt, &out.CompletedAt, &out.CompletedBy,
		&out.CreatedBy, &out.CreatedByName,
		&out.IsVisibleToClient, &out.Source, &out.CreatedAt, &out.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if out.FieldTeamMemberIDs == nil {
		out.FieldTeamMemberIDs = []int64{}
	}
	if out.PMAssigneeIDs == nil {
		out.PMAssigneeIDs = []int64{}
	}
	return &out, nil
}

// ======================================================
// CREATE
// ======================================================

func (r *FTORepositoryPG) Create(ctx context.Context, orgID int64, dto fto.CreateFTODTO) (*fto.FieldTeamOrder, error) {
	source := "manual"
	if dto.Source != nil && *dto.Source != "" {
		source = *dto.Source
	}
	if dto.FieldTeamMemberIDs == nil {
		dto.FieldTeamMemberIDs = []int64{}
	}
	if dto.PMAssigneeIDs == nil {
		dto.PMAssigneeIDs = []int64{}
	}

	row := r.db.QueryRow(ctx, `
		WITH new AS (
			INSERT INTO field_team_orders (
				org_id, name, description, property_id, task_id,
				field_team_member_ids, pm_assignee_ids,
				priority, due_date, event_start_at, event_end_at,
				unit_id, location_id, created_by, source
			)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
			RETURNING *
		)
		SELECT `+ftoColumns+`
		FROM new f`+ftoJoins,
		orgID, dto.Name, dto.Description, dto.PropertyID, dto.TaskID,
		dto.FieldTeamMemberIDs, dto.PMAssigneeIDs,
		dto.Priority,
		parseTimePtr(dto.DueDate), parseTimePtr(dto.EventStartAt), parseTimePtr(dto.EventEndAt),
		dto.UnitID, dto.LocationID, dto.CreatedBy, source,
	)
	return scanFTO(row)
}

// ======================================================
// GET BY ID
// ======================================================

func (r *FTORepositoryPG) GetByID(ctx context.Context, orgID, id int64) (*fto.FieldTeamOrder, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+ftoColumns+`
		FROM field_team_orders f`+ftoJoins+`
		WHERE f.id = $1 AND f.org_id = $2`, id, orgID)
	return scanFTO(row)
}

// ======================================================
// UPDATE
// ======================================================

func (r *FTORepositoryPG) Update(ctx context.Context, orgID, id int64, dto fto.UpdateFTODTO) (*fto.FieldTeamOrder, error) {
	// task_id semantics: omit = keep, 0 = clear, non-zero = set.
	var taskID *int64
	clearTask := false
	if dto.TaskID != nil {
		if *dto.TaskID == 0 {
			clearTask = true
		} else {
			taskID = dto.TaskID
		}
	}

	var ftIDs, pmIDs []int64
	if dto.FieldTeamMemberIDs != nil {
		ftIDs = *dto.FieldTeamMemberIDs
		if ftIDs == nil {
			ftIDs = []int64{}
		}
	}
	if dto.PMAssigneeIDs != nil {
		pmIDs = *dto.PMAssigneeIDs
		if pmIDs == nil {
			pmIDs = []int64{}
		}
	}

	tag, err := r.db.Exec(ctx, `
		UPDATE field_team_orders f SET
			name                  = COALESCE($1, f.name),
			description           = COALESCE($2, f.description),
			status                = COALESCE($3, f.status),
			field_team_member_ids = COALESCE($4::bigint[], f.field_team_member_ids),
			pm_assignee_ids       = COALESCE($5::bigint[], f.pm_assignee_ids),
			priority              = COALESCE($6, f.priority),
			due_date              = COALESCE($7, f.due_date),
			event_start_at        = COALESCE($8, f.event_start_at),
			event_end_at          = COALESCE($9, f.event_end_at),
			pm_notes              = COALESCE($10, f.pm_notes),
			property_id           = COALESCE($11, f.property_id),
			unit_id               = COALESCE($12, f.unit_id),
			location_id           = COALESCE($13, f.location_id),
			task_id               = CASE WHEN $14::boolean THEN NULL
			                             ELSE COALESCE($15, f.task_id) END,
			dispatched_at         = CASE WHEN $3::text = 'dispatched' AND f.dispatched_at IS NULL
			                             THEN now() ELSE f.dispatched_at END,
			completed_at          = CASE WHEN $3::text = 'completed' AND f.completed_at IS NULL
			                             THEN now() ELSE f.completed_at END,
			updated_at            = now()
		WHERE f.id = $16 AND f.org_id = $17`,
		dto.Name, dto.Description, dto.Status, ftIDs, pmIDs,
		dto.Priority, parseTimePtr(dto.DueDate),
		parseTimePtr(dto.EventStartAt), parseTimePtr(dto.EventEndAt),
		dto.PMNotes, dto.PropertyID, dto.UnitID, dto.LocationID,
		clearTask, taskID, id, orgID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return r.GetByID(ctx, orgID, id)
}

// ======================================================
// DELETE
// ======================================================

func (r *FTORepositoryPG) Delete(ctx context.Context, orgID, id int64) error {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM field_team_orders WHERE id = $1 AND org_id = $2`, id, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ======================================================
// LIST
// ======================================================

func (r *FTORepositoryPG) List(
	ctx context.Context,
	orgID int64,
	taskID *int64,
	propertyID *int64,
	assigneeID *int64,
	fieldTeamMemberID *int64,
	status *string,
	priority *string,
	limit int,
) ([]*fto.FieldTeamOrder, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
		SELECT `+ftoColumns+`
		FROM field_team_orders f`+ftoJoins+`
		WHERE f.org_id = $1
		  AND ($2::bigint IS NULL OR f.task_id = $2)
		  AND ($3::bigint IS NULL OR f.property_id = $3)
		  AND ($4::bigint IS NULL OR $4 = ANY(f.pm_assignee_ids))
		  AND ($5::bigint IS NULL OR $5 = ANY(f.field_team_member_ids))
		  AND ($6::text IS NULL OR f.status = $6)
		  AND ($7::text IS NULL OR f.priority = $7)
		ORDER BY f.created_at DESC
		LIMIT $8`,
		orgID, taskID, propertyID, assigneeID, fieldTeamMemberID, status, priority, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*fto.FieldTeamOrder{}
	for rows.Next() {
		f, err := scanFTO(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ======================================================
// MARK COMPLETED
// ======================================================

func (r *FTORepositoryPG) MarkCompleted(ctx context.Context, orgID, id int64, completedBy int64) (*fto.FieldTeamOrder, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE field_team_orders SET
			status                 = 'completed',
			completed_at           = now(),
			completed_by           = $1,
			form_submission_status = 'complete',
			updated_at             = now()
		WHERE id = $2 AND org_id = $3`,
		completedBy, id, orgID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return r.GetByID(ctx, orgID, id)
}

// ======================================================
// RESOLUTION
// ======================================================

func (r *FTORepositoryPG) SetResolution(ctx context.Context, orgID, id int64, resolved bool, summary *string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE field_team_orders SET
			issue_resolved     = $1,
			resolution_summary = COALESCE($2, resolution_summary),
			updated_at         = now()
		WHERE id = $3 AND org_id = $4`,
		resolved, summary, id, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ======================================================
// TOUCH
// ======================================================

func (r *FTORepositoryPG) Touch(ctx context.Context, orgID, id int64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE field_team_orders SET updated_at = now() WHERE id = $1 AND org_id = $2`, id, orgID)
	return err
}

// ======================================================
// LIST PENDING (form_submission_status = 'none', not terminal)
// ======================================================

func (r *FTORepositoryPG) ListPending(ctx context.Context, orgID int64, fieldTeamMemberID *int64) ([]*fto.FieldTeamOrder, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+ftoColumns+`
		FROM field_team_orders f`+ftoJoins+`
		WHERE f.org_id = $1
		  AND f.form_submission_status = 'none'
		  AND f.status NOT IN ('cancelled', 'completed')
		  AND ($2::bigint IS NULL OR $2 = ANY(f.field_team_member_ids))
		ORDER BY f.due_date ASC NULLS LAST, f.created_at DESC`,
		orgID, fieldTeamMemberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*fto.FieldTeamOrder{}
	for rows.Next() {
		f, err := scanFTO(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ======================================================
// TASK NAME LOOKUP
// ======================================================

func (r *FTORepositoryPG) GetTaskNameByID(ctx context.Context, orgID, taskID int64) (*string, error) {
	var name string
	err := r.db.QueryRow(ctx,
		`SELECT name FROM tasks WHERE id = $1 AND org_id = $2`, taskID, orgID).Scan(&name)
	if err != nil {
		return nil, err
	}
	return &name, nil
}
