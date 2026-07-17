package repository

import (
	"context"

	"maintenancehub/modules/maintenance/taskcategories"
)

type TaskCategoryRepository interface {
	List(ctx context.Context, orgID int64) ([]taskcategories.TaskCategory, error)
	Create(ctx context.Context, orgID int64, name string, createdByUserID *int64) (*taskcategories.TaskCategory, error)
	Rename(ctx context.Context, orgID, id int64, newName string) (*taskcategories.TaskCategory, error)
	Deactivate(ctx context.Context, orgID, id int64) error
}
