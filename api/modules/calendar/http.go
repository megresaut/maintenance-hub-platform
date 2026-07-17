package calendar

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"maintenancehub/httpx"
	"maintenancehub/middleware"
	"maintenancehub/modules/calendar/outlook"
)

// Routes builds the authenticated router for the calendar module. main.go
// mounts it under /api/calendar inside the RequireAuth group.
func Routes(svc *Service) chi.Router {
	r := chi.NewRouter()

	// Per-org Outlook connection admin CRUD.
	r.Get("/connection", getConnection(svc))
	r.Put("/connection", putConnection(svc))
	r.Delete("/connection", deleteConnection(svc))

	// Mirrored/local events.
	r.Get("/events", listEvents(svc))
	r.Post("/events", createEvent(svc))
	r.Post("/events/{id}/push", pushEvent(svc))
	r.Delete("/events/{id}", removeEvent(svc))

	// Manual sync trigger for testing and debugging.
	r.Post("/sync", manualSync(svc))

	// Subscription lifecycle + delta reconciliation sweep.
	r.Post("/subscription/ensure", ensureSubscription(svc))
	r.Post("/delta-sync", deltaSync(svc))

	return r
}

// WebhookRoutes builds the PUBLIC router for Microsoft Graph. main.go must
// mount it UNAUTHENTICATED (outside the RequireAuth group) at
// /api/calendar/webhook — Graph cannot send a bearer token. Notifications
// are authenticated by subscription id + clientState instead, which also
// resolves the org.
func WebhookRoutes(svc *Service) chi.Router {
	r := chi.NewRouter()
	r.Post("/", handleWebhook(svc))
	r.Get("/", handleWebhook(svc)) // validation handshake tolerance
	return r
}

// ---------- connection admin ----------

type connectionInput struct {
	TenantID     string `json:"tenant_id"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	MailboxUPN   string `json:"mailbox_upn"`
	CalendarID   string `json:"calendar_id"`
	Enabled      *bool  `json:"enabled"`
}

// connectionView never exposes the client secret.
type connectionView struct {
	OrgID           int64     `json:"org_id"`
	TenantID        string    `json:"tenant_id"`
	ClientID        string    `json:"client_id"`
	ClientSecretSet bool      `json:"client_secret_set"`
	MailboxUPN      string    `json:"mailbox_upn"`
	CalendarID      string    `json:"calendar_id"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
}

func viewOf(c *Connection) connectionView {
	return connectionView{
		OrgID:           c.OrgID,
		TenantID:        c.TenantID,
		ClientID:        c.ClientID,
		ClientSecretSet: c.ClientSecret != "",
		MailboxUPN:      c.MailboxUPN,
		CalendarID:      c.CalendarID,
		Enabled:         c.Enabled,
		CreatedAt:       c.CreatedAt,
	}
}

func getConnection(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.GetOrgID(r.Context())
		conn, err := svc.GetConnection(r.Context(), orgID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		if conn == nil {
			httpx.Error(w, http.StatusNotFound, "no calendar connection configured")
			return
		}
		httpx.JSON(w, http.StatusOK, viewOf(conn))
	}
}

func putConnection(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.GetOrgID(r.Context())
		var in connectionInput
		if !httpx.Decode(w, r, &in) {
			return
		}
		if in.TenantID == "" || in.ClientID == "" || in.MailboxUPN == "" {
			httpx.Error(w, http.StatusBadRequest, "tenant_id, client_id, and mailbox_upn are required")
			return
		}

		secret := in.ClientSecret
		if secret == "" {
			// Allow updating other fields without re-sending the secret.
			existing, err := svc.GetConnection(r.Context(), orgID)
			if err != nil {
				httpx.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
			if existing == nil {
				httpx.Error(w, http.StatusBadRequest, "client_secret is required")
				return
			}
			secret = existing.ClientSecret
		}

		enabled := true
		if in.Enabled != nil {
			enabled = *in.Enabled
		}

		saved, err := svc.SaveConnection(r.Context(), &Connection{
			OrgID:        orgID,
			TenantID:     in.TenantID,
			ClientID:     in.ClientID,
			ClientSecret: secret,
			MailboxUPN:   in.MailboxUPN,
			CalendarID:   in.CalendarID,
			Enabled:      enabled,
		})
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, viewOf(saved))
	}
}

func deleteConnection(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.GetOrgID(r.Context())
		ok, err := svc.DeleteConnection(r.Context(), orgID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !ok {
			httpx.Error(w, http.StatusNotFound, "no calendar connection configured")
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// ---------- events ----------

func listEvents(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.GetOrgID(r.Context())
		from, err := parseDateParam(r, "from", false)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		to, err := parseDateParam(r, "to", true)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		events, err := svc.ListEvents(r.Context(), orgID, from, to)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, events)
	}
}

type eventInput struct {
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	Location      string    `json:"location"`
	StartAt       time.Time `json:"start_at"`
	EndAt         time.Time `json:"end_at"`
	AllDay        bool      `json:"all_day"`
	ShowAs        string    `json:"show_as"`
	PushToOutlook bool      `json:"push_to_outlook"`
}

// createEvent inserts a local event; when push_to_outlook is true it is
// immediately synced to the org's Outlook calendar.
func createEvent(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.GetOrgID(r.Context())
		var in eventInput
		if !httpx.Decode(w, r, &in) {
			return
		}
		if in.Title == "" || in.StartAt.IsZero() || in.EndAt.IsZero() {
			httpx.Error(w, http.StatusBadRequest, "title, start_at, and end_at are required")
			return
		}
		showAs := in.ShowAs
		if showAs == "" {
			showAs = "busy"
		}
		created, err := svc.repo.CreateLocalEvent(r.Context(), &Event{
			OrgID:       orgID,
			Title:       in.Title,
			Description: in.Description,
			Location:    in.Location,
			StartAt:     in.StartAt,
			EndAt:       in.EndAt,
			AllDay:      in.AllDay,
			ShowAs:      showAs,
		})
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		if in.PushToOutlook {
			if err := svc.SyncEventToOutlook(r.Context(), orgID, created.ID); err != nil {
				log.Printf("[calendar] org %d: push new event %d to Outlook failed: %v", orgID, created.ID, err)
			} else if refreshed, err := svc.repo.GetEvent(r.Context(), orgID, created.ID); err == nil && refreshed != nil {
				created = refreshed
			}
		}
		httpx.JSON(w, http.StatusCreated, created)
	}
}

