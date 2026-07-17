package http

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"maintenancehub/modules/activity"
	"maintenancehub/modules/maintenance/recurring/repository"
	"maintenancehub/modules/maintenance/recurring/service"
)

// Routes builds the recurring-series router. Mounted (behind RequireAuth) at
// /api/recurring.
func Routes(db *pgxpool.Pool, rec *activity.Recorder) chi.Router {
	repo := repository.NewRecurringRepositoryPG(db)
	svc := service.NewRecurringService(repo, rec)

	r := chi.NewRouter()

	// Create + list series
	r.Post("/", func(w http.ResponseWriter, r *http.Request) { handleCreateSeries(w, r, svc) })
	r.Get("/", func(w http.ResponseWriter, r *http.Request) { handleListSeries(w, r, svc) })

	// Get / update / delete series
	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) { handleGetSeries(w, r, svc) })
	r.Put("/{id}", func(w http.ResponseWriter, r *http.Request) { handleUpdateSeries(w, r, svc) })
	r.Delete("/{id}", func(w http.ResponseWriter, r *http.Request) { handleDeleteSeries(w, r, svc) })

	// Set active/inactive
	r.Post("/{id}/active", func(w http.ResponseWriter, r *http.Request) { handleSetActive(w, r, svc) })

	// Manual run (for scheduler or testing)
	r.Post("/{id}/run", func(w http.ResponseWriter, r *http.Request) { handleRunSeries(w, r, svc) })

	return r
}

// StartPoller launches the background ticker that materializes due series
// into tasks. Call from main: `go recurringhttp.StartPoller(ctx, pool, recorder, 5*time.Minute)`.
func StartPoller(ctx context.Context, db *pgxpool.Pool, rec *activity.Recorder, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	repo := repository.NewRecurringRepositoryPG(db)
	svc := service.NewRecurringService(repo, rec)
	service.NewPoller(repo, svc, interval).Start(ctx)
}
