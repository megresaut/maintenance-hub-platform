package outreach

import (
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"maintenancehub/httpx"
	"maintenancehub/middleware"
)

// Routes (authenticated): trigger + inspect + dispatch, keyed by work order,
// plus the org-wide /all listing.
func Routes(svc *Service, repo *Repo) chi.Router {
	r := chi.NewRouter()
	AllRoutes(r, repo, svc.db)

	// Preview the shortlist without sending anything.
	r.Get("/work-orders/{woID}/shortlist", func(w http.ResponseWriter, req *http.Request) {
		orgID := middleware.GetOrgID(req.Context())
		woID, _ := strconv.ParseInt(chi.URLParam(req, "woID"), 10, 64)
		max, _ := strconv.Atoi(req.URL.Query().Get("max"))
		list, source, err := svc.Shortlist(req.Context(), orgID, woID, req.URL.Query().Get("category"), max)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"source": source, "vendors": list})
	})

	// Trigger outreach: shortlist + send.
	r.Post("/work-orders/{woID}/trigger", func(w http.ResponseWriter, req *http.Request) {
		orgID := middleware.GetOrgID(req.Context())
		userID := middleware.GetUserID(req.Context())
		woID, _ := strconv.ParseInt(chi.URLParam(req, "woID"), 10, 64)
		var in TriggerRequest
		if !httpx.Decode(w, req, &in) {
			return
		}
		reqs, err := svc.Trigger(req.Context(), orgID, woID, userID, in)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, reqs)
	})

	// The side-by-side comparison feed: all requests + replies for a WO.
	r.Get("/work-orders/{woID}", func(w http.ResponseWriter, req *http.Request) {
		orgID := middleware.GetOrgID(req.Context())
		woID, _ := strconv.ParseInt(chi.URLParam(req, "woID"), 10, 64)
		list, err := repo.ListForWorkOrder(req.Context(), orgID, woID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, list)
	})

	// Dispatch: pick the winning vendor/quote.
	r.Post("/work-orders/{woID}/dispatch", func(w http.ResponseWriter, req *http.Request) {
		orgID := middleware.GetOrgID(req.Context())
		userID := middleware.GetUserID(req.Context())
		woID, _ := strconv.ParseInt(chi.URLParam(req, "woID"), 10, 64)
		var in DispatchRequest
		if !httpx.Decode(w, req, &in) {
			return
		}
		if err := svc.Dispatch(req.Context(), orgID, woID, userID, in); err != nil {
			httpx.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	return r
}

// EmailWebhookRoutes (public): inbound email replies in the common
// inbound-parse form format (SendGrid/Mailgun style: from, to, subject,
// text). The `to` address resolves the org via organizations.outreach_email_from.
func EmailWebhookRoutes(svc *Service, db *pgxpool.Pool) chi.Router {
	r := chi.NewRouter()
	r.Post("/inbound", func(w http.ResponseWriter, req *http.Request) {
		if err := req.ParseMultipartForm(10 << 20); err != nil {
			if err := req.ParseForm(); err != nil {
				httpx.Error(w, http.StatusBadRequest, "unparseable form")
				return
			}
		}
		from := req.FormValue("from")
		to := req.FormValue("to")
		subject := req.FormValue("subject")
		text := req.FormValue("text")

		var orgID int64
		err := db.QueryRow(req.Context(), `
			SELECT id FROM organizations
			WHERE LOWER(COALESCE(outreach_email_from,'')) <> ''
			  AND POSITION(LOWER(outreach_email_from) IN LOWER($1)) > 0
			LIMIT 1`, to).Scan(&orgID)
		if err != nil {
			log.Printf("[outreach] inbound email: no org for to=%q", to)
			w.WriteHeader(http.StatusOK) // don't make the provider retry
			return
		}

		routed, err := svc.RouteEmailReply(req.Context(), orgID, from, subject, text)
		if err != nil {
			log.Printf("[outreach] inbound email routing error: %v", err)
		}
		if !routed {
			log.Printf("[outreach] inbound email from %q did not match any open outreach request", from)
		}
		w.WriteHeader(http.StatusOK)
	})
	return r
}
