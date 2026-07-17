package vendors

import "time"

// Vendor carries only directory/contact columns — the plan explicitly
// excludes all accounting and compliance fields.
type Vendor struct {
	ID              int64     `json:"id"`
	OrgID           int64     `json:"org_id"`
	Name            string    `json:"name"`
	Category        string    `json:"category"`
	ServiceAreaTags []string  `json:"service_area_tags"`
	PrimaryEmail    *string   `json:"primary_email,omitempty"`
	Phone           *string   `json:"phone,omitempty"`
	AltEmail        *string   `json:"alt_email,omitempty"`
	AltPhone        *string   `json:"alt_phone,omitempty"`
	Notes           *string   `json:"notes,omitempty"`
	Website         *string   `json:"website,omitempty"`
	Address         *string   `json:"address,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// PreferredVendor is one row of a property+trade preferred list.
type PreferredVendor struct {
	ID         int64     `json:"id"`
	OrgID      int64     `json:"org_id"`
	PropertyID int64     `json:"property_id"`
	Category   string    `json:"category"`
	VendorID   int64     `json:"vendor_id"`
	Priority   int       `json:"priority"`
	CreatedAt  time.Time `json:"created_at"`

	// Joined
	VendorName   string `json:"vendor_name,omitempty"`
	PropertyName string `json:"property_name,omitempty"`
}
