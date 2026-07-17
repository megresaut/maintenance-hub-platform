package tasks

import "time"

// =========================
// ENUMS (match DB constraints)
// =========================

type TaskStatus string

const (
	TaskStatusOpen          TaskStatus = "open"
	TaskStatusInProgress    TaskStatus = "in_progress"
	TaskStatusPendingVendor TaskStatus = "pending_vendor"
	TaskStatusScheduled     TaskStatus = "scheduled"
	TaskStatusCompleted     TaskStatus = "completed"
	TaskStatusClosed        TaskStatus = "closed"
	TaskStatusCancelled     TaskStatus = "cancelled"
	TaskStatusDeferred      TaskStatus = "deferred"
)

func ValidStatus(s TaskStatus) bool {
	switch s {
	case TaskStatusOpen, TaskStatusInProgress, TaskStatusPendingVendor,
		TaskStatusScheduled, TaskStatusCompleted, TaskStatusClosed,
		TaskStatusCancelled, TaskStatusDeferred:
		return true
	}
	return false
}

type TaskPriority string

const (
	PriorityLow    TaskPriority = "low"
	PriorityMedium TaskPriority = "medium"
	PriorityHigh   TaskPriority = "high"
	PriorityUrgent TaskPriority = "urgent"
)

func ValidPriority(p TaskPriority) bool {
	switch p {
	case PriorityLow, PriorityMedium, PriorityHigh, PriorityUrgent:
		return true
	}
	return false
}

// =========================
// DB MODEL
// =========================

// TaskAssignee is an org_users row projected onto a task.
type TaskAssignee struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role,omitempty"`
}

type Task struct {
	ID              int64        `json:"id"`
	OrgID           int64        `json:"org_id"`
	PropertyID      int64        `json:"property_id"`
	Name            string       `json:"name"`
	Description     string       `json:"description"`
	Status          TaskStatus   `json:"status"`
	Priority        TaskPriority `json:"priority"`
	PropertyName    string       `json:"property_name"`
	PropertyAddress string       `json:"property_address"`
	// category is free text (no enum constraint); managed via task_categories
	Category string `json:"category"`

	DueDate       *time.Time `json:"due_date"`
	DateRequested time.Time  `json:"date_requested"`
	LastUpdated   time.Time  `json:"last_updated"`

	RequestedBy   *int64         `json:"requested_by"` // nullable
	Assignees     []TaskAssignee `json:"assignees"`
	Collaborators []TaskAssignee `json:"collaborators"`

	IsVisibleToClient  bool `json:"is_visible_to_client"`
	IsEditableByClient bool `json:"is_editable_by_client"`

	Tags []string `json:"tags"`

	ClosedBy   *int64     `json:"closed_by"`
	ClosedAt   *time.Time `json:"closed_at"`
	ReopenedAt *time.Time `json:"reopened_at"`

	IsTemplate bool   `json:"is_template"`
	UnitID     *int64 `json:"unit_id,omitempty"`
	LocationID *int64 `json:"location_id,omitempty"`
}

// =========================
// CREATE DTO
// =========================

type CreateTaskRequest struct {
	PropertyID    int64        `json:"property_id"`
	Name          string       `json:"name"`
	Description   string       `json:"description"`
	Priority      TaskPriority `json:"priority"`
	Category      string       `json:"category"`
	DueDate       *time.Time   `json:"due_date,omitempty"`
	RequestedBy   *int64       `json:"requested_by,omitempty"`
	Assignees     []int64      `json:"assignees,omitempty"`
	Tags          []string     `json:"tags,omitempty"`
	Collaborators []int64      `json:"collaborators,omitempty"`
	UnitID        *int64       `json:"unit_id,omitempty"`
	LocationID    *int64       `json:"location_id,omitempty"`
	Status        TaskStatus   `json:"status"`
}

// =========================
// UPDATE DTO
// =========================

type UpdateTaskRequest struct {
	Name        *string       `json:"name,omitempty"`
	Description *string       `json:"description,omitempty"`
	Status      *TaskStatus   `json:"status,omitempty"`
	Priority    *TaskPriority `json:"priority,omitempty"`
	Category    *string       `json:"category,omitempty"` // free text
	DueDate     *time.Time    `json:"due_date,omitempty"`
	Assignees   []int64       `json:"assignees,omitempty"`
	Tags        []string      `json:"tags,omitempty"`
}

// =========================
// LIST FILTERS (GET /tasks)
// =========================

type TaskListFilters struct {
	PropertyID *int64
	Status     *TaskStatus
	Assignee   *int64
	Limit      int
}

type CancelTaskRequest struct {
	Message string `json:"message,omitempty"`
}
