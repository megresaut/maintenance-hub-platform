package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"maintenancehub/modules/activity"
	workorder "maintenancehub/modules/maintenance/work_order"
	worrepo "maintenancehub/modules/maintenance/work_order/repository"
	taskrepo "maintenancehub/modules/maintenance/tasks/repository"
)

type workOrderService struct {
	repo     worrepo.WorkOrderRepository
	taskRepo taskrepo.TaskRepository
	rec      *activity.Recorder
}

func NewWorkOrderService(
	repo worrepo.WorkOrderRepository,
	taskRepo taskrepo.TaskRepository,
	rec *activity.Recorder,
) WorkOrderService {
	return &workOrderService{repo: repo, taskRepo: taskRepo, rec: rec}
}

func actorPtr(actorID int64) *int64 {
	if actorID == 0 {
		return nil
	}
	return &actorID
}

func actorType(actorID int64) string {
	if actorID == 0 {
		return "system"
	}
	return "user"
}

func (s *workOrderService) Create(ctx context.Context, orgID, actorID int64, dto workorder.CreateWorkOrderDTO) (*workorder.WorkOrder, error) {
	dto.Name = strings.TrimSpace(dto.Name)
	if dto.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if dto.PropertyID == 0 {
		return nil, fmt.Errorf("property_id is required")
	}
	if dto.Status == "" {
		dto.Status = workorder.WorkOrderStatusNew
	}
	if !workorder.ValidStatus(dto.Status) {
		return nil, fmt.Errorf("invalid status: %s", dto.Status)
	}
	if dto.Priority == "" {
		dto.Priority = "medium"
	}
	if !workorder.ValidPriority(dto.Priority) {
		return nil, fmt.Errorf("invalid priority: %s", dto.Priority)
	}
	if dto.RequestedBy == nil && actorID != 0 {
		dto.RequestedBy = &actorID
	}

	// Guard: when linked, the task must exist in this org.
	if dto.TaskID != nil {
		if _, err := s.taskRepo.GetTaskByID(ctx, orgID, *dto.TaskID); err != nil {
			return nil, fmt.Errorf("task lookup: %w", err)
		}
	}

	wo, err := s.repo.Create(ctx, orgID, dto)
	if err != nil {
		return nil, fmt.Errorf("create work order: %w", err)
	}

	// Register attachment metadata (best-effort; no upload plumbing).
	for _, a := range dto.Attachments {
		uploadedBy := int64(0)
		if dto.RequestedBy != nil {
			uploadedBy = *dto.RequestedBy
		}
		if _, err := s.repo.InsertAttachment(ctx, orgID, workorder.Attachment{
			WorkOrderID:    &wo.ID,
			UploadedByType: actorType(uploadedBy),
			UploadedByID:   uploadedBy,
			FileName:       a.FileName,
			FileType:       a.FileType,
			FileSize:       a.FileSize,
			StoragePath:    a.StoragePath,
			Label:          a.Label,
		}); err != nil {
			log.Printf("[WARN] failed to attach file to work order %d: %v", wo.ID, err)
		}
	}

	s.rec.Record(ctx, activity.Entry{
		OrgID:      orgID,
		TicketType: "work_order",
		TicketID:   wo.ID,
		Kind:       activity.KindCreated,
		ActorType:  actorType(actorID),
		ActorID:    actorPtr(actorID),
		Body:       fmt.Sprintf("Work order %q created", wo.Name),
	})
	if wo.VendorID != nil {
		s.recordVendorAssigned(ctx, orgID, wo.ID, actorID, *wo.VendorID)
	}

	return wo, nil
}

func (s *workOrderService) GetByID(ctx context.Context, orgID, id int64) (*workorder.WorkOrder, error) {
	return s.repo.GetByID(ctx, orgID, id)
}

func (s *workOrderService) Update(ctx context.Context, orgID, id, actorID int64, dto workorder.UpdateWorkOrderDTO) (*workorder.WorkOrder, error) {
	if dto.Status != nil && !workorder.ValidStatus(*dto.Status) {
		return nil, fmt.Errorf("invalid status: %s", *dto.Status)
	}
	if dto.Priority != nil && !workorder.ValidPriority(*dto.Priority) {
		return nil, fmt.Errorf("invalid priority: %s", *dto.Priority)
	}

	before, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}

	wo, err := s.repo.Update(ctx, orgID, id, dto)
	if err != nil {
		return nil, err
	}

	if dto.UpdatedBy != nil && len(dto.Attachments) > 0 {
		for _, a := range dto.Attachments {
			if _, err := s.repo.InsertAttachment(ctx, orgID, workorder.Attachment{
				WorkOrderID:    &wo.ID,
				UploadedByType: "user",
				UploadedByID:   *dto.UpdatedBy,
				FileName:       a.FileName,
				FileType:       a.FileType,
				FileSize:       a.FileSize,
				StoragePath:    a.StoragePath,
				Label:          a.Label,
			}); err != nil {
				log.Printf("[WARN] failed to attach file to work order %d: %v", wo.ID, err)
			}
		}
	}

	if dto.Status != nil && *dto.Status != before.Status {
		meta, _ := json.Marshal(map[string]any{"from": before.Status, "to": *dto.Status})
		s.rec.Record(ctx, activity.Entry{
			OrgID:      orgID,
			TicketType: "work_order",
			TicketID:   id,
			Kind:       activity.KindStatusChange,
			ActorType:  actorType(actorID),
			ActorID:    actorPtr(actorID),
			Body:       fmt.Sprintf("Status changed from %s to %s", before.Status, *dto.Status),
			Metadata:   meta,
		})
	}
	if dto.VendorID != nil && (before.VendorID == nil || *before.VendorID != *dto.VendorID) {
		s.recordVendorAssigned(ctx, orgID, id, actorID, *dto.VendorID)
	}

	return wo, nil
}

