package outlook

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// MaxSubscriptionLifetime is the maximum validity for a Graph calendar subscription (3 days).
const MaxSubscriptionLifetime = 3 * 24 * time.Hour

// CreateSubscription registers a new webhook subscription with Graph.
func (c *GraphCalendarClient) CreateSubscription(ctx context.Context, req CreateSubscriptionRequest) (*Subscription, error) {
	url := fmt.Sprintf("%s/subscriptions", graphBaseURL)
	var sub Subscription
	if err := c.doJSON(ctx, http.MethodPost, url, req, &sub); err != nil {
		return nil, fmt.Errorf("create subscription: %w", err)
	}
	return &sub, nil
}

// RenewSubscription extends the expiration of an existing subscription.
func (c *GraphCalendarClient) RenewSubscription(ctx context.Context, subscriptionID string, newExpiry time.Time) (*Subscription, error) {
	url := fmt.Sprintf("%s/subscriptions/%s", graphBaseURL, subscriptionID)
	body := map[string]string{
		"expirationDateTime": newExpiry.UTC().Format(time.RFC3339),
	}
	var sub Subscription
	if err := c.doJSON(ctx, http.MethodPatch, url, body, &sub); err != nil {
		return nil, fmt.Errorf("renew subscription: %w", err)
	}
	return &sub, nil
}

// DeleteSubscription removes a Graph subscription.
func (c *GraphCalendarClient) DeleteSubscription(ctx context.Context, subscriptionID string) error {
	url := fmt.Sprintf("%s/subscriptions/%s", graphBaseURL, subscriptionID)
	if err := c.doJSON(ctx, http.MethodDelete, url, nil, nil); err != nil {
		return fmt.Errorf("delete subscription: %w", err)
	}
	return nil
}
