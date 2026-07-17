package repository

import (
	"context"
	"errors"

	"maintenancehub/modules/maintenance/fto"
)

var ErrNotFound = errors.New("field team order not found")

// FTORepository defines all DB operations for field team orders. Every
// method is org-scoped. The source's submissions surface (hours/expenses/
// mileage/merge) was stripped.
type FTORepository interface {
	Create(ctx context.Context, orgID int64, dto fto.CreateFTODTO) (*fto.FieldTeamOrder, error)
	GetByID(ctx context.Context, orgID, id int64) (*fto.FieldTeamOrder, error)
	Update(ctx context.Context, orgID, id int64, dto fto.UpdateFTODTO) (*fto.FieldTeamOrder, error)
	Delete(ctx context.Context, orgID, id int64) error

	List(ctx context.Context,
		orgID int64,
		taskID *int64,
		propertyID *int64,
		assigneeID *int64,
		fieldTeamMemberID *int64,
		status *string,
		priority *string,
		limit int,
	) ([]*fto.FieldTeamOrder, error)

	MarkCompleted(ctx context.Context, orgID, id int64, completedBy int64) (*fto.FieldTeamOrder, error)

	// Used by service to set resolution fields
	SetResolution(ctx context.Context, orgID, id int64, resolved bool, summary *string) error

	// Update timestamps (optional helper)
	Touch(ctx context.Context, orgID, id int64) error

	ListPending(ctx context.Context, orgID int64, fieldTeamMemberID *int64) ([]*fto.FieldTeamOrder, error)

	GetTaskNameByID(ctx context.Context, orgID, taskID int64) (*string, error)
}
