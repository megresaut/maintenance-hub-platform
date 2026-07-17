package http

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"maintenancehub/modules/activity"
	"maintenancehub/modules/maintenance/tasks/repository"
	"maintenancehub/modules/maintenance/tasks/service"
)

// Routes builds the tasks router. Mounted (behind RequireAuth) at /api/tasks.
func Routes(db *pgxpool.Pool, rec *activity.Recorder) chi.Router {
	repo := repository.NewTaskRepositoryPG(db)
	svc := service.NewTaskService(repo, rec)
	h := NewTaskHandlers(svc)

	r := chi.NewRouter()

	// CREATE + LIST
	r.Post("/", h.CreateTask)
	r.Get("/", h.ListTasks)

	// GET ONE / BY PROPERTY
	r.Get("/{id}", h.GetTask)
	r.Get("/by-property/{property_id}", h.GetTasksByProperty)

	// UPDATE (fields + status change)
	r.Patch("/{id}", h.UpdateTask)

	// STATUS LIFECYCLE ACTIONS
	r.Post("/{id}/close", h.CloseTask)
	r.Post("/{id}/reopen", h.ReopenTask)
	r.Post("/{id}/cancel", h.CancelTask)

	// SOFT DELETE
	r.Delete("/{id}", h.DeleteTask)

	return r
}

// decodeOptional decodes a JSON body into v; an empty body is not an error.
func decodeOptional(r *http.Request, v any) error {
	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		return err
	}
	return json.Unmarshal(body, v)
}
