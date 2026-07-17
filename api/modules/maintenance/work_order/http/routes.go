package http

import (
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"maintenancehub/modules/activity"
	taskrepo "maintenancehub/modules/maintenance/tasks/repository"
	worrepo "maintenancehub/modules/maintenance/work_order/repository"
	"maintenancehub/modules/maintenance/work_order/service"
)

// Routes builds the work-orders router. Mounted (behind RequireAuth) at
// /api/work-orders.
func Routes(db *pgxpool.Pool, rec *activity.Recorder) chi.Router {
	repo := worrepo.NewWorkOrderRepositoryPG(db)
	svc := service.NewWorkOrderService(repo, taskrepo.NewTaskRepositoryPG(db), rec)
	h := NewWorkOrderHandlers(svc)

	r := chi.NewRouter()

	// CREATE + LIST
	r.Post("/", h.Create)
	r.Get("/", h.List)

	// GET ONE + UPDATE ONE + DELETE
	r.Get("/{id}", h.Get)
	r.Patch("/{id}", h.Update)
	r.Delete("/{id}", h.Delete)

	// COMPLETE
	r.Post("/{id}/complete", h.Complete)

	// ATTACHMENTS (metadata only)
	r.Get("/{id}/attachments", h.GetAttachments)
	r.Post("/{id}/attachments", h.CreateAttachment)
	r.Patch("/attachments/{attachment_id}", h.UpdateAttachment)
	r.Delete("/attachments/{attachment_id}", h.DeleteAttachment)

	// NOTES
	r.Get("/{id}/notes", h.GetNotes)
	r.Post("/{id}/notes", h.CreateNote)
	r.Patch("/notes/{note_id}", h.UpdateNote)
	r.Delete("/notes/{note_id}", h.DeleteNote)

	return r
}
