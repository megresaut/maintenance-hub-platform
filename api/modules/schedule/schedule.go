// Package schedule aggregates every dated piece of maintenance work — work
// orders, field team orders, recurring series, and synced calendar events —
// into one org-scoped feed for the calendar UI.
package schedule

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"maintenancehub/httpx"
	"maintenancehub/middleware"
)

// Item is one entry on the maintenance calendar.
type Item struct {
	Kind         string     `json:"kind"`        // work_order | fto | recurring | calendar_event
	ID           int64      `json:"id"`          // id within its kind
	Name         string     `json:"name"`
	PropertyID   *int64     `json:"property_id,omitempty"`
	PropertyName string     `json:"property_name,omitempty"`
	Status       string     `json:"status,omitempty"`
	Priority     string     `json:"priority,omitempty"`
	Category     string     `json:"category,omitempty"`
	VendorName   string     `json:"vendor_name,omitempty"`
	StartAt      time.Time  `json:"start_at"`            // the date it lands on
	EndAt        *time.Time `json:"end_at,omitempty"`
	DateSource   string     `json:"date_source"`         // event | due | dispatch | next_run
}

func Routes(db *pgxpool.Pool) chi.Router {
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		orgID := middleware.GetOrgID(req.Context())
		from, to := parseRange(req)
		items, err := load(req.Context(), db, orgID, from, to)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, items)
	})
	return r
}

func parseRange(req *http.Request) (time.Time, time.Time) {
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	to := from.AddDate(0, 1, 0)
	if v := req.URL.Query().Get("from"); v != "" {
		if t, err := time.ParseInLocation("2006-01-02", v, time.Local); err == nil {
			from = t
		}
	}
	if v := req.URL.Query().Get("to"); v != "" {
		if t, err := time.ParseInLocation("2006-01-02", v, time.Local); err == nil {
			to = t.AddDate(0, 0, 1) // inclusive end date
		}
	}
	return from, to
}

func load(ctx context.Context, db *pgxpool.Pool, orgID int64, from, to time.Time) ([]Item, error) {
	items := []Item{}

	// Work orders: prefer the scheduled visit window, else due date, else
	// dispatch time — one entry per order, on its most meaningful date.
	rows, err := db.Query(ctx, `
		SELECT w.id, w.name, w.property_id, COALESCE(p.name,''), w.status, w.priority,
		       COALESCE(w.category,''), COALESCE(v.name,''),
		       COALESCE(w.event_start_at, w.due_date, w.dispatched_at) AS start_at,
		       w.event_end_at,
		       CASE WHEN w.event_start_at IS NOT NULL THEN 'event'
		            WHEN w.due_date IS NOT NULL THEN 'due'
		            ELSE 'dispatch' END
		FROM work_orders w
		LEFT JOIN properties p ON p.id = w.property_id
		LEFT JOIN vendors v ON v.id = w.vendor_id
		WHERE w.org_id = $1
		  AND COALESCE(w.event_start_at, w.due_date, w.dispatched_at) >= $2
		  AND COALESCE(w.event_start_at, w.due_date, w.dispatched_at) < $3
		  AND w.status NOT IN ('cancelled')`, orgID, from, to)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		it := Item{Kind: "work_order"}
		if err := rows.Scan(&it.ID, &it.Name, &it.PropertyID, &it.PropertyName, &it.Status,
			&it.Priority, &it.Category, &it.VendorName, &it.StartAt, &it.EndAt, &it.DateSource); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Field team orders — the on-site visit window or due date.
	rows, err = db.Query(ctx, `
		SELECT f.id, f.name, f.property_id, COALESCE(p.name,''), f.status, f.priority,
		       COALESCE(f.event_start_at, f.due_date, f.dispatched_at) AS start_at,
		       f.event_end_at,
		       CASE WHEN f.event_start_at IS NOT NULL THEN 'event'
		            WHEN f.due_date IS NOT NULL THEN 'due'
		            ELSE 'dispatch' END
		FROM field_team_orders f
		LEFT JOIN properties p ON p.id = f.property_id
		WHERE f.org_id = $1
		  AND COALESCE(f.event_start_at, f.due_date, f.dispatched_at) >= $2
		  AND COALESCE(f.event_start_at, f.due_date, f.dispatched_at) < $3
		  AND f.status NOT IN ('cancelled')`, orgID, from, to)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		it := Item{Kind: "fto"}
		if err := rows.Scan(&it.ID, &it.Name, &it.PropertyID, &it.PropertyName, &it.Status,
			&it.Priority, &it.StartAt, &it.EndAt, &it.DateSource); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Recurring series — upcoming run dates.
	rows, err = db.Query(ctx, `
		SELECT r.id, r.name, r.property_id, COALESCE(p.name,''), r.priority,
		       COALESCE(r.category,''), r.next_run_at
		FROM recurring_task_series r
		LEFT JOIN properties p ON p.id = r.property_id
		WHERE r.org_id = $1 AND r.active
		  AND r.next_run_at >= $2 AND r.next_run_at < $3`, orgID, from, to)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		it := Item{Kind: "recurring", Status: "scheduled", DateSource: "next_run"}
		if err := rows.Scan(&it.ID, &it.Name, &it.PropertyID, &it.PropertyName,
			&it.Priority, &it.Category, &it.StartAt); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Synced/local calendar events (Outlook mirror).
	rows, err = db.Query(ctx, `
		SELECT id, COALESCE(title,''), start_at, end_at
		FROM calendar_events
		WHERE org_id = $1 AND COALESCE(sync_status,'') NOT IN ('deleted')
		  AND start_at >= $2 AND start_at < $3`, orgID, from, to)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		it := Item{Kind: "calendar_event", DateSource: "event"}
		if err := rows.Scan(&it.ID, &it.Name, &it.StartAt, &it.EndAt); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, it)
	}
	rows.Close()
	return items, rows.Err()
}
