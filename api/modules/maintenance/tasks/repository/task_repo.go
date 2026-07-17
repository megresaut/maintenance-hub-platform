package repository

import (
	"context"
	"errors"

	"maintenancehub/modules/maintenance/tasks"
)

var ErrNotFound = errors.New("task not found")

// TaskRepository defines all DB operations for tasks. Every method is
// org-scoped: rows outside orgID are invisible.
type TaskRepository interface {
	CreateTask(ctx context.Context, orgID int64, req tasks.CreateTaskRequest) (*tasks.Task, error)
	GetTaskByID(ctx context.Context, orgID, id int64) (*tasks.Task, error)
	ListTasks(ctx context.Context, orgID int64, filters tasks.TaskListFilters) ([]*tasks.Task, error)
	GetTasksByProperty(ctx context.Context, orgID, propertyID int64) ([]*tasks.Task, error)
	UpdateTask(ctx context.Context, orgID, id int64, req tasks.UpdateTaskRequest) (*tasks.Task, error)

	// Close / reopen lifecycle
	MarkClosed(ctx context.Context, orgID, taskID, closedBy int64) error
	MarkReopened(ctx context.Context, orgID, taskID int64) error

	// Cancel (status transition)
	CancelTask(ctx context.Context, orgID, taskID int64) (*tasks.Task, error)

	// Soft delete
	SoftDeleteTask(ctx context.Context, orgID, taskID int64) error
}
