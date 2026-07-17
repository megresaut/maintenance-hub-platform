package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"maintenancehub/modules/activity"
	"maintenancehub/modules/maintenance/fto"
	ftoRepo "maintenancehub/modules/maintenance/fto/repository"
	taskrepo "maintenancehub/modules/maintenance/tasks/repository"
)

type ftoService struct {
	repo     ftoRepo.FTORepository
	taskRepo taskrepo.TaskRepository
	rec      *activity.Recorder
}

func NewFTOService(
	repo ftoRepo.FTORepository,
	taskRepo taskrepo.TaskRepository,
	rec *activity.Recorder,
) FTOService {
	return &ftoService{repo: repo, taskRepo: taskRepo, rec: rec}
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

// ======================================================
// CREATE FTO
// ======================================================

func (s *ftoService) Create(ctx context.Context, orgID, actorID int64, dto fto.CreateFTODTO) (*fto.FieldTeamOrder, error) {
	dto.Name = strings.TrimSpace(dto.Name)
	if dto.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if dto.Priority == "" {
		dto.Priority = "medium"
	}
	if !fto.ValidPriority(dto.Priority) {
		return nil, fmt.Errorf("invalid priority: %s", dto.Priority)
	}
	if dto.UnitID != nil && *dto.UnitID == 0 {
		dto.UnitID = nil
	}
	if dto.LocationID != nil && *dto.LocationID == 0 {
		dto.LocationID = nil
	}
	if dto.CreatedBy == nil && actorID != 0 {
		dto.CreatedBy = &actorID
	}

	// If task_id is provided, verify the task exists in this org.
	if dto.TaskID != nil {
		if _, err := s.taskRepo.GetTaskByID(ctx, orgID, *dto.TaskID); err != nil {
			return nil, fmt.Errorf("task lookup: %w", err)
		}
	}

	f, err := s.repo.Create(ctx, orgID, dto)
	if err != nil {
		return nil, fmt.Errorf("create fto: %w", err)
	}

	meta, _ := json.Marshal(map[string]any{
		"field_team_member_ids": f.FieldTeamMemberIDs,
		"pm_assignee_ids":       f.PMAssigneeIDs,
		"priority":              f.Priority,
	})
	s.rec.Record(ctx, activity.Entry{
		OrgID:      orgID,
		TicketType: "fto",
		TicketID:   f.ID,
		Kind:       activity.KindCreated,
		ActorType:  actorType(actorID),
		ActorID:    actorPtr(actorID),
		Body:       fmt.Sprintf("Field team order %q created", f.Name),
		Metadata:   meta,
	})

	return f, nil
}

// ======================================================
// GET
// ======================================================

func (s *ftoService) GetByID(ctx context.Context, orgID, id int64) (*fto.FieldTeamOrder, error) {
	return s.repo.GetByID(ctx, orgID, id)
}

// ======================================================
// UPDATE
// ======================================================

func (s *ftoService) Update(ctx context.Context, orgID, id, actorID int64, dto fto.UpdateFTODTO) (*fto.FieldTeamOrder, error) {
	if dto.Status != nil && !fto.ValidStatus(*dto.Status) {
		return nil, fmt.Errorf("invalid status: %s", *dto.Status)
	}
	if dto.Priority != nil && !fto.ValidPriority(*dto.Priority) {
		return nil, fmt.Errorf("invalid priority: %s", *dto.Priority)
	}
	// When setting a non-null, non-zero task_id, validate task exists
	// (omit = keep, 0 = clear).
	if dto.TaskID != nil && *dto.TaskID != 0 {
		if _, err := s.taskRepo.GetTaskByID(ctx, orgID, *dto.TaskID); err != nil {
			return nil, fmt.Errorf("task not found: %w", fto.ErrTaskNotFound)
		}
	}

	before, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}

	f, err := s.repo.Update(ctx, orgID, id, dto)
	if err != nil {
		return nil, err
	}

	if dto.Status != nil && *dto.Status != before.Status {
		meta, _ := json.Marshal(map[string]any{"from": before.Status, "to": *dto.Status})
		s.rec.Record(ctx, activity.Entry{
			OrgID:      orgID,
			TicketType: "fto",
			TicketID:   id,
			Kind:       activity.KindStatusChange,
			ActorType:  actorType(actorID),
			ActorID:    actorPtr(actorID),
			Body:       fmt.Sprintf("Status changed from %s to %s", before.Status, *dto.Status),
			Metadata:   meta,
		})
	}

	return f, nil
}

// ======================================================
// DELETE
// ======================================================

func (s *ftoService) Delete(ctx context.Context, orgID, id int64) error {
	return s.repo.Delete(ctx, orgID, id)
}

// ======================================================
// LIST
// ======================================================

func (s *ftoService) List(
	ctx context.Context,
	orgID int64,
	taskID *int64,
	propertyID *int64,
	assigneeID *int64,
	fieldTeamMemberID *int64,
	status *string,
	priority *string,
	limit int,
) ([]*fto.FieldTeamOrder, error) {
	return s.repo.List(ctx, orgID, taskID, propertyID, assigneeID, fieldTeamMemberID, status, priority, limit)
}

// ======================================================
// MARK COMPLETED
// ======================================================

func (s *ftoService) MarkCompleted(ctx context.Context, orgID, id int64, completedBy int64) (*fto.FieldTeamOrder, error) {
	before, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, fmt.Errorf("lookup: %w", err)
	}

	f, err := s.repo.MarkCompleted(ctx, orgID, id, completedBy)
	if err != nil {
		return nil, fmt.Errorf("mark completed: %w", err)
	}

	meta, _ := json.Marshal(map[string]any{"from": before.Status, "to": f.Status})
	s.rec.Record(ctx, activity.Entry{
		OrgID:      orgID,
		TicketType: "fto",
		TicketID:   id,
		Kind:       activity.KindStatusChange,
		ActorType:  actorType(completedBy),
		ActorID:    actorPtr(completedBy),
		Body:       fmt.Sprintf("FTO #%d completed", id),
		Metadata:   meta,
	})

	return f, nil
}

// ======================================================
// RESOLUTION
// ======================================================

func (s *ftoService) SetResolution(ctx context.Context, orgID, id int64, resolved bool, summary *string) error {
	if _, err := s.repo.GetByID(ctx, orgID, id); err != nil {
		return fmt.Errorf("lookup: %w", err)
	}
	if err := s.repo.SetResolution(ctx, orgID, id, resolved, summary); err != nil {
		return fmt.Errorf("set resolution: %w", err)
	}
	return nil
}

// ======================================================
// PENDING
// ======================================================

func (s *ftoService) ListPending(ctx context.Context, orgID int64, fieldTeamMemberID *int64) ([]*fto.FieldTeamOrder, error) {
	return s.repo.ListPending(ctx, orgID, fieldTeamMemberID)
}
