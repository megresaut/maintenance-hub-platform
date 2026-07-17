package http

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"maintenancehub/httpx"
	"maintenancehub/middleware"
	"maintenancehub/modules/maintenance/fto"
	ftorepo "maintenancehub/modules/maintenance/fto/repository"
	"maintenancehub/modules/maintenance/fto/service"
)

type FTOHandlers struct {
	svc service.FTOService
}

func NewFTOHandlers(svc service.FTOService) *FTOHandlers {
	return &FTOHandlers{svc: svc}
}

func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return id, err == nil && id > 0
}

func writeFTOErr(w http.ResponseWriter, err error) {
	if errors.Is(err, ftorepo.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, err.Error())
		return
	}
	httpx.Error(w, http.StatusInternalServerError, err.Error())
}

// POST /
func (h *FTOHandlers) Create(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	actorID := middleware.GetUserID(r.Context())

	var req fto.CreateFTODTO
	if !httpx.Decode(w, r, &req) {
		return
	}

	out, err := h.svc.Create(r.Context(), orgID, actorID, req)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

// GET /?task_id=&property_id=&assignee_id=&field_team_member_id=&status=&priority=&limit=
func (h *FTOHandlers) List(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	q := r.URL.Query()

	var (
		taskID            *int64
		propertyID        *int64
		assigneeID        *int64
		fieldTeamMemberID *int64
		status            *string
		priority          *string
	)

	parse := func(key string) (*int64, bool) {
		v := q.Get(key)
		if v == "" {
			return nil, true
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid "+key)
			return nil, false
		}
		return &n, true
	}

	var ok bool
	if taskID, ok = parse("task_id"); !ok {
		return
	}
	if propertyID, ok = parse("property_id"); !ok {
		return
	}
	if assigneeID, ok = parse("assignee_id"); !ok {
		return
	}
	if fieldTeamMemberID, ok = parse("field_team_member_id"); !ok {
		return
	}
	if v := q.Get("status"); v != "" {
		if !fto.ValidStatus(v) {
			httpx.Error(w, http.StatusBadRequest, "invalid status")
			return
		}
		status = &v
	}
	if v := q.Get("priority"); v != "" {
		priority = &v
	}
	limit := 100
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	list, err := h.svc.List(r.Context(), orgID, taskID, propertyID, assigneeID, fieldTeamMemberID, status, priority, limit)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*fto.FieldTeamOrder{}
	}
	httpx.JSON(w, http.StatusOK, list)
}

// GET /{id}
func (h *FTOHandlers) Get(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	obj, err := h.svc.GetByID(r.Context(), orgID, id)
	if err != nil {
		writeFTOErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, obj)
}

// PATCH /{id}
func (h *FTOHandlers) Update(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	actorID := middleware.GetUserID(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	var body fto.UpdateFTODTO
	if !httpx.Decode(w, r, &body) {
		return
	}

	out, err := h.svc.Update(r.Context(), orgID, id, actorID, body)
	if err != nil {
		if errors.Is(err, fto.ErrTaskNotFound) {
			httpx.Error(w, http.StatusBadRequest, "task not found")
			return
		}
		if errors.Is(err, ftorepo.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, err.Error())
			return
		}
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// DELETE /{id}
func (h *FTOHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.Delete(r.Context(), orgID, id); err != nil {
		writeFTOErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"success": true})
}

// POST /{id}/complete
func (h *FTOHandlers) Complete(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	actorID := middleware.GetUserID(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	var body struct {
		CompletedBy int64  `json:"completed_by"`
		Summary     string `json:"summary"`  // optional
		Resolved    bool   `json:"resolved"` // optional
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	if body.CompletedBy == 0 {
		body.CompletedBy = actorID
	}

	// First: mark it completed in DB
	out, err := h.svc.MarkCompleted(r.Context(), orgID, id, body.CompletedBy)
	if err != nil {
		writeFTOErr(w, err)
		return
	}

	// Second: apply resolution fields if provided
	if body.Summary != "" || body.Resolved {
		if err := h.svc.SetResolution(r.Context(), orgID, id, body.Resolved, &body.Summary); err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	httpx.JSON(w, http.StatusOK, out)
}

// GET /pending?field_team_member_id=7
func (h *FTOHandlers) ListPending(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())

	var fieldTeamMemberID *int64
	if v := r.URL.Query().Get("field_team_member_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid field_team_member_id")
			return
		}
		fieldTeamMemberID = &n
	}

	list, err := h.svc.ListPending(r.Context(), orgID, fieldTeamMemberID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*fto.FieldTeamOrder{}
	}
	httpx.JSON(w, http.StatusOK, list)
}
