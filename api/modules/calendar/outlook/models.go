package outlook

import "time"

// CalendarEvent represents a Microsoft Graph calendar event.
type CalendarEvent struct {
	ID           string     `json:"id"`
	ChangeKey    string     `json:"changeKey"`
	Subject      string     `json:"subject"`
	BodyPreview  string     `json:"bodyPreview"`
	Body         EventBody  `json:"body"`
	Start        DateTimeTZ `json:"start"`
	End          DateTimeTZ `json:"end"`
	Location     Location   `json:"location"`
	Attendees    []Attendee `json:"attendees"`
	Organizer    Attendee   `json:"organizer"`
	IsAllDay     bool       `json:"isAllDay"`
	IsCancelled  bool       `json:"isCancelled"`
	ShowAs       string     `json:"showAs"`
	Sensitivity  string     `json:"sensitivity"`
	Categories   []string   `json:"categories"`
	WebLink      string     `json:"webLink"`
	CreatedAt    time.Time  `json:"createdDateTime"`
	LastModified time.Time  `json:"lastModifiedDateTime"`
}

type EventBody struct {
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

type DateTimeTZ struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

type Location struct {
	DisplayName string `json:"displayName"`
}

type Attendee struct {
	EmailAddress EmailAddress `json:"emailAddress"`
	Type         string       `json:"type,omitempty"`
}

type EmailAddress struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// Subscription represents a Microsoft Graph webhook subscription.
type Subscription struct {
	ID                 string `json:"id"`
	Resource           string `json:"resource"`
	ChangeType         string `json:"changeType"`
	NotificationURL    string `json:"notificationUrl"`
	ExpirationDateTime string `json:"expirationDateTime"`
	ClientState        string `json:"clientState"`
}

// Notification is a single entry in a Graph webhook payload.
type Notification struct {
	SubscriptionID         string `json:"subscriptionId"`
	ChangeType             string `json:"changeType"`
	ClientState            string `json:"clientState"`
	Resource               string `json:"resource"`
	SubscriptionExpiration string `json:"subscriptionExpirationDateTime"`
	ResourceData           struct {
		ID   string `json:"id"`
		Type string `json:"@odata.type"`
	} `json:"resourceData"`
}

// NotificationPayload is the top-level webhook body from Graph.
type NotificationPayload struct {
	Value []Notification `json:"value"`
}

// DeltaResponse wraps a delta query result set.
type DeltaResponse struct {
	Events    []CalendarEvent
	DeltaLink string
	NextLink  string
}

// CreateEventRequest is the body for creating a calendar event via Graph.
type CreateEventRequest struct {
	Subject   string     `json:"subject"`
	Body      EventBody  `json:"body"`
	Start     DateTimeTZ `json:"start"`
	End       DateTimeTZ `json:"end"`
	Location  *Location  `json:"location,omitempty"`
	Attendees []Attendee `json:"attendees,omitempty"`
	IsAllDay  bool       `json:"isAllDay,omitempty"`
}

// UpdateEventRequest is the body for patching a calendar event via Graph.
type UpdateEventRequest struct {
	Subject   *string     `json:"subject,omitempty"`
	Body      *EventBody  `json:"body,omitempty"`
	Start     *DateTimeTZ `json:"start,omitempty"`
	End       *DateTimeTZ `json:"end,omitempty"`
	Location  *Location   `json:"location,omitempty"`
	Attendees []Attendee  `json:"attendees,omitempty"`
	IsAllDay  *bool       `json:"isAllDay,omitempty"`
}

// CreateSubscriptionRequest is the body for creating a Graph subscription.
type CreateSubscriptionRequest struct {
	ChangeType         string `json:"changeType"`
	NotificationURL    string `json:"notificationUrl"`
	Resource           string `json:"resource"`
	ExpirationDateTime string `json:"expirationDateTime"`
	ClientState        string `json:"clientState"`
}
