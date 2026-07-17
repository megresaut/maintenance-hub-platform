package drafts

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// TicketCreator decouples draft approval from the maintenance modules: it
// turns approved extracted_data into a real task / work order / FTO. The
// concrete implementation lives in main.go wiring.
type TicketCreator interface {
	CreateEntity(ctx context.Context, orgID int64, entityType string, data json.RawMessage) (int64, error)
	DeleteEntity(ctx context.Context, orgID int64, entityType string, id int64) error
}

// Service is the drafts review queue. Ported from ra-avm minus the
// websocket hub, duplicate detection, and FTO-merge subsystems.
type Service struct {
	repo    *Repo
	creator TicketCreator
}

func NewService(repo *Repo, creator TicketCreator) *Service {
	return &Service{repo: repo, creator: creator}
}

func (s *Service) CreateDraft(ctx context.Context, params CreateDraftParams) (*AIDraft, error) {
	return s.repo.Create(ctx, params)
}

func (s *Service) GetDraft(ctx context.Context, orgID, id int64) (*AIDraft, error) {
	draft, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if draft == nil {
		return nil, fmt.Errorf("draft not found: %d", id)
	}
	children, err := s.repo.GetChildDrafts(ctx, orgID, id)
	if err != nil {
		log.Printf("[drafts] failed to load children for draft %d: %v", id, err)
	} else {
		draft.ChildDrafts = children
	}
	return draft, nil
}

func (s *Service) ListDrafts(ctx context.Context, orgID int64, status string, limit int) ([]*AIDraft, error) {
	if limit <= 0 {
		limit = 50
	}
	list, err := s.repo.ListByStatus(ctx, orgID, status, limit)
	if err != nil {
		return nil, err
	}
	// Attach children so the queue can render task→WO/FTO pairs together.
	for _, d := range list {
		if d.ParentDraftID == nil {
			children, err := s.repo.GetChildDrafts(ctx, orgID, d.ID)
			if err == nil {
				d.ChildDrafts = children
			}
		}
	}
	return list, nil
}

func (s *Service) CountPending(ctx context.Context, orgID int64) (int, error) {
	return s.repo.CountPending(ctx, orgID)
}

func (s *Service) GetDraftBySourceRef(ctx context.Context, orgID int64, source, sourceRef string) (*AIDraft, error) {
	return s.repo.GetBySourceRef(ctx, orgID, source, sourceRef)
}

// ApproveDraft turns a pending draft into a real entity. If the draft is a
// work_order/fto with a pending parent task draft, the parent is
// auto-approved first and its created task_id injected into the child.
func (s *Service) ApproveDraft(ctx context.Context, orgID, id int64, req ApproveRequest) (*AIDraft, error) {
	draft, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if draft == nil {
		return nil, fmt.Errorf("draft not found: %d", id)
	}
	if draft.Status != DraftStatusPending {
		return nil, fmt.Errorf("draft %d is not pending (status: %s)", id, draft.Status)
	}

	extractedData := draft.ExtractedData
	if len(req.FieldOverrides) > 0 {
		extractedData, err = mergeJSON(extractedData, req.FieldOverrides)
		if err != nil {
			return nil, fmt.Errorf("failed to apply overrides: %w", err)
		}
	}

	// Auto-approve pending parent task draft first, then inject its task_id.
	if (draft.EntityType == "work_order" || draft.EntityType == "fto") && draft.ParentDraftID != nil {
		parentDraft, err := s.repo.GetByID(ctx, orgID, *draft.ParentDraftID)
		if err != nil {
			return nil, fmt.Errorf("failed to get parent draft: %w", err)
		}
		if parentDraft != nil && parentDraft.Status == DraftStatusPending {
			if _, err := s.ApproveDraft(ctx, orgID, parentDraft.ID, ApproveRequest{ReviewedBy: req.ReviewedBy}); err != nil {
				return nil, fmt.Errorf("failed to auto-approve parent task draft: %w", err)
			}
			parentDraft, err = s.repo.GetByID(ctx, orgID, parentDraft.ID)
			if err != nil {
				return nil, fmt.Errorf("failed to reload parent draft: %w", err)
			}
		}
		if parentDraft != nil && parentDraft.CreatedEntityID != nil {
			var entityData map[string]interface{}
			if err := json.Unmarshal(extractedData, &entityData); err == nil {
				entityData["task_id"] = *parentDraft.CreatedEntityID
				extractedData, _ = json.Marshal(entityData)
			}
		}
	}

	extractedData = injectRequestedBy(extractedData, req.ReviewedBy)

	if req.Cascade {
		return s.approveCascade(ctx, orgID, draft, extractedData, req)
	}

	entityID, err := s.creator.CreateEntity(ctx, orgID, draft.EntityType, extractedData)
	if err != nil {
		return nil, fmt.Errorf("failed to create entity from draft: %w", err)
	}

	if err := s.repo.UpdateStatus(ctx, orgID, id, DraftStatusApproved, &req.ReviewedBy); err != nil {
		return nil, err
	}
	if err := s.repo.SetCreatedEntityID(ctx, orgID, id, entityID); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, orgID, id)
}

