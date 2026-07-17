package recurring

import "time"

type RecurringFrequency string

const (
	FrequencyDaily   RecurringFrequency = "daily"
	FrequencyWeekly  RecurringFrequency = "weekly"
	FrequencyMonthly RecurringFrequency = "monthly"
	FrequencyYearly  RecurringFrequency = "yearly"
)

type RecurringSeries struct {
	ID             int64              `json:"id"`
	OrgID          int64              `json:"org_id"`
	PropertyID     int64              `json:"property_id"`
	UnitID         *int64             `json:"unit_id"`
	LocationID     *int64             `json:"location_id,omitempty"`
	TemplateTaskID *int64             `json:"template_task_id"`
	Name           string             `json:"name"`
	Description    string             `json:"description"`
	Assignees      []int64            `json:"assignees"`
	Collaborators  []int64            `json:"collaborators"`
	Priority       *string            `json:"priority"`
	Category       *string            `json:"category"`
	Frequency      RecurringFrequency `json:"frequency"`
	Interval       int                `json:"interval"`
	ByDay          []string           `json:"by_day"`
	DayOfMonth     *int               `json:"day_of_month"`
	WeekOfMonth    *int               `json:"week_of_month"`
	WeekdayOfMonth *string            `json:"weekday_of_month"`

	StartDate time.Time  `json:"start_date"`
	EndDate   *time.Time `json:"end_date"`

	DueAfterDays int  `json:"due_after_days"`
	Active       bool `json:"active"`

	NextRunAt time.Time  `json:"next_run_at"`
	LastRunAt *time.Time `json:"last_run_at"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DueAfterInput struct {
	Count  int    `json:"count"`
	Period string `json:"period"`
}

type CreateSeriesRequest struct {
	PropertyID int64  `json:"property_id"`
	UnitID     *int64 `json:"unit_id"`
	LocationID *int64 `json:"location_id,omitempty"`

	// template info
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Category      string   `json:"category"`
	Priority      string   `json:"priority"`
	Assignees     []int64  `json:"assignees"`
	Collaborators []int64  `json:"collaborators"`
	Tags          []string `json:"tags"`

	// scheduling
	Frequency      RecurringFrequency `json:"frequency"`
	Interval       int                `json:"interval"`
	ByDay          []string           `json:"by_day"`
	DayOfMonth     *int               `json:"day_of_month"`
	WeekOfMonth    *int               `json:"week_of_month"`
	WeekdayOfMonth *string            `json:"weekday_of_month"`

	StartDate time.Time  `json:"start_date"`
	EndDate   *time.Time `json:"end_date"`

	DueAfter     *DueAfterInput `json:"due_after"`
	DueAfterDays int            `json:"due_after_days"`

	EndMode             string `json:"end_mode"`
	EndAfterOccurrences *int   `json:"end_after_occurrences"`
}

type UpdateSeriesRequest struct {
	Name          *string  `json:"name"`
	Description   *string  `json:"description"`
	Category      *string  `json:"category"`
	Priority      *string  `json:"priority"`
	Assignees     []int64  `json:"assignees"`
	Collaborators []int64  `json:"collaborators"`
	Tags          []string `json:"tags"`

	Frequency      *RecurringFrequency `json:"frequency"`
	Interval       *int                `json:"interval"`
	ByDay          []string            `json:"by_day"`
	DayOfMonth     *int                `json:"day_of_month"`
	WeekOfMonth    *int                `json:"week_of_month"`
	WeekdayOfMonth *string             `json:"weekday_of_month"`

	StartDate *time.Time `json:"start_date"`
	EndDate   *time.Time `json:"end_date"`

	DueAfterDays *int `json:"due_after_days"`
}
