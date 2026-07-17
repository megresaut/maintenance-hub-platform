package activity

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"maintenancehub/httpx"
	"maintenancehub/middleware"
)

// Routes serves the unified per-ticket activity feed:
// GET /{ticketType}/{id} where ticketType is work_order | fto | task.
func Routes(rec *Recorder) chi.Router {
	r := chi.NewRouter()
	r.Get("/{ticketType}/{id}", func(w http.ResponseWriter, req *http.Request) {
		orgID := middleware.GetOrgID(req.Context())
		ticketType := chi.URLParam(req, "ticketType")
		switch ticketType {
		case "work_order", "fto", "task":
		default:
			httpx.Error(w, http.StatusBadRequest, "ticketType must be work_order, fto, or task")
			return
		}
		id, _ := strconv.ParseInt(chi.URLParam(req, "id"), 10, 64)
		entries, err := rec.List(req.Context(), orgID, ticketType, id)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, entries)
	})

	// POST a manual note onto a ticket's feed.
	r.Post("/{ticketType}/{id}/notes", func(w http.ResponseWriter, req *http.Request) {
		orgID := middleware.GetOrgID(req.Context())
		userID := middleware.GetUserID(req.Context())
		ticketType := chi.URLParam(req, "ticketType")
		id, _ := strconv.ParseInt(chi.URLParam(req, "id"), 10, 64)
		var in struct {
			Body string `json:"body"`
		}
		if !httpx.Decode(w, req, &in) {
			return
		}
		if in.Body == "" {
			httpx.Error(w, http.StatusBadRequest, "body is required")
			return
		}
		rec.Record(req.Context(), Entry{
			OrgID: orgID, TicketType: ticketType, TicketID: id,
			Kind: KindNote, ActorType: "user", ActorID: &userID, Body: in.Body,
		})
		httpx.JSON(w, http.StatusCreated, map[string]bool{"ok": true})
	})
	return r
}