// approveCascade approves a parent draft and all its pending children,
// rolling back created entities if any child fails.
func (s *Service) approveCascade(ctx context.Context, orgID int64, parent *AIDraft, parentData json.RawMessage, req ApproveRequest) (*AIDraft, error) {
	children, err := s.repo.GetChildDrafts(ctx, orgID, parent.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get child drafts: %w", err)
	}
	var pendingChildren []*AIDraft
	for _, c := range children {
		if c.Status == DraftStatusPending {
			pendingChildren = append(pendingChildren, c)
		}
	}

	parentEntityID, err := s.creator.CreateEntity(ctx, orgID, parent.EntityType, parentData)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s: %w", parent.EntityType, err)
	}

	type childResult struct {
		draft    *AIDraft
		entityID int64
	}
	var results []childResult

	var parentOverrides map[string]interface{}
	if len(req.FieldOverrides) > 0 {
		_ = json.Unmarshal(req.FieldOverrides, &parentOverrides)
	}

	for _, child := range pendingChildren {
		childData := child.ExtractedData
		var entityData map[string]interface{}
		if err := json.Unmarshal(childData, &entityData); err == nil {
			entityData["task_id"] = parentEntityID
			if parentOverrides != nil {
				if v, ok := parentOverrides["property_id"]; ok && v != nil {
					entityData["property_id"] = v
				}
			}
			childData, _ = json.Marshal(entityData)
		}
		childData = injectRequestedBy(childData, req.ReviewedBy)

		childEntityID, err := s.creator.CreateEntity(ctx, orgID, child.EntityType, childData)
		if err != nil {
			// Roll back everything created so far.
			s.rollback(ctx, orgID, parent.EntityType, parentEntityID)
			for _, r := range results {
				s.rollback(ctx, orgID, r.draft.EntityType, r.entityID)
			}
			return nil, fmt.Errorf("failed to create %s: %w", child.EntityType, err)
		}
		results = append(results, childResult{draft: child, entityID: childEntityID})
	}

	if err := s.repo.UpdateStatus(ctx, orgID, parent.ID, DraftStatusApproved, &req.ReviewedBy); err != nil {
		return nil, err
	}
	if err := s.repo.SetCreatedEntityID(ctx, orgID, parent.ID, parentEntityID); err != nil {
		return nil, err
	}
	for _, r := range results {
		if err := s.repo.UpdateStatus(ctx, orgID, r.draft.ID, DraftStatusApproved, &req.ReviewedBy); err != nil {
			log.Printf("[drafts] failed to update child draft %d status: %v", r.draft.ID, err)
		}
		if err := s.repo.SetCreatedEntityID(ctx, orgID, r.draft.ID, r.entityID); err != nil {
			log.Printf("[drafts] failed to set child draft %d entity ID: %v", r.draft.ID, err)
		}
	}
	return s.repo.GetByID(ctx, orgID, parent.ID)
}

func (s *Service) rollback(ctx context.Context, orgID int64, entityType string, entityID int64) {
	if err := s.creator.DeleteEntity(ctx, orgID, entityType, entityID); err != nil {
		log.Printf("[drafts] rollback: failed to delete %s %d: %v", entityType, entityID, err)
	}
}

// RejectDraft rejects a draft and cascades to its pending children.
func (s *Service) RejectDraft(ctx context.Context, orgID, id int64, req RejectRequest) error {
	draft, err := s.repo.GetByID(ctx, orgID, id)
	if err != nil {
		return err
	}
	if draft == nil {
		return fmt.Errorf("draft not found: %d", id)
	}
	if err := s.repo.UpdateStatus(ctx, orgID, id, DraftStatusRejected, &req.ReviewedBy); err != nil {
		return err
	}
	children, err := s.repo.GetChildDrafts(ctx, orgID, id)
	if err != nil {
		log.Printf("[drafts] failed to get children for cascade reject: %v", err)
	}
	for _, child := range children {
		if child.Status == DraftStatusPending {
			if err := s.repo.UpdateStatus(ctx, orgID, child.ID, DraftStatusRejected, &req.ReviewedBy); err != nil {
				log.Printf("[drafts] failed to reject child draft %d: %v", child.ID, err)
			}
		}
	}
	return nil
}

// normalizeDateFields converts date-only strings to RFC3339 so Go time.Time
// parsing succeeds. Exposed for the TicketCreator implementation.
func NormalizeDateFields(data json.RawMessage, fields ...string) json.RawMessage {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return data
	}
	changed := false
	for _, f := range fields {
		v, ok := m[f].(string)
		if !ok || v == "" {
			continue
		}
		if len(v) == 10 {
			if _, err := time.Parse("2006-01-02", v); err == nil {
				m[f] = v + "T00:00:00Z"
				changed = true
			}
		}
	}
	if !changed {
		return data
	}
	out, err := json.Marshal(m)
	if err != nil {
		return data
	}
	return out
}

// NormalizePriority maps AI-generated priority values onto the allowed set.
func NormalizePriority(p string) string {
	switch p {
	case "low", "medium", "high", "urgent":
		return p
	case "normal":
		return "medium"
	case "critical":
		return "urgent"
	default:
		return "medium"
	}
}

func injectRequestedBy(data json.RawMessage, userID int64) json.RawMessage {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return data
	}
	m["requested_by"] = userID
	m["created_by"] = userID
	out, err := json.Marshal(m)
	if err != nil {
		return data
	}
	return out
}

func mergeJSON(base, overrides json.RawMessage) (json.RawMessage, error) {
	var baseMap map[string]interface{}
	if err := json.Unmarshal(base, &baseMap); err != nil {
		return nil, err
	}
	var overrideMap map[string]interface{}
	if err := json.Unmarshal(overrides, &overrideMap); err != nil {
		return nil, err
	}
	for k, v := range overrideMap {
		baseMap[k] = v
	}
	return json.Marshal(baseMap)
}
