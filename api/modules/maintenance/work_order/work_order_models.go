package workorder

import "time"

// WorkOrderStatus values mirror the work_orders_status_check DB constraint.
// `dispatched` was added in this port (vendor has been dispatched to the
// property); the rest are faithful to the source.
const (
	WorkOrderStatusNew        = "new"
	WorkOrderStatusSent       = "sent"
	WorkOrderStatusDispatched = "dispatched"
	WorkOrderStatusScheduled  = "scheduled"
	WorkOrderStatusInProgress = "in_progress"
	WorkOrderStatusCompleted  = "completed"
	WorkOrderStatusCancelled  = "cancelled"
	WorkOrderStatusDeferred   = "deferred"
	WorkOrderStatusClosed     = "closed"
)

func ValidStatus(s string) bool {
	switch s {
	case WorkOrderStatusNew, WorkOrderStatusSent, WorkOrderStatusDispatched,
		WorkOrderStatusScheduled, WorkOrderStatusInProgress, WorkOrderStatusCompleted,
		WorkOrderStatusCancelled, WorkOrderStatusDeferred, WorkOrderStatusClosed:
		return true
	}
	return false
}

func ValidPriority(p string) bool {
	switch p {
	case "low", "medium", "high", "urgent":
		return true
	}
	return false
}

// WorkOrder represents a full work order record fetched from the DB.
// All bill/QBO/Buildium/estimate-tax fields from the source were stripped;
// quote_amount_cents is a plain informational quote/estimate.
type WorkOrder struct {
	ID              int64      `json:"id"`
	OrgID           int64      `json:"org_id"`
	TaskID          *int64     `json:"task_id,omitempty"` // nullable linkage
	PropertyID      int64      `json:"property_id"`
	VendorID        *int64     `json:"vendor_id,omitempty"`
	Name            string     `json:"name"`
	WorkDescription string     `json:"description"`
	VendorNotes     *string    `json:"vendor_notes,omitempty"`
	InternalNotes   *string    `json:"internal_notes,omitempty"`
	Status          string     `json:"status"`
	Priority        string     `json:"priority"`
	DueDate         *time.Time `json:"due_date,omitempty"`
	EntryPreference *string    `json:"entry_preference,omitempty"`

	QuoteAmountCents *int64   `json:"quote_amount_cents,omitempty"`
	ActualCost       *float64 `json:"actual_cost,omitempty"`

	EventStartAt *time.Time `json:"event_start_at,omitempty"`
	EventEndAt   *time.Time `json:"event_end_at,omitempty"`
	DispatchedAt *time.Time `json:"dispatched_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	CompletedBy  *int64     `json:"completed_by,omitempty"`
	CreatedBy    *int64     `json:"created_by,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`

	Attachments []Attachment `json:"attachments,omitempty"`

	// Property information (joined from properties table)
	PropertyName *string `json:"property_name,omitempty"`
	// Task information (joined from tasks table)
	TaskName *string `json:"task_name,omitempty"`
}

// CreateWorkOrderDTO is the input for creating a new work order.
type CreateWorkOrderDTO struct {
	TaskID     *int64 `json:"task_id,omitempty"`
	PropertyID int64  `json:"property_id"`
	VendorID   *int64 `json:"vendor_id,omitempty"`

	Name            string `json:"name"`
	WorkDescription string `json:"description"`

	Status   string `json:"status"`
	Priority string `json:"priority"`

	DueDate      *time.Time `json:"due_date,omitempty"`
	EventStartAt *time.Time `json:"event_start_at,omitempty"`
	EventEndAt   *time.Time `json:"event_end_at,omitempty"`

	EntryPreference  *string `json:"entry_preference,omitempty"`
	QuoteAmountCents *int64  `json:"quote_amount_cents,omitempty"`

	RequestedBy *int64                   `json:"requested_by,omitempty"`
	Attachments []WorkOrderAttachmentDTO `json:"attachments,omitempty"`
}

// UpdateWorkOrderDTO is used for PATCH updates.
type UpdateWorkOrderDTO struct {
	VendorID         *int64                   `json:"vendor_id,omitempty"`
	Name             *string                  `json:"name,omitempty"`
	WorkDescription  *string                  `json:"work_description,omitempty"`
	VendorNotes      *string                  `json:"vendor_notes,omitempty"`
	InternalNotes    *string                  `json:"internal_notes,omitempty"`
	Priority         *string                  `json:"priority,omitempty"`
	Status           *string                  `json:"status,omitempty"`
	DueDate          *time.Time               `json:"due_date,omitempty"`
	EventStartAt     *time.Time               `json:"event_start_at,omitempty"`
	EventEndAt       *time.Time               `json:"event_end_at,omitempty"`
	EntryPreference  *string                  `json:"entry_preference,omitempty"`
	QuoteAmountCents *int64                   `json:"quote_amount_cents,omitempty"`
	ActualCost       *float64                 `json:"actual_cost,omitempty"`
	CompletedBy      *int64                   `json:"completed_by,omitempty"`
	UpdatedBy        *int64                   `json:"updated_by,omitempty"`
	Attachments      []WorkOrderAttachmentDTO `json:"attachments,omitempty"`
}

// WorkOrderAttachmentDTO registers attachment metadata created at
// create/update time (upload plumbing was not ported; storage keys are
// opaque references).
type WorkOrderAttachmentDTO struct {
	StoragePath string  `json:"storage_path"`
	FileName    string  `json:"file_name"`
	FileType    string  `json:"file_type"`
	FileSize    int64   `json:"file_size"`
	Label       *string `json:"label,omitempty"`
}

type Attachment struct {
	ID             int64     `json:"id"`
	WorkOrderID    *int64    `json:"work_order_id,omitempty"`
	FileName       string    `json:"file_name"`
	FileType       string    `json:"file_type"`
	FileSize       int64     `json:"file_size"`
	StoragePath    string    `json:"storage_path"`
	UploadedByType string    `json:"uploaded_by_type"`
	UploadedByID   int64     `json:"uploaded_by"`
	Label          *string   `json:"label,omitempty"`
	Description    *string   `json:"description,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type UpdateWorkOrderAttachmentRequest struct {
	FileName    *string `json:"file_name,omitempty"`
	Label       *string `json:"label,omitempty"`
	Description *string `json:"description,omitempty"`
}

type WorkOrderNote struct {
	ID                int64      `json:"id"`
	WorkOrderID       int64      `json:"work_order_id"`
	AuthorType        string     `json:"author_type"`
	AuthorID          int64      `json:"author_id"`
	Body              string     `json:"body"`
	IsVisibleToClient bool       `json:"is_visible_to_client"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         *time.Time `json:"updated_at,omitempty"`
}

type CreateWorkOrderNoteRequest struct {
	AuthorType        string `json:"author_type"`
	AuthorID          int64  `json:"author_id"`
	Body              string `json:"body"`
	IsVisibleToClient bool   `json:"is_visible_to_client"`
}

type UpdateWorkOrderNoteRequest struct {
	Body              string `json:"body"`
	IsVisibleToClient bool   `json:"is_visible_to_client"`
}
