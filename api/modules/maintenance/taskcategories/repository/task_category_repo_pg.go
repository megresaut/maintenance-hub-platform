package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"maintenancehub/modules/maintenance/taskcategories"
)

var ErrNameConflict = errors.New("a category with this name already exists")
var ErrNotFound = errors.New("task category not found")

type TaskCategoryRepositoryPG struct {
	db *pgxpool.Pool
}

func NewTaskCategoryRepositoryPG(db *pgxpool.Pool) *TaskCategoryRepositoryPG {
	return &TaskCategoryRepositoryPG{db: db}
}

func scanTaskCategory(row pgx.Row) (*taskcategories.TaskCategory, error) {
	var c taskcategories.TaskCategory
	if err := row.Scan(&c.ID, &c.OrgID, &c.Name, &c.IsActive, &c.CreatedByUserID, &c.CreatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *TaskCategoryRepositoryPG) List(ctx context.Context, orgID int64) ([]taskcategories.TaskCategory, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, org_id, name, is_active, created_by_user_id, created_at
		FROM task_categories
		WHERE org_id = $1 AND is_active = true
		ORDER BY name ASC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []taskcategories.TaskCategory{}
	for rows.Next() {
		c, err := scanTaskCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (r *TaskCategoryRepositoryPG) Create(ctx context.Context, orgID int64, name string, createdByUserID *int64) (*taskcategories.TaskCategory, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO task_categories (org_id, name, created_by_user_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (org_id, lower(name)) DO UPDATE SET is_active = true
		RETURNING id, org_id, name, is_active, created_by_user_id, created_at
	`, orgID, name, createdByUserID)
	return scanTaskCategory(row)
}

func (r *TaskCategoryRepositoryPG) Rename(ctx context.Context, orgID, id int64, newName string) (*taskcategories.TaskCategory, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var oldName string
	err = tx.QueryRow(ctx, `SELECT name FROM task_categories WHERE id = $1 AND org_id = $2`, id, orgID).Scan(&oldName)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	var conflictID int64
	err = tx.QueryRow(ctx, `
		SELECT id FROM task_categories
		WHERE org_id = $1 AND lower(name) = lower($2) AND id <> $3
	`, orgID, newName, id).Scan(&conflictID)
	if err == nil {
		return nil, ErrNameConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `UPDATE task_categories SET name = $1 WHERE id = $2 AND org_id = $3`, newName, id, orgID); err != nil {
		return nil, err
	}

	// rename cascade onto historical tasks (org-scoped)
	if _, err := tx.Exec(ctx, `UPDATE tasks SET category = $1 WHERE category = $2 AND org_id = $3`, newName, oldName, orgID); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx, `
		SELECT id, org_id, name, is_active, created_by_user_id, created_at
		FROM task_categories WHERE id = $1 AND org_id = $2
	`, id, orgID)
	c, err := scanTaskCategory(row)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

func (r *TaskCategoryRepositoryPG) Deactivate(ctx context.Context, orgID, id int64) error {
	tag, err := r.db.Exec(ctx, `UPDATE task_categories SET is_active = false WHERE id = $1 AND org_id = $2`, id, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
