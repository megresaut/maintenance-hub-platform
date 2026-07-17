package outlook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// ListEventsDelta performs a delta query against the user's calendar.
// Pass an empty deltaLink for the initial sync; subsequent calls should provide
// the deltaLink from the previous response.
// If start and end are provided, they define the date range for the query.
// If both are nil, defaults to 2020-01-01 through 2030-12-31.
func (c *GraphCalendarClient) ListEventsDelta(ctx context.Context, deltaLink string, start, end *time.Time) (*DeltaResponse, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, err
	}

	url := deltaLink
	if url == "" {
		startStr := "2020-01-01T00:00:00Z"
		endStr := "2030-12-31T23:59:59Z"

		if start != nil {
			startStr = start.UTC().Format(time.RFC3339)
		}
		if end != nil {
			endStr = end.UTC().Format(time.RFC3339)
		}

		url = c.calendarURL(fmt.Sprintf("/calendarView/delta?startDateTime=%s&endDateTime=%s", startStr, endStr))
	}

	var allEvents []CalendarEvent
	for url != "" {
		events, next, delta, err := c.fetchDeltaPage(ctx, url)
		if err != nil {
			return nil, err
		}
		allEvents = append(allEvents, events...)

		if delta != "" {
			return &DeltaResponse{Events: allEvents, DeltaLink: delta}, nil
		}
		url = next
	}

	log.Printf("[calendar] delta query returned %d events", len(allEvents))
	return &DeltaResponse{Events: allEvents}, nil
}

func (c *GraphCalendarClient) fetchDeltaPage(ctx context.Context, url string) ([]CalendarEvent, string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", "", fmt.Errorf("build delta request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Prefer", "odata.maxpagesize=50")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", "", fmt.Errorf("delta request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, "", "", fmt.Errorf("delta query %d: %s", resp.StatusCode, string(b))
	}

	var page struct {
		Value     []CalendarEvent `json:"value"`
		NextLink  string          `json:"@odata.nextLink"`
		DeltaLink string          `json:"@odata.deltaLink"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, "", "", fmt.Errorf("decode delta page: %w", err)
	}

	return page.Value, page.NextLink, page.DeltaLink, nil
}
