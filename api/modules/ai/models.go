package ai

// AIClassificationResult is the parsed output of a classification call.
// Ported from ra-avm; location (sublocation) support removed — properties in
// this product are a flat list.
type AIClassificationResult struct {
	EventType          string  `json:"event_type"`
	VendorID           *int64  `json:"vendor_id,omitempty"`
	PropertyID         *int64  `json:"property_id,omitempty"`
	FieldTeamMemberIDs []int64 `json:"field_team_member_ids,omitempty"`
	CreateProposedWO   bool    `json:"create_proposed_wo"`
	Confidence         float64 `json:"confidence"`
	Reasoning          string  `json:"reasoning"`
	TaskName           string  `json:"task_name,omitempty"`
	WorkOrderName      string  `json:"work_order_name,omitempty"`
	FTOName            string  `json:"fto_name,omitempty"`
	Description        string  `json:"description,omitempty"`
	Priority           string  `json:"priority,omitempty"`
	Category           string  `json:"category,omitempty"` // trade, e.g. plumbing/electrical/hvac
}

// ContextData holds cached per-org reference data injected into AI prompts.
type ContextData struct {
	Vendors    []VendorInfo   `json:"vendors"`
	Properties []PropertyInfo `json:"properties"`
	Users      []UserInfo     `json:"users"`
}

type VendorInfo struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
}

type PropertyInfo struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

type UserInfo struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}
