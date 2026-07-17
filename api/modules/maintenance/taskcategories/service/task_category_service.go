package service

import (
	"context"
	"errors"
	"strings"

	"maintenancehub/modules/maintenance/taskcategories"
	"maintenancehub/modules/maintenance/taskcategories/repository"
)

var ErrEmptyName = errors.New("category name is required")

type TaskCategoryService interface {
	List(ctx context.Context, orgID int64) ([]taskcategories.TaskCategory, error)
	Create(ctx context.Context, orgID int64, name string, createdByUserID *int64) (*taskcategories.TaskCategory, error)
	Rename(ctx context.Context, orgID, id int64, newName string) (*taskcategories.TaskCategory, error)
	Deactivate(ctx context.Context, orgID, id int64) error
}

type taskCategoryService struct {
	repo repository.TaskCategoryRepository
}

func NewTaskCategoryService(repo repository.TaskCategoryRepository) TaskCategoryService {
	return &taskCategoryService{repo: repo}
}

func (s *taskCategoryService) List(ctx context.Context, orgID int64) ([]taskcategories.TaskCategory, error) {
	return s.repo.List(ctx, orgID)
}

func (s *taskCategoryService) Create(ctx context.Context, orgID int64, name string, createdByUserID *int64) (*taskcategories.TaskCategory, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrEmptyName
	}
	if strings.EqualFold(name, "other") {
		return nil, errors.New(`"Other" is reserved`)
	}
	return s.repo.Create(ctx, orgID, name, createdByUserID)
}

func (s *taskCategoryService) Rename(ctx context.Context, orgID, id int64, newName string) (*taskcategories.TaskCategory, error) {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return nil, ErrEmptyName
	}
	if strings.EqualFold(newName, "other") {
		return nil, errors.New(`"Other" is reserved`)
	}
	return s.repo.Rename(ctx, orgID, id, newName)
}

func (s *taskCategoryService) Deactivate(ctx context.Context, orgID, id int64) error {
	return s.repo.Deactivate(ctx, orgID, id)
}
