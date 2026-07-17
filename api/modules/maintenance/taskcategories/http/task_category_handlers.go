package http

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"maintenancehub/httpx"
	"maintenancehub/middleware"
	"maintenancehub/modules/maintenance/taskcategories"
	"maintenancehub/modules/maintenance/taskcategories/repository"
	"maintenancehub/modules/maintenance/taskcategories/service"
)

type TaskCategoryHandlers struct {
	svc service.TaskCategoryService
}

func NewTaskCategoryHandlers(svc service.TaskCategoryService) *TaskCategoryHandlers {
	return &TaskCategoryHandlers{svc: svc}
}

// Routes builds the task-categories router. Mounted (behind RequireAuth) at
// /api/task-categories.
func Routes(db *pgxpool.Pool) chi.Router {
	svc := service.NewTaskCategoryService(repository.NewTaskCategoryRepositoryPG(db))
	h := NewTaskCategoryHandlers(svc)

	r := chi.NewRouter()
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Patch("/{id}", h.Rename)
	r.Delete("/{id}", h.Deactivate)
	return r
}

func (h *TaskCategoryHandlers) List(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	cats, err := h.svc.List(r.Context(), orgID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, cats)
}

func (h *TaskCategoryHandlers) Create(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	var req taskcategories.CreateTaskCategoryRequest
	if !httpx.Decode(w, r, &req) {
		return
	}

	var createdBy *int64
	if uid := middleware.GetUserID(r.Context()); uid != 0 {
		createdBy = &uid
	}

	cat, err := h.svc.Create(r.Context(), orgID, req.Name, createdBy)
	if err != nil {
		writeCategoryError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, cat)
}

func (h *TaskCategoryHandlers) Rename(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	id, ok := parseIDParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req taskcategories.UpdateTaskCategoryRequest
	if !httpx.Decode(w, r, &req) {
		return
	}

	cat, err := h.svc.Rename(r.Context(), orgID, id, req.Name)
	if err != nil {
		writeCategoryError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, cat)
}

func (h *TaskCategoryHandlers) Deactivate(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	id, ok := parseIDParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.svc.Deactivate(r.Context(), orgID, id); err != nil {
		writeCategoryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeCategoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, err.Error())
	case errors.Is(err, repository.ErrNameConflict):
		httpx.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrEmptyName):
		httpx.Error(w, http.StatusBadRequest, err.Error())
	case strings.Contains(err.Error(), "reserved"):
		httpx.Error(w, http.StatusBadRequest, err.Error())
	default:
		httpx.Error(w, http.StatusInternalServerError, err.Error())
	}
}

func parseIDParam(raw string) (int64, bool) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}
