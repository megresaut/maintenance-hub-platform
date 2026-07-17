package http

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"maintenancehub/httpx"
	"maintenancehub/middleware"
	"maintenancehub/modules/maintenance/tasks"
	"maintenancehub/modules/maintenance/tasks/repository"
	"maintenancehub/modules/maintenance/tasks/service"
)

type TaskHandlers struct {
	svc service.TaskService
}

func NewTaskHandlers(svc service.TaskService) *TaskHandlers {
	return &TaskHandlers{svc: svc}
}

func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return id, err == nil && id > 0
}

func writeTaskErr(w http.ResponseWriter, err error) {
	if errors.Is(err, repository.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, err.Error())
		return
	}
	httpx.Error(w, http.StatusInternalServerError, err.Error())
}

// POST /
func (h *TaskHandlers) CreateTask(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	actorID := middleware.GetUserID(r.Context())

	var req tasks.CreateTaskRequest
	if !httpx.Decode(w, r, &req) {
		return
	}

	t, err := h.svc.CreateTask(r.Context(), orgID, actorID, req)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.JSON(w, http.StatusCreated, t)
}

// GET /?property_id=&status=&assignee=&limit=
func (h *TaskHandlers) ListTasks(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	q := r.URL.Query()

	var f tasks.TaskListFilters
	if v := q.Get("property_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid property_id")
			return
		}
		f.PropertyID = &n
	}
	if v := q.Get("status"); v != "" {
		st := tasks.TaskStatus(v)
		if !tasks.ValidStatus(st) {
			httpx.Error(w, http.StatusBadRequest, "invalid status")
			return
		}
		f.Status = &st
	}
	if v := q.Get("assignee"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid assignee")
			return
		}
		f.Assignee = &n
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Limit = n
		}
	}

	list, err := h.svc.ListTasks(r.Context(), orgID, f)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*tasks.Task{}
	}
	httpx.JSON(w, http.StatusOK, list)
}

// GET /{id}
func (h *TaskHandlers) GetTask(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	t, err := h.svc.GetTaskByID(r.Context(), orgID, id)
	if err != nil {
		writeTaskErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

// GET /by-property/{property_id}
func (h *TaskHandlers) GetTasksByProperty(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	pid, ok := pathID(r, "property_id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid property_id")
		return
	}
	list, err := h.svc.GetTasksByProperty(r.Context(), orgID, pid)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*tasks.Task{}
	}
	httpx.JSON(w, http.StatusOK, list)
}

// PATCH /{id}
func (h *TaskHandlers) UpdateTask(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	actorID := middleware.GetUserID(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req tasks.UpdateTaskRequest
	if !httpx.Decode(w, r, &req) {
		return
	}

	t, err := h.svc.UpdateTask(r.Context(), orgID, id, actorID, req)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, err.Error())
			return
		}
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

// POST /{id}/close
func (h *TaskHandlers) CloseTask(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	actorID := middleware.GetUserID(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.CloseTask(r.Context(), orgID, id, actorID); err != nil {
		writeTaskErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"closed": true})
}

// POST /{id}/reopen
func (h *TaskHandlers) ReopenTask(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	actorID := middleware.GetUserID(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.ReopenTask(r.Context(), orgID, id, actorID); err != nil {
		writeTaskErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"reopened": true})
}

// POST /{id}/cancel
func (h *TaskHandlers) CancelTask(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	actorID := middleware.GetUserID(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req tasks.CancelTaskRequest
	// Body is optional for cancel.
	_ = decodeOptional(r, &req)

	t, err := h.svc.CancelTask(r.Context(), orgID, id, actorID, req)
	if err != nil {
		writeTaskErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

// DELETE /{id}
func (h *TaskHandlers) DeleteTask(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.DeleteTask(r.Context(), orgID, id); err != nil {
		writeTaskErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
