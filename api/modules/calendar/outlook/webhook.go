package outlook

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ParseWebhookRequest handles Graph webhook validation and notification parsing.
// Returns (validationToken, notifications, error).
// If validationToken is non-empty, the caller must echo it back as text/plain 200.
func ParseWebhookRequest(r *http.Request) (string, []Notification, error) {
	if tok := r.URL.Query().Get("validationToken"); tok != "" {
		return tok, nil, nil
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", nil, fmt.Errorf("read webhook body: %w", err)
	}

	var payload NotificationPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", nil, fmt.Errorf("parse webhook payload: %w", err)
	}

	return "", payload.Value, nil
}

// ExtractEventID pulls the calendar event ID from a Graph notification resource string.
// Handles both "users/{upn}/events/{eventId}" and "Users/{guid}/Events/{eventId}" (Graph may send either).
func ExtractEventID(resource string) string {
	parts := strings.Split(resource, "/")
	for i, p := range parts {
		if strings.EqualFold(p, "events") && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