func (s *workOrderService) recordVendorAssigned(ctx context.Context, orgID, workOrderID, actorID, vendorID int64) {
	meta, _ := json.Marshal(map[string]any{"vendor_id": vendorID})
	s.rec.Record(ctx, activity.Entry{
		OrgID:      orgID,
		TicketType: "work_order",
		TicketID:   workOrderID,
		Kind:       activity.KindAssigned,
		ActorType:  actorType(actorID),
		ActorID:    actorPtr(actorID),
		Body:       fmt.Sprintf("Vendor #%d assigned", vendorID),
		Metadata:   meta,
	})
}

func (s *workOrderService) Delete(ctx context.Context, orgID, id int64) error {
	// Work orders reference tasks via task_id; the relationship is
	// one-to-many so deleting a work order never touches the task row.
	return s.repo.Delete(ctx, orgID, id)
}

func (s *workOrderService) List(ctx context.Context, orgID int64, taskID *int64, propertyID *int64, vendorID *int64, status *string, limit int) ([]*workorder.WorkOrder, error) {
	return s.repo.List(ctx, orgID, taskID, propertyID, vendorID, status, limit)
}

func (s *workOrderService) MarkCompleted(ctx context.Context, orgID, id int64, completedBy int64, actualCost *float64) (*workorder.WorkOrder, error) {
	before, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, fmt.Errorf("lookup: %w", err)
	}

	after, err := s.repo.MarkCompleted(ctx, orgID, id, completedBy, actualCost)
	if err != nil {
		return nil, fmt.Errorf("mark completed: %w", err)
	}

	meta, _ := json.Marshal(map[string]any{"from": before.Status, "to": after.Status})
	s.rec.Record(ctx, activity.Entry{
		OrgID:      orgID,
		TicketType: "work_order",
		TicketID:   id,
		Kind:       activity.KindStatusChange,
		ActorType:  actorType(completedBy),
		ActorID:    actorPtr(completedBy),
		Body:       fmt.Sprintf("Work order #%d completed", id),
		Metadata:   meta,
	})

	return after, nil
}

// ===== ATTACHMENTS =====

func (s *workOrderService) GetWorkOrderAttachments(ctx context.Context, orgID, workOrderID int64) ([]workorder.Attachment, error) {
	return s.repo.GetWorkOrderAttachments(ctx, orgID, workOrderID)
}

func (s *workOrderService) CreateWorkOrderAttachment(ctx context.Context, orgID, workOrderID int64, a workorder.Attachment) (*workorder.Attachment, error) {
	a.WorkOrderID = &workOrderID
	return s.repo.InsertAttachment(ctx, orgID, a)
}

func (s *workOrderService) UpdateWorkOrderAttachment(ctx context.Context, orgID, attachmentID int64, req workorder.UpdateWorkOrderAttachmentRequest) (*workorder.Attachment, error) {
	return s.repo.UpdateWorkOrderAttachment(ctx, orgID, attachmentID, req)
}

func (s *workOrderService) DeleteWorkOrderAttachment(ctx context.Context, orgID, attachmentID int64) error {
	return s.repo.DeleteWorkOrderAttachment(ctx, orgID, attachmentID)
}

// ===== NOTES =====

func (s *workOrderService) GetWorkOrderNotes(ctx context.Context, orgID, workOrderID int64) ([]workorder.WorkOrderNote, error) {
	return s.repo.GetWorkOrderNotes(ctx, orgID, workOrderID)
}

func (s *workOrderService) CreateWorkOrderNote(ctx context.Context, orgID, workOrderID int64, req workorder.CreateWorkOrderNoteRequest) (*workorder.WorkOrderNote, error) {
	n := &workorder.WorkOrderNote{
		WorkOrderID:       workOrderID,
		AuthorType:        req.AuthorType,
		AuthorID:          req.AuthorID,
		Body:              req.Body,
		IsVisibleToClient: req.IsVisibleToClient,
	}
	if n.AuthorType == "" {
		n.AuthorType = "user"
	}
	if err := s.repo.InsertWorkOrderNote(ctx, orgID, n); err != nil {
		return nil, err
	}
	return n, nil
}

func (s *workOrderService) UpdateWorkOrderNote(ctx context.Context, orgID, noteID int64, req workorder.UpdateWorkOrderNoteRequest) (*workorder.WorkOrderNote, error) {
	return s.repo.UpdateWorkOrderNote(ctx, orgID, noteID, req)
}

func (s *workOrderService) DeleteWorkOrderNote(ctx context.Context, orgID, noteID int64) error {
	return s.repo.DeleteWorkOrderNote(ctx, orgID, noteID)
}
