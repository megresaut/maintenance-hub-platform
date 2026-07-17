package repository

import (
	"context"
	"errors"

	workorder "maintenancehub/modules/maintenance/work_order"
)

var ErrNotFound = errors.New("work order not found")

// WorkOrderRepository defines all DB operations for work orders. Every
// method is org-scoped.
type WorkOrderRepository interface {
	Create(ctx context.Context, orgID int64, dto workorder.CreateWorkOrderDTO) (*workorder.WorkOrder, error)
	GetByID(ctx context.Context, orgID, id int64) (*workorder.WorkOrder, error)
	Update(ctx context.Context, orgID, id int64, dto workorder.UpdateWorkOrderDTO) (*workorder.WorkOrder, error)
	Delete(ctx context.Context, orgID, id int64) error
	List(ctx context.Context, orgID int64, taskID *int64, propertyID *int64, vendorID *int64, status *string, limit int) ([]*workorder.WorkOrder, error)
	MarkCompleted(ctx context.Context, orgID, id int64, completedBy int64, actualCost *float64) (*workorder.WorkOrder, error)

	InsertAttachment(ctx context.Context, orgID int64, a workorder.Attachment) (*workorder.Attachment, error)
	GetWorkOrderAttachments(ctx context.Context, orgID, workOrderID int64) ([]workorder.Attachment, error)
	UpdateWorkOrderAttachment(ctx context.Context, orgID, attachmentID int64, req workorder.UpdateWorkOrderAttachmentRequest) (*workorder.Attachment, error)
	DeleteWorkOrderAttachment(ctx context.Context, orgID, attachmentID int64) error

	GetWorkOrderNotes(ctx context.Context, orgID, workOrderID int64) ([]workorder.WorkOrderNote, error)
	InsertWorkOrderNote(ctx context.Context, orgID int64, n *workorder.WorkOrderNote) error
	UpdateWorkOrderNote(ctx context.Context, orgID, noteID int64, req workorder.UpdateWorkOrderNoteRequest) (*workorder.WorkOrderNote, error)
	DeleteWorkOrderNote(ctx context.Context, orgID, noteID int64) error
}
