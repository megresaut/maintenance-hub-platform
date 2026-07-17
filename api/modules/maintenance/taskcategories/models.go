package taskcategories

import "time"

// TaskCategory is a user-manageable option for tasks.category (free text,
// not a FK — renaming/deactivating here does not require touching historical
// task rows except an explicit rename cascade).
type TaskCategory struct {
	ID              int64     `json:"id"`
	OrgID           int64     `json:"org_id"`
	Name            string    `json:"name"`
	IsActive        bool      `json:"is_active"`
	CreatedByUserID *int64    `json:"created_by_user_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type CreateTaskCategoryRequest struct {
	Name string `json:"name"`
}

type UpdateTaskCategoryRequest struct {
	Name string `json:"name"`
}