// pushEvent syncs an existing local event to Outlook (create or patch).
func pushEvent(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.GetOrgID(r.Context())
		id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err := svc.SyncEventToOutlook(r.Context(), orgID, id); err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// removeEvent deletes the linked Outlook event (if any) and marks the local
// row deleted.
func removeEvent(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.GetOrgID(r.Context())
		id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err := svc.DeleteEventFromOutlook(r.Context(), orgID, id); err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// ---------- manual sync ----------

func manualSync(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.GetOrgID(r.Context())
		var req struct {
			Direction      string `json:"direction"` // outlook_to_local | local_to_outlook
			OutlookEventID string `json:"outlook_event_id"`
			EventID        int64  `json:"event_id"`
		}
		if !httpx.Decode(w, r, &req) {
			return
		}

		switch req.Direction {
		case DirectionOutlookToLocal:
			if req.OutlookEventID == "" {
				httpx.Error(w, http.StatusBadRequest, "outlook_event_id required for outlook_to_local")
				return
			}
			if err := svc.SyncEventFromOutlook(r.Context(), orgID, req.OutlookEventID); err != nil {
				log.Printf("[calendar manual sync] org %d: %v", orgID, err)
				httpx.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
		case DirectionLocalToOutlook:
			if req.EventID == 0 {
				httpx.Error(w, http.StatusBadRequest, "event_id required for local_to_outlook")
				return
			}
			if err := svc.SyncEventToOutlook(r.Context(), orgID, req.EventID); err != nil {
				log.Printf("[calendar manual sync] org %d: %v", orgID, err)
				httpx.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
		default:
			httpx.Error(w, http.StatusBadRequest, "direction must be outlook_to_local or local_to_outlook")
			return
		}

		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// ---------- subscription / delta ----------

func ensureSubscription(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.GetOrgID(r.Context())
		if err := svc.EnsureSubscription(r.Context(), orgID); err != nil {
			log.Printf("[calendar admin] org %d: ensure subscription error: %v", orgID, err)
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "subscription_active"})
	}
}

// deltaSync triggers a delta reconciliation sweep for the caller's org.
// Query params:
//   - date: single date (YYYY-MM-DD) — syncs events for that day
//   - start / end: date range bounds (YYYY-MM-DD)
//   - (no params): full sync using the stored delta checkpoint
func deltaSync(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.GetOrgID(r.Context())
		q := r.URL.Query()

		var start, end *time.Time
		if dateStr := q.Get("date"); dateStr != "" {
			parsed, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				httpx.Error(w, http.StatusBadRequest, "invalid date format, use YYYY-MM-DD")
				return
			}
			s := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC)
			e := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 23, 59, 59, 0, time.UTC)
			start, end = &s, &e
		} else {
			var err error
			if start, err = parseDateParam(r, "start", false); err != nil {
				httpx.Error(w, http.StatusBadRequest, err.Error())
				return
			}
			if end, err = parseDateParam(r, "end", true); err != nil {
				httpx.Error(w, http.StatusBadRequest, err.Error())
				return
			}
		}

		count, err := svc.RunDeltaSync(r.Context(), orgID, start, end)
		if err != nil {
			log.Printf("[calendar admin] org %d: delta sync error: %v", orgID, err)
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}

		resp := map[string]any{
			"status":           "ok",
			"events_processed": count,
		}
		if start != nil {
			resp["start"] = start.Format(time.RFC3339)
		}
		if end != nil {
			resp["end"] = end.Format(time.RFC3339)
		}
		httpx.JSON(w, http.StatusOK, resp)
	}
}

// parseDateParam parses a YYYY-MM-DD query param; endOfDay shifts it to 23:59:59.
func parseDateParam(r *http.Request, name string, endOfDay bool) (*time.Time, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", v)
	if err != nil {
		return nil, fmt.Errorf("invalid %s date format, use YYYY-MM-DD", name)
	}
	var t time.Time
	if endOfDay {
		t = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 23, 59, 59, 0, time.UTC)
	} else {
		t = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC)
	}
	return &t, nil
}

// ---------- public webhook ----------

// handleWebhook processes Graph webhook traffic: the validation handshake
// (echo validationToken as text/plain) and change notifications. It must
// answer 200 quickly — notifications are processed asynchronously with a
// background context so Graph's timeout/retry doesn't cancel mid-flight
// Graph or classification calls.
func handleWebhook(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		validationToken, notifications, err := outlook.ParseWebhookRequest(r)
		if err != nil {
			log.Printf("[calendar webhook] parse error: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if validationToken != "" {
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, validationToken)
			return
		}

		w.WriteHeader(http.StatusOK)

		for _, n := range notifications {
			n := n
			go func() {
				ctx := context.Background()
				if err := svc.HandleWebhookNotification(ctx, n.SubscriptionID, n.ClientState, n.Resource, n.ChangeType); err != nil {
					log.Printf("[calendar webhook] notification error: %v", err)
				}
			}()
		}
	}
}
