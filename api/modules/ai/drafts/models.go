package drafts

import (
	"encoding/json"
	"time"
)

type DraftStatus string

const (
	DraftStatusPending  DraftStatus = "pending"
	DraftStatusApproved DraftStatus = "approved"
	DraftStatusRejected DraftStatus = "rejected"
	DraftStatusExpired  DraftStatus = "expired"
)

type AIDraft struct {
	ID              int64           `json:"id"`
	OrgID           int64           `json:"org_id"`
	Source          string          `json:"source"` // sms | calendar | manual
	SourceRef       *string         `json:"source_ref,omitempty"`
	EntityType      string          `json:"entity_type"` // task | work_order | fto
	ExtractedData   json.RawMessage `json:"extracted_data"`
	AIConfidence    *float64        `json:"ai_confidence,omitempty"`
	AIReasoning     *string         `json:"ai_reasoning,omitempty"`
	AIModel         *string         `json:"ai_model,omitempty"`
	RawAIResponse   json.RawMessage `json:"raw_ai_response,omitempty"`
	Status          DraftStatus     `json:"status"`
	ParentDraftID   *int64          `json:"parent_draft_id,omitempty"`
	ReviewedBy      *int64          `json:"reviewed_by,omitempty"`
	ReviewedAt      *time.Time      `json:"reviewed_at,omitempty"`
	CreatedEntityID *int64          `json:"created_entity_id,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`

	// Joined data (not columns)
	ChildDrafts    []*AIDraft `json:"child_drafts,omitempty"`
	ReviewedByName *string    `json:"reviewed_by_name,omitempty"`
}

type CreateDraftParams struct {
	OrgID         int64
	Source        string
	SourceRef     *string
	EntityType    string
	ExtractedData json.RawMessage
	AIConfidence  *float64
	AIReasoning   *string
	AIModel       *string
	RawAIResponse json.RawMessage
	ParentDraftID *int64
}

type ApproveRequest struct {
	ReviewedBy     int64           `json:"reviewed_by"`
	FieldOverrides json.RawMessage `json:"field_overrides,omitempty"`
	Cascade        bool            `json:"cascade,omitempty"`
}

type RejectRequest struct {
	ReviewedBy int64 `json:"reviewed_by"`
}
