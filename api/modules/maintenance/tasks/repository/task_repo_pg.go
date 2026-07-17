package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"maintenancehub/modules/maintenance/tasks"
)

type TaskRepositoryPG struct {
	db *pgxpool.Pool
}

func NewTaskRepositoryPG(db *pgxpool.Pool) *TaskRepositoryPG {
	return &TaskRepositoryPG{db: db}
}

// taskColumns is the shared SELECT list; assignees/collaborators are scanned
// as raw id arrays and enriched from org_users afterwards.
const taskColumns = `
	t.id, t.org_id, t.property_id, t.name, t.description, t.status, t.priority,
	t.category, t.due_date, t.date_requested, t.last_updated, t.requested_by,
	t.assignees, t.collaborators, t.tags,
	t.is_visible_to_client, t.is_editable_by_client, t.is_template,
	t.unit_id, t.location_id, t.closed_by, t.closed_at, t.reopened_at,
	COALESCE(p.name, ''), COALESCE(p.address, '')`

// taskRow is the pre-enrichment scan target.
type taskRow struct {
	task          tasks.Task
	assigneeIDs   []int64
	collaboratorIDs []int64
}

func scanTaskRow(row pgx.Row) (*taskRow, error) {
	var tr taskRow
	t := &tr.task
	err := row.Scan(
		&t.ID, &t.OrgID, &t.PropertyID, &t.Name, &t.Description, &t.Status, &t.Priority,
		&t.Category, &t.DueDate, &t.DateRequested, &t.LastUpdated, &t.RequestedBy,
		&tr.assigneeIDs, &tr.collaboratorIDs, &t.Tags,
		&t.IsVisibleToClient, &t.IsEditableByClient, &t.IsTemplate,
		&t.UnitID, &t.LocationID, &t.ClosedBy, &t.ClosedAt, &t.ReopenedAt,
		&t.PropertyName, &t.PropertyAddress,
	)
	if err != nil {
		return nil, err
	}
	return &tr, nil
}

// enrich resolves assignee/collaborator ids into TaskAssignee records from
// org_users, org-scoped.
func (r *TaskRepositoryPG) enrich(ctx context.Context, orgID int64, rows []*taskRow) ([]*tasks.Task, error) {
	idset := map[int64]struct{}{}
	for _, tr := range rows {
		for _, id := range tr.assigneeIDs {
			idset[id] = struct{}{}
		}
		for _, id := range tr.collaboratorIDs {
			idset[id] = struct{}{}
		}
	}
	users := map[int64]tasks.TaskAssignee{}
	if len(idset) > 0 {
		ids := make([]int64, 0, len(idset))
		for id := range idset {
			ids = append(ids, id)
		}
		urows, err := r.db.Query(ctx, `
			SELECT id, name, email, role FROM org_users
			WHERE org_id = $1 AND id = ANY($2)`, orgID, ids)
		if err != nil {
			return nil, fmt.Errorf("load assignees: %w", err)
		}
		defer urows.Close()
		for urows.Next() {
			var u tasks.TaskAssignee
			if err := urows.Scan(&u.ID, &u.Name, &u.Email, &u.Role); err != nil {
				return nil, err
			}
			users[u.ID] = u
		}
		if err := urows.Err(); err != nil {
			return nil, err
		}
	}
	project := func(ids []int64) []tasks.TaskAssignee {
		out := []tasks.TaskAssignee{}
		for _, id := range ids {
			if u, ok := users[id]; ok {
				out = append(out, u)
			} else {
				out = append(out, tasks.TaskAssignee{ID: id})
			}
		}
		return out
	}
	out := make([]*tasks.Task, 0, len(rows))
	for _, tr := range rows {
		tr.task.Assignees = project(tr.assigneeIDs)
		tr.task.Collaborators = project(tr.collaboratorIDs)
		if tr.task.Tags == nil {
			tr.task.Tags = []string{}
		}
		out = append(out, &tr.task)
	}
	return out, nil
}

func (r *TaskRepositoryPG) one(ctx context.Context, orgID int64, row pgx.Row) (*tasks.Task, error) {
	tr, err := scanTaskRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	list, err := r.enrich(ctx, orgID, []*taskRow{tr})
	if err != nil {
		return nil, err
	}
	return list[0], nil
}

// ===== CREATE =====

