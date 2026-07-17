package service

import (
	"context"

	"maintenancehub/modules/maintenance/fto"
)

// FTOService defines business-level operations for field team orders.
// Submission processing (hours/expenses), merge machinery, Outlook sync and
// notification dispatch from the source were stripped.
type FTOService interface {
	Create(ctx context.Context, orgID, actorID int64, dto fto.CreateFTODTO) (*fto.FieldTeamOrder, error)
	GetByID(ctx context.Context, orgID, id int64) (*fto.FieldTeamOrder, error)
	Update(ctx context.Context, orgID, id, actorID int64, dto fto.UpdateFTODTO) (*fto.FieldTeamOrder, error)
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

	SetResolution(ctx context.Context, orgID, id int64, resolved bool, summary *string) error

	ListPending(ctx context.Context, orgID int64, fieldTeamMemberID *int64) ([]*fto.FieldTeamOrder, error)
}
