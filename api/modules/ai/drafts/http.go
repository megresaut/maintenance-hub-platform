package drafts

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"maintenancehub/httpx"
	"maintenancehub/middleware"
)

func Routes(svc *Service) chi.Router {
	r := chi.NewRouter()
	r.Get("/", listHandler(svc))
	r.Get("/count", countHandler(svc))
	r.Get("/{id}", getHandler(svc))
	r.Post("/{id}/approve", approveHandler(svc))
	r.Post("/{id}/reject", rejectHandler(svc))
	return r
}

func listHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.GetOrgID(r.Context())
		status := r.URL.Query().Get("status")
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		list, err := svc.ListDrafts(r.Context(), orgID, status, limit)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, list)
	}
}

func countHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.GetOrgID(r.Context())
		n, err := svc.CountPending(r.Context(), orgID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]int{"pending": n})
	}
}

func getHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.GetOrgID(r.Context())
		id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		draft, err := svc.GetDraft(r.Context(), orgID, id)
		if err != nil {
			httpx.Error(w, http.StatusNotFound, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, draft)
	}
}

func approveHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.GetOrgID(r.Context())
		id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		var req ApproveRequest
		if !httpx.Decode(w, r, &req) {
			return
		}
		req.ReviewedBy = middleware.GetUserID(r.Context())
		draft, err := svc.ApproveDraft(r.Context(), orgID, id, req)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, draft)
	}
}

func rejectHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.GetOrgID(r.Context())
		id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		req := RejectRequest{ReviewedBy: middleware.GetUserID(r.Context())}
		if err := svc.RejectDraft(r.Context(), orgID, id, req); err != nil {
			httpx.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
