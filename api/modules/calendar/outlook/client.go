package outlook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const graphBaseURL = "https://graph.microsoft.com/v1.0"

// GraphCalendarClient wraps Microsoft Graph calendar API operations.
// Uses OAuth2 client-credentials flow (application permissions). One client
// is constructed per org connection; the access token is cached in memory
// on the client instance.
type GraphCalendarClient struct {
	TenantID     string
	ClientID     string
	ClientSecret string
	UserUPN      string // calendar owner mailbox UPN
	CalendarID   string // optional: specific calendar ID (for shared calendars)

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
	httpClient  *http.Client
}

// NewGraphCalendarClient constructs a client for Graph calendar operations.
// If calendarID is provided, queries will target that specific calendar instead of the default.
func NewGraphCalendarClient(tenantID, clientID, clientSecret, userUPN, calendarID string) *GraphCalendarClient {
	return &GraphCalendarClient{
		TenantID:     tenantID,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		UserUPN:      userUPN,
		CalendarID:   calendarID,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

// ---------- token management ----------

func (c *GraphCalendarClient) ensureToken(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.tokenExpiry.Add(-1*time.Minute)) {
		return nil
	}

	endpoint := fmt.Sprintf(
		"https://login.microsoftonline.com/%s/oauth2/v2.0/token", c.TenantID,
	)
	body := strings.NewReader(fmt.Sprintf(
		"client_id=%s&client_secret=%s&scope=https%%3A%%2F%%2Fgraph.microsoft.com%%2F.default&grant_type=client_credentials",
		c.ClientID, c.ClientSecret,
	))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token error %d: %s", resp.StatusCode, string(b))
	}

	var data struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("decode token: %w", err)
	}

	c.token = data.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(data.ExpiresIn) * time.Second)
	return nil
}

// ---------- internal helpers ----------

func (c *GraphCalendarClient) doJSON(ctx context.Context, method, url string, reqBody any, out any) error {
	// Marshal the body once; we may have to send it twice if the token was
	// rejected by Graph despite our cached expiry saying it's still valid.
	var rawBody []byte
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		rawBody = b
	}

	attempt := func() (*http.Response, error) {
		if err := c.ensureToken(ctx); err != nil {
			return nil, err
		}
		var bodyReader io.Reader
		if rawBody != nil {
			bodyReader = bytes.NewReader(rawBody)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		return c.httpClient.Do(req)
	}

	resp, err := attempt()
	if err != nil {
		return fmt.Errorf("graph request failed: %w", err)
	}

	// If the token was rejected (Graph returns 401 InvalidAuthenticationToken)
	// drop the cached token and retry once. Covers cases where the cached
	// expiry is still in the future but Microsoft invalidated the token —
	// e.g. server was paused, clock drift, AAD key rotation.
	if resp.StatusCode == http.StatusUnauthorized {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		c.mu.Lock()
		c.token = ""
		c.tokenExpiry = time.Time{}
		c.mu.Unlock()
		retry, rerr := attempt()
		if rerr != nil {
			return fmt.Errorf("graph request failed after 401 refresh: %w (original body: %s)", rerr, string(b))
		}
		resp = retry
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("graph %s %s → %d: %s", method, url, resp.StatusCode, string(b))
	}

	if out != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func (c *GraphCalendarClient) calendarURL(path string) string {
	if c.CalendarID != "" {
		return fmt.Sprintf("%s/users/%s/calendars/%s%s", graphBaseURL, c.UserUPN, c.CalendarID, path)
	}
	return fmt.Sprintf("%s/users/%s%s", graphBaseURL, c.UserUPN, path)
}

// ---------- Calendar Event CRUD ----------

// GetEvent fetches a single calendar event by ID.
func (c *GraphCalendarClient) GetEvent(ctx context.Context, eventID string) (*CalendarEvent, error) {
	url := c.calendarURL(fmt.Sprintf("/events/%s", eventID))
	var ev CalendarEvent
	if err := c.doJSON(ctx, http.MethodGet, url, nil, &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

// CreateEvent creates a new event on the configured calendar.
func (c *GraphCalendarClient) CreateEvent(ctx context.Context, req CreateEventRequest) (*CalendarEvent, error) {
	url := c.calendarURL("/events")
	var ev CalendarEvent
	if err := c.doJSON(ctx, http.MethodPost, url, req, &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

// UpdateEvent patches an existing calendar event.
func (c *GraphCalendarClient) UpdateEvent(ctx context.Context, eventID string, req UpdateEventRequest) (*CalendarEvent, error) {
	url := c.calendarURL(fmt.Sprintf("/events/%s", eventID))
	var ev CalendarEvent
	if err := c.doJSON(ctx, http.MethodPatch, url, req, &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

// DeleteEvent removes a calendar event. Returns nil if the event is already gone (404).
func (c *GraphCalendarClient) DeleteEvent(ctx context.Context, eventID string) error {
	url := c.calendarURL(fmt.Sprintf("/events/%s", eventID))
	err := c.doJSON(ctx, http.MethodDelete, url, nil, nil)
	if err != nil && strings.Contains(err.Error(), "→ 404:") {
		return nil
	}
	return err
}

// IsNotFoundError returns true if the error indicates a 404 from Graph.
func IsNotFoundError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "→ 404:")
}
