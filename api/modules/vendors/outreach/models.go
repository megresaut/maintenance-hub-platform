package outreach

import (
	"encoding/json"
	"time"
)

type RequestStatus string

const (
	StatusPending  RequestStatus = "pending"
	StatusSent     RequestStatus = "sent"
	StatusFailed   RequestStatus = "failed"
	StatusReplied  RequestStatus = "replied"
	StatusSelected RequestStatus = "selected"
)

// Request is one outreach message sent (or attempted) to one vendor for one
// work order.
type Request struct {
	ID          int64         `json:"id"`
	OrgID       int64         `json:"org_id"`
	WorkOrderID int64         `json:"work_order_id"`
	VendorID    int64         `json:"vendor_id"`
	Channel     string        `json:"channel"` // sms | email
	ToAddress   string        `json:"to_address"`
	MessageBody string        `json:"message_body"`
	ProviderRef *string       `json:"provider_ref,omitempty"`
	Status      RequestStatus `json:"status"`
	Error       *string       `json:"error,omitempty"`
	SentAt      *time.Time    `json:"sent_at,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`

	// Joined
	VendorName  string   `json:"vendor_name,omitempty"`
	VendorPhone *string  `json:"vendor_phone,omitempty"`
	VendorEmail *string  `json:"vendor_email,omitempty"`
	Replies     []*Reply `json:"replies,omitempty"`
}

// Reply is one inbound vendor response tied back to its outreach request.
type Reply struct {
	ID                 int64           `json:"id"`
	OutreachRequestID  int64           `json:"outreach_request_id"`
	Body               string          `json:"body"`
	MediaURLs          json.RawMessage `json:"media_urls"`
	ParsedQuoteCents   *int64          `json:"parsed_quote_cents,omitempty"`
	ParsedAvailability *string         `json:"parsed_availability,omitempty"`
	RawSourceRef       *string         `json:"raw_source_ref,omitempty"`
	ReceivedAt         time.Time       `json:"received_at"`
	CreatedAt          time.Time       `json:"created_at"`
}

// TriggerRequest is the API payload to start outreach for a work order.
type TriggerRequest struct {
	Category   string  `json:"category"`     // trade; defaults to the WO's category
	MaxVendors int     `json:"max_vendors"`  // default 3
	VendorIDs  []int64 `json:"vendor_ids"`   // explicit shortlist override
	Channels   []string `json:"channels"`    // subset of {sms,email}; default both-available
	Note       string  `json:"note"`         // extra PM note appended to the message
}

// DispatchRequest selects a vendor (usually from a reply) and dispatches.
type DispatchRequest struct {
	VendorID   int64  `json:"vendor_id"`
	QuoteCents *int64 `json:"quote_cents,omitempty"`
	ReplyID    *int64 `json:"reply_id,omitempty"`
	Note       string `json:"note,omitempty"`
	NotifyVendor bool `json:"notify_vendor"`
}