func (r *TaskRepositoryPG) CreateTask(ctx context.Context, orgID int64, req tasks.CreateTaskRequest) (*tasks.Task, error) {
	if req.Assignees == nil {
		req.Assignees = []int64{}
	}
	if req.Collaborators == nil {
		req.Collaborators = []int64{}
	}
	if req.Tags == nil {
		req.Tags = []string{}
	}
	row := r.db.QueryRow(ctx, `
		WITH new AS (
			INSERT INTO tasks (
				org_id, property_id, name, description, status, priority, category,
				due_date, requested_by, assignees, collaborators, tags,
				unit_id, location_id
			)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
			RETURNING *
		)
		SELECT `+taskColumns+`
		FROM new t
		LEFT JOIN properties p ON p.id = t.property_id AND p.org_id = t.org_id`,
		orgID, req.PropertyID, req.Name, req.Description, req.Status, req.Priority,
		req.Category, req.DueDate, req.RequestedBy, req.Assignees, req.Collaborators,
		req.Tags, req.UnitID, req.LocationID,
	)
	return r.one(ctx, orgID, row)
}

// ===== GET ONE =====

func (r *TaskRepositoryPG) GetTaskByID(ctx context.Context, orgID, id int64) (*tasks.Task, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+taskColumns+`
		FROM tasks t
		LEFT JOIN properties p ON p.id = t.property_id AND p.org_id = t.org_id
		WHERE t.id = $1 AND t.org_id = $2 AND t.deleted_at IS NULL`, id, orgID)
	return r.one(ctx, orgID, row)
}

// ===== LIST / FILTER =====

func (r *TaskRepositoryPG) ListTasks(ctx context.Context, orgID int64, f tasks.TaskListFilters) ([]*tasks.Task, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
		SELECT `+taskColumns+`
		FROM tasks t
		LEFT JOIN properties p ON p.id = t.property_id AND p.org_id = t.org_id
		WHERE t.org_id = $1
		  AND t.deleted_at IS NULL
		  AND t.is_template = false
		  AND ($2::bigint IS NULL OR t.property_id = $2)
		  AND ($3::text IS NULL OR t.status = $3)
		  AND ($4::bigint IS NULL OR $4 = ANY(t.assignees))
		ORDER BY t.date_requested DESC
		LIMIT $5`,
		orgID, f.PropertyID, statusPtr(f.Status), f.Assignee, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trs []*taskRow
	for rows.Next() {
		tr, err := scanTaskRow(rows)
		if err != nil {
			return nil, err
		}
		trs = append(trs, tr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return r.enrich(ctx, orgID, trs)
}

func (r *TaskRepositoryPG) GetTasksByProperty(ctx context.Context, orgID, propertyID int64) ([]*tasks.Task, error) {
	return r.ListTasks(ctx, orgID, tasks.TaskListFilters{PropertyID: &propertyID, Limit: 500})
}

func statusPtr(s *tasks.TaskStatus) *string {
	if s == nil {
		return nil
	}
	v := string(*s)
	return &v
}

// ===== UPDATE (PATCH) =====

func (r *TaskRepositoryPG) UpdateTask(ctx context.Context, orgID, id int64, req tasks.UpdateTaskRequest) (*tasks.Task, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE tasks SET
			name          = COALESCE($1, name),
			description   = COALESCE($2, description),
			status        = COALESCE($3, status),
			priority      = COALESCE($4, priority),
			category      = COALESCE($5, category),
			due_date      = COALESCE($6, due_date),
			assignees     = COALESCE($7::bigint[], assignees),
			tags          = COALESCE($8::text[], tags),
			last_updated  = now()
		WHERE id = $9 AND org_id = $10 AND deleted_at IS NULL`,
		req.Name, req.Description, statusPtr(req.Status), priorityPtr(req.Priority),
		req.Category, req.DueDate, req.Assignees, req.Tags, id, orgID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return r.GetTaskByID(ctx, orgID, id)
}

func priorityPtr(p *tasks.TaskPriority) *string {
	if p == nil {
		return nil
	}
	v := string(*p)
	return &v
}

// ===== CLOSE / REOPEN =====

func (r *TaskRepositoryPG) MarkClosed(ctx context.Context, orgID, taskID, closedBy int64) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE tasks
		SET status = 'closed', closed_by = $1, closed_at = now(), last_updated = now()
		WHERE id = $2 AND org_id = $3 AND deleted_at IS NULL`,
		closedBy, taskID, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *TaskRepositoryPG) MarkReopened(ctx context.Context, orgID, taskID int64) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE tasks
		SET status = 'open', closed_by = NULL, closed_at = NULL,
		    reopened_at = now(), last_updated = now()
		WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL`,
		taskID, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ===== CANCEL =====

func (r *TaskRepositoryPG) CancelTask(ctx context.Context, orgID, taskID int64) (*tasks.Task, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE tasks SET status = 'cancelled', last_updated = now()
		WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL`,
		taskID, orgID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return r.GetTaskByID(ctx, orgID, taskID)
}

// ===== SOFT DELETE =====

func (r *TaskRepositoryPG) SoftDeleteTask(ctx context.Context, orgID, taskID int64) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE tasks SET deleted_at = now(), last_updated = now()
		WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL`,
		taskID, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
