package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"maintenancehub/modules/activity"
	"maintenancehub/modules/maintenance/tasks"
	"maintenancehub/modules/maintenance/tasks/repository"
)

// TaskService defines business-level operations for tasks. Project
// validation, notification dispatch, subtasks/links/diffs and websocket/feed
// integrations from the source were stripped; activity is recorded through
// the unified ticket_activity feed instead.
type TaskService interface {
	CreateTask(ctx context.Context, orgID int64, actorID int64, req tasks.CreateTaskRequest) (*tasks.Task, error)
	GetTaskByID(ctx context.Context, orgID, id int64) (*tasks.Task, error)
	ListTasks(ctx context.Context, orgID int64, filters tasks.TaskListFilters) ([]*tasks.Task, error)
	GetTasksByProperty(ctx context.Context, orgID, propertyID int64) ([]*tasks.Task, error)
	UpdateTask(ctx context.Context, orgID, id int64, actorID int64, req tasks.UpdateTaskRequest) (*tasks.Task, error)
	CloseTask(ctx context.Context, orgID, taskID, closedBy int64) error
	ReopenTask(ctx context.Context, orgID, taskID, actorID int64) error
	CancelTask(ctx context.Context, orgID, taskID, actorID int64, req tasks.CancelTaskRequest) (*tasks.Task, error)
	DeleteTask(ctx context.Context, orgID, taskID int64) error
}

type taskService struct {
	repo repository.TaskRepository
	rec  *activity.Recorder
}

func NewTaskService(repo repository.TaskRepository, rec *activity.Recorder) TaskService {
	return &taskService{repo: repo, rec: rec}
}

func actorPtr(actorID int64) *int64 {
	if actorID == 0 {
		return nil
	}
	return &actorID
}

func (s *taskService) recordStatusChange(ctx context.Context, orgID, taskID int64, actorID int64, from, to tasks.TaskStatus) {
	meta, _ := json.Marshal(map[string]any{"from": from, "to": to})
	s.rec.Record(ctx, activity.Entry{
		OrgID:      orgID,
		TicketType: "task",
		TicketID:   taskID,
		Kind:       activity.KindStatusChange,
		ActorType:  actorType(actorID),
		ActorID:    actorPtr(actorID),
		Body:       fmt.Sprintf("Status changed from %s to %s", from, to),
		Metadata:   meta,
	})
}

func actorType(actorID int64) string {
	if actorID == 0 {
		return "system"
	}
	return "user"
}

// ===== CREATE =====

func (s *taskService) CreateTask(ctx context.Context, orgID int64, actorID int64, req tasks.CreateTaskRequest) (*tasks.Task, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.PropertyID == 0 {
		return nil, fmt.Errorf("property_id is required")
	}
	if req.Status == "" {
		req.Status = tasks.TaskStatusOpen
	}
	if !tasks.ValidStatus(req.Status) {
		return nil, fmt.Errorf("invalid status: %s", req.Status)
	}
	if req.Priority == "" {
		req.Priority = tasks.PriorityMedium
	}
	if !tasks.ValidPriority(req.Priority) {
		return nil, fmt.Errorf("invalid priority: %s", req.Priority)
	}
	if req.RequestedBy == nil && actorID != 0 {
		req.RequestedBy = &actorID
	}

	t, err := s.repo.CreateTask(ctx, orgID, req)
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}

	s.rec.Record(ctx, activity.Entry{
		OrgID:      orgID,
		TicketType: "task",
		TicketID:   t.ID,
		Kind:       activity.KindCreated,
		ActorType:  actorType(actorID),
		ActorID:    actorPtr(actorID),
		Body:       fmt.Sprintf("Task %q created", t.Name),
	})
	return t, nil
}

// ===== READ =====

func (s *taskService) GetTaskByID(ctx context.Context, orgID, id int64) (*tasks.Task, error) {
	return s.repo.GetTaskByID(ctx, orgID, id)
}

func (s *taskService) ListTasks(ctx context.Context, orgID int64, filters tasks.TaskListFilters) ([]*tasks.Task, error) {
	return s.repo.ListTasks(ctx, orgID, filters)
}

func (s *taskService) GetTasksByProperty(ctx context.Context, orgID, propertyID int64) ([]*tasks.Task, error) {
	return s.repo.GetTasksByProperty(ctx, orgID, propertyID)
}

// ===== UPDATE =====

func (s *taskService) UpdateTask(ctx context.Context, orgID, id int64, actorID int64, req tasks.UpdateTaskRequest) (*tasks.Task, error) {
	if req.Status != nil && !tasks.ValidStatus(*req.Status) {
		return nil, fmt.Errorf("invalid status: %s", *req.Status)
	}
	if req.Priority != nil && !tasks.ValidPriority(*req.Priority) {
		return nil, fmt.Errorf("invalid priority: %s", *req.Priority)
	}

	before, err := s.repo.GetTaskByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}

	t, err := s.repo.UpdateTask(ctx, orgID, id, req)
	if err != nil {
		return nil, err
	}

	if req.Status != nil && *req.Status != before.Status {
		s.recordStatusChange(ctx, orgID, id, actorID, before.Status, *req.Status)
	}
	return t, nil
}

// ===== STATUS LIFECYCLE =====

func (s *taskService) CloseTask(ctx context.Context, orgID, taskID, closedBy int64) error {
	before, err := s.repo.GetTaskByID(ctx, orgID, taskID)
	if err != nil {
		return err
	}
	if err := s.repo.MarkClosed(ctx, orgID, taskID, closedBy); err != nil {
		return err
	}
	s.recordStatusChange(ctx, orgID, taskID, closedBy, before.Status, tasks.TaskStatusClosed)
	return nil
}

func (s *taskService) ReopenTask(ctx context.Context, orgID, taskID, actorID int64) error {
	before, err := s.repo.GetTaskByID(ctx, orgID, taskID)
	if err != nil {
		return err
	}
	if err := s.repo.MarkReopened(ctx, orgID, taskID); err != nil {
		return err
	}
	s.recordStatusChange(ctx, orgID, taskID, actorID, before.Status, tasks.TaskStatusOpen)
	return nil
}

func (s *taskService) CancelTask(ctx context.Context, orgID, taskID, actorID int64, req tasks.CancelTaskRequest) (*tasks.Task, error) {
	before, err := s.repo.GetTaskByID(ctx, orgID, taskID)
	if err != nil {
		return nil, err
	}
	t, err := s.repo.CancelTask(ctx, orgID, taskID)
	if err != nil {
		return nil, err
	}
	s.recordStatusChange(ctx, orgID, taskID, actorID, before.Status, tasks.TaskStatusCancelled)
	if req.Message != "" {
		s.rec.Record(ctx, activity.Entry{
			OrgID:      orgID,
			TicketType: "task",
			TicketID:   taskID,
			Kind:       activity.KindNote,
			ActorType:  actorType(actorID),
			ActorID:    actorPtr(actorID),
			Body:       req.Message,
		})
	}
	return t, nil
}

// ===== DELETE =====

func (s *taskService) DeleteTask(ctx context.Context, orgID, taskID int64) error {
	return s.repo.SoftDeleteTask(ctx, orgID, taskID)
}
