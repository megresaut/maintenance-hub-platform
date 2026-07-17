package http

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"maintenancehub/httpx"
	"maintenancehub/middleware"
	"maintenancehub/modules/maintenance/recurring"
	"maintenancehub/modules/maintenance/recurring/repository"
	"maintenancehub/modules/maintenance/recurring/service"
)

// ========== helpers ==========

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func writeSeriesErr(w http.ResponseWriter, err error) {
	if errors.Is(err, repository.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, err.Error())
		return
	}
	httpx.Error(w, http.StatusInternalServerError, err.Error())
}

// ========== HANDLERS ==========

func handleCreateSeries(w http.ResponseWriter, r *http.Request, svc service.RecurringService) {
	orgID := middleware.GetOrgID(r.Context())

	var req recurring.CreateSeriesRequest
	if !httpx.Decode(w, r, &req) {
		return
	}

	out, err := svc.CreateSeries(r.Context(), orgID, req)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func handleListSeries(w http.ResponseWriter, r *http.Request, svc service.RecurringService) {
	orgID := middleware.GetOrgID(r.Context())

	var out []*recurring.RecurringSeries
	var err error
	if v := r.URL.Query().Get("property_id"); v != "" {
		propertyID, parseErr := strconv.ParseInt(v, 10, 64)
		if parseErr != nil || propertyID <= 0 {
			httpx.Error(w, http.StatusBadRequest, "invalid property_id")
			return
		}
		out, err = svc.ListSeriesByProperty(r.Context(), orgID, propertyID)
	} else {
		out, err = svc.ListSeries(r.Context(), orgID)
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Ensure we always return an array, even if empty
	if out == nil {
		out = []*recurring.RecurringSeries{}
	}
	httpx.JSON(w, http.StatusOK, out)
}

func handleGetSeries(w http.ResponseWriter, r *http.Request, svc service.RecurringService) {
	orgID := middleware.GetOrgID(r.Context())
	id, err := parseID(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	series, err := svc.GetSeries(r.Context(), orgID, id)
	if err != nil {
		writeSeriesErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, series)
}

func handleUpdateSeries(w http.ResponseWriter, r *http.Request, svc service.RecurringService) {
	orgID := middleware.GetOrgID(r.Context())
	id, err := parseID(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req recurring.UpdateSeriesRequest
	if !httpx.Decode(w, r, &req) {
		return
	}

	out, err := svc.UpdateSeries(r.Context(), orgID, id, req)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, err.Error())
			return
		}
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func handleDeleteSeries(w http.ResponseWriter, r *http.Request, svc service.RecurringService) {
	orgID := middleware.GetOrgID(r.Context())
	id, err := parseID(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := svc.DeleteSeries(r.Context(), orgID, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, err.Error())
			return
		}
		httpx.Error(w, http.StatusInternalServerError, fmt.Sprintf("failed to delete: %s", err.Error()))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func handleSetActive(w http.ResponseWriter, r *http.Request, svc service.RecurringService) {
	orgID := middleware.GetOrgID(r.Context())
	id, err := parseID(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	var body struct {
		Active bool `json:"active"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}

	if err := svc.SetActive(r.Context(), orgID, id, body.Active); err != nil {
		writeSeriesErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"active": body.Active})
}

func handleRunSeries(w http.ResponseWriter, r *http.Request, svc service.RecurringService) {
	orgID := middleware.GetOrgID(r.Context())
	id, err := parseID(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := svc.RunSeries(r.Context(), orgID, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, err.Error())
			return
		}
		httpx.Error(w, http.StatusInternalServerError, fmt.Sprintf("run error: %s", err))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"run": "ok"})
}
