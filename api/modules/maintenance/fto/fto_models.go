package fto

import "time"

// FTO status values (field_team_orders_status_check). The source's
// submissions/billing surface (hours, expenses, amount_cents, bill drafts,
// checklists) is stripped in this port.
const (
	FTOStatusNew        = "new"
	FTOStatusScheduled  = "scheduled"
	FTOStatusDispatched = "dispatched"
	FTOStatusInProgress = "in_progress"
	FTOStatusCompleted  = "completed"
	FTOStatusCancelled  = "cancelled"
)

func ValidStatus(s string) bool {
	switch s {
	case FTOStatusNew, FTOStatusScheduled, FTOStatusDispatched,
		FTOStatusInProgress, FTOStatusCompleted, FTOStatusCancelled:
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

// ======================================================
// Core FTO Model
// ======================================================

type FieldTeamOrder struct {
	ID              int64   `json:"id"`
	OrgID           int64   `json:"org_id"`
	Name            string  `json:"name"`
	Description     *string `json:"description,omitempty"`
	PropertyID      *int64  `json:"property_id,omitempty"`
	PropertyName    *string `json:"property_name,omitempty"`
	PropertyAddress *string `json:"property_address,omitempty"`
	TaskID          *int64  `json:"task_id,omitempty"`
	UnitID          *int64  `json:"unit_id,omitempty"`
	LocationID      *int64  `json:"location_id,omitempty"`

	// Internal assignees: org_users ids.
	FieldTeamMemberIDs []int64 `json:"field_team_member_ids"`
	PMAssigneeIDs      []int64 `json:"pm_assignee_ids"`

	Status   string     `json:"status"`
	Priority string     `json:"priority"`
	DueDate  *time.Time `json:"due_date,omitempty"`
	// EventStartAt/EventEndAt are the calendar event time slot. When set,
	// used instead of due_date.
	EventStartAt *time.Time `json:"event_start_at,omitempty"`
	EventEndAt   *time.Time `json:"event_end_at,omitempty"`

	PMNotes           *string `json:"pm_notes,omitempty"`
	ResolutionSummary *string `json:"resolution_summary,omitempty"`
	IssueResolved     *bool   `json:"issue_resolved,omitempty"`

	ApprovalStatus string `json:"approval_status"`
	FormStatus     string `json:"form_submission_status"`

	DispatchedAt *time.Time `json:"dispatched_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	CompletedBy  *int64     `json:"completed_by,omitempty"`

	CreatedBy     *int64  `json:"created_by,omitempty"`
	CreatedByName *string `json:"created_by_name,omitempty"`

	IsVisibleToClient bool `json:"is_visible_to_client"`

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`

	TaskName *string `json:"task_name,omitempty"`

	Source string `json:"source,omitempty"`
}

// ======================================================
// CREATE DTO — used by PM to create a new FTO
// ======================================================

type CreateFTODTO struct {
	Name               string  `json:"name"`
	Description        *string `json:"description,omitempty"`
	PropertyID         *int64  `json:"property_id,omitempty"`
	TaskID             *int64  `json:"task_id,omitempty"`
	FieldTeamMemberIDs []int64 `json:"field_team_member_ids"`
	PMAssigneeIDs      []int64 `json:"pm_assignee_ids"`
	Priority           string  `json:"priority"`
	DueDate            *string `json:"due_date,omitempty"`
	EventStartAt       *string `json:"event_start_at,omitempty"`
	EventEndAt         *string `json:"event_end_at,omitempty"`
	UnitID             *int64  `json:"unit_id,omitempty"`
	LocationID         *int64  `json:"location_id,omitempty"`
	CreatedBy          *int64  `json:"created_by,omitempty"`
	Source             *string `json:"source,omitempty"`
}

// ======================================================
// UPDATE DTO — PM updating schedule, fields, notes, etc.
// ======================================================

type UpdateFTODTO struct {
	Name               *string  `json:"name,omitempty"`
	Description        *string  `json:"description,omitempty"`
	Status             *string  `json:"status,omitempty"`
	FieldTeamMemberIDs *[]int64 `json:"field_team_member_ids,omitempty"`
	PMAssigneeIDs      *[]int64 `json:"pm_assignee_ids,omitempty"`
	Priority           *string  `json:"priority,omitempty"`
	DueDate            *string  `json:"due_date,omitempty"`
	EventStartAt       *string  `json:"event_start_at,omitempty"`
	EventEndAt         *string  `json:"event_end_at,omitempty"`
	PMNotes            *string  `json:"pm_notes,omitempty"`
	PropertyID         *int64   `json:"property_id,omitempty"`
	UnitID             *int64   `json:"unit_id,omitempty"`
	LocationID         *int64   `json:"location_id,omitempty"`
	TaskID             *int64   `json:"task_id,omitempty"`
}

// ======================================================
// COMPLETION DTO — tech completes job or PM completes manually
// ======================================================

type CompleteFTODTO struct {
	FieldTeamMemberID *int64  `json:"field_team_member_id,omitempty"`
	ResolutionSummary *string `json:"resolution_summary,omitempty"`
	IssueResolved     *bool   `json:"issue_resolved,omitempty"`
	FormStatus        *string `json:"form_submission_status,omitempty"` // "none" | "partial" | "complete"
}
