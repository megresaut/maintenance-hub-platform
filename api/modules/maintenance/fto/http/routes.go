package http

import (
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"maintenancehub/modules/activity"
	ftorepo "maintenancehub/modules/maintenance/fto/repository"
	"maintenancehub/modules/maintenance/fto/service"
	taskrepo "maintenancehub/modules/maintenance/tasks/repository"
)

// Routes builds the FTO router. Mounted (behind RequireAuth) at /api/ftos.
func Routes(db *pgxpool.Pool, rec *activity.Recorder) chi.Router {
	repo := ftorepo.NewFTORepositoryPG(db)
	svc := service.NewFTOService(repo, taskrepo.NewTaskRepositoryPG(db), rec)
	h := NewFTOHandlers(svc)

	r := chi.NewRouter()

	// CREATE + LIST
	r.Post("/", h.Create)
	r.Get("/", h.List)

	// PENDING (form_submission_status = 'none')
	r.Get("/pending", h.ListPending)

	// GET ONE + UPDATE + DELETE
	r.Get("/{id}", h.Get)
	r.Patch("/{id}", h.Update)
	r.Delete("/{id}", h.Delete)

	// COMPLETE
	r.Post("/{id}/complete", h.Complete)

	return r
}
