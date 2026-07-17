package http

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"maintenancehub/httpx"
	"maintenancehub/middleware"
	workorder "maintenancehub/modules/maintenance/work_order"
	worrepo "maintenancehub/modules/maintenance/work_order/repository"
	"maintenancehub/modules/maintenance/work_order/service"
)

type WorkOrderHandlers struct {
	svc service.WorkOrderService
}

func NewWorkOrderHandlers(svc service.WorkOrderService) *WorkOrderHandlers {
	return &WorkOrderHandlers{svc: svc}
}

func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return id, err == nil && id > 0
}

func writeWOErr(w http.ResponseWriter, err error) {
	if errors.Is(err, worrepo.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, err.Error())
		return
	}
	httpx.Error(w, http.StatusInternalServerError, err.Error())
}

// POST /
func (h *WorkOrderHandlers) Create(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	actorID := middleware.GetUserID(r.Context())

	var req workorder.CreateWorkOrderDTO
	if !httpx.Decode(w, r, &req) {
		return
	}

	wo, err := h.svc.Create(r.Context(), orgID, actorID, req)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.JSON(w, http.StatusCreated, wo)
}

// GET /?task_id=&property_id=&vendor_id=&status=&limit=
func (h *WorkOrderHandlers) List(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	q := r.URL.Query()

	var (
		taskID     *int64
		propertyID *int64
		vendorID   *int64
		status     *string
	)
	if v := q.Get("task_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid task_id")
			return
		}
		taskID = &n
	}
	if v := q.Get("property_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid property_id")
			return
		}
		propertyID = &n
	}
	if v := q.Get("vendor_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid vendor_id")
			return
		}
		vendorID = &n
	}
	if v := q.Get("status"); v != "" {
		if !workorder.ValidStatus(v) {
			httpx.Error(w, http.StatusBadRequest, "invalid status")
			return
		}
		status = &v
	}
	limit := 100
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	list, err := h.svc.List(r.Context(), orgID, taskID, propertyID, vendorID, status, limit)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, list)
}

// GET /{id}
func (h *WorkOrderHandlers) Get(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	wo, err := h.svc.GetByID(r.Context(), orgID, id)
	if err != nil {
		writeWOErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, wo)
}

// PATCH /{id}
func (h *WorkOrderHandlers) Update(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	actorID := middleware.GetUserID(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req workorder.UpdateWorkOrderDTO
	if !httpx.Decode(w, r, &req) {
		return
	}
	if req.UpdatedBy == nil && actorID != 0 {
		req.UpdatedBy = &actorID
	}

	wo, err := h.svc.Update(r.Context(), orgID, id, actorID, req)
	if err != nil {
		if errors.Is(err, worrepo.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, err.Error())
			return
		}
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, wo)
}

// DELETE /{id}
func (h *WorkOrderHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.Delete(r.Context(), orgID, id); err != nil {
		writeWOErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"success": true})
}

// POST /{id}/complete
func (h *WorkOrderHandlers) Complete(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	actorID := middleware.GetUserID(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	var body struct {
		CompletedBy int64    `json:"completed_by"`
		ActualCost  *float64 `json:"actual_cost"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	if body.CompletedBy == 0 {
		body.CompletedBy = actorID
	}

	wo, err := h.svc.MarkCompleted(r.Context(), orgID, id, body.CompletedBy, body.ActualCost)
	if err != nil {
		writeWOErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, wo)
}

// ===== ATTACHMENTS =====

// GET /{id}/attachments
func (h *WorkOrderHandlers) GetAttachments(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	list, err := h.svc.GetWorkOrderAttachments(r.Context(), orgID, id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, list)
}

// POST /{id}/attachments
func (h *WorkOrderHandlers) CreateAttachment(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	actorID := middleware.GetUserID(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	var a workorder.Attachment
	if !httpx.Decode(w, r, &a) {
		return
	}
	if a.UploadedByID == 0 {
		a.UploadedByID = actorID
	}
	if a.UploadedByType == "" {
		a.UploadedByType = "user"
	}

	created, err := h.svc.CreateWorkOrderAttachment(r.Context(), orgID, id, a)
	if err != nil {
		writeWOErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, created)
}

// PATCH /attachments/{attachment_id}
func (h *WorkOrderHandlers) UpdateAttachment(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	id, ok := pathID(r, "attachment_id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid attachment_id")
		return
	}
	var req workorder.UpdateWorkOrderAttachmentRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	updated, err := h.svc.UpdateWorkOrderAttachment(r.Context(), orgID, id, req)
	if err != nil {
		writeWOErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, updated)
}

// DELETE /attachments/{attachment_id}
func (h *WorkOrderHandlers) DeleteAttachment(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	id, ok := pathID(r, "attachment_id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid attachment_id")
		return
	}
	if err := h.svc.DeleteWorkOrderAttachment(r.Context(), orgID, id); err != nil {
		writeWOErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ===== NOTES =====

// GET /{id}/notes
func (h *WorkOrderHandlers) GetNotes(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	list, err := h.svc.GetWorkOrderNotes(r.Context(), orgID, id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, list)
}

// POST /{id}/notes
func (h *WorkOrderHandlers) CreateNote(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	actorID := middleware.GetUserID(r.Context())
	id, ok := pathID(r, "id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req workorder.CreateWorkOrderNoteRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	if req.AuthorID == 0 {
		req.AuthorID = actorID
	}
	n, err := h.svc.CreateWorkOrderNote(r.Context(), orgID, id, req)
	if err != nil {
		writeWOErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, n)
}

// PATCH /notes/{note_id}
func (h *WorkOrderHandlers) UpdateNote(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	id, ok := pathID(r, "note_id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid note_id")
		return
	}
	var req workorder.UpdateWorkOrderNoteRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	n, err := h.svc.UpdateWorkOrderNote(r.Context(), orgID, id, req)
	if err != nil {
		writeWOErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, n)
}

// DELETE /notes/{note_id}
func (h *WorkOrderHandlers) DeleteNote(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	id, ok := pathID(r, "note_id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid note_id")
		return
	}
	if err := h.svc.DeleteWorkOrderNote(r.Context(), orgID, id); err != nil {
		writeWOErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
