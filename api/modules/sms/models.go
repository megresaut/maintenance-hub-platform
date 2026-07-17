package sms

import (
	"encoding/json"
	"time"
)

type Conversation struct {
	ID               int64      `json:"id"`
	OrgID            int64      `json:"org_id"`
	PhoneNumber      string     `json:"phone_number"`
	TwilioSID        *string    `json:"twilio_sid,omitempty"`
	Status           string     `json:"status"`
	LastMessageAt    *time.Time `json:"last_message_at,omitempty"`
	LastClassifiedAt *time.Time `json:"last_classified_at,omitempty"`
	IsGroup          bool       `json:"is_group"`
	GroupName        *string    `json:"group_name,omitempty"`
	Participants     []string   `json:"participants,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type Message struct {
	ID             int64           `json:"id"`
	ConversationID int64           `json:"conversation_id"`
	TwilioSID      *string         `json:"twilio_sid,omitempty"`
	Direction      string          `json:"direction"` // inbound | outbound
	FromNumber     string          `json:"from_number"`
	ToNumber       string          `json:"to_number"`
	Body           string          `json:"body"`
	MediaURLs      json.RawMessage `json:"media_urls"`
	Processed      bool            `json:"processed"`
	CreatedAt      time.Time       `json:"created_at"`
}
