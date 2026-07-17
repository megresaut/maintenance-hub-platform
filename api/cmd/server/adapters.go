package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"

	"maintenancehub/modules/ai/drafts"
	aiservice "maintenancehub/modules/ai/service"
	ftomod "maintenancehub/modules/maintenance/fto"
	ftoservice "maintenancehub/modules/maintenance/fto/service"
	tasksmod "maintenancehub/modules/maintenance/tasks"
	taskservice "maintenancehub/modules/maintenance/tasks/service"
	workorder "maintenancehub/modules/maintenance/work_order"
	woservice "maintenancehub/modules/maintenance/work_order/service"
)

// ticketCreator implements drafts.TicketCreator on top of the ported
// maintenance services — the seam between the drafts review queue and real
// tickets.
type ticketCreator struct {
	db      *pgxpool.Pool
	taskSvc taskservice.TaskService
	woSvc   woservice.WorkOrderService
	ftoSvc  ftoservice.FTOService
}

func (tc *ticketCreator) CreateEntity(ctx context.Context, orgID int64, entityType string, data json.RawMessage) (int64, error) {
	switch entityType {
	case "task":
		data = drafts.NormalizeDateFields(data, "due_date")
		var req tasksmod.CreateTaskRequest
		if err := json.Unmarshal(data, &req); err != nil {
			return 0, fmt.Errorf("unmarshal task request: %w", err)
		}
		if req.PropertyID == 0 {
			return 0, fmt.Errorf("a property is required to approve this task — edit the draft to select one first")
		}
		req.Priority = tasksmod.TaskPriority(drafts.NormalizePriority(string(req.Priority)))
		if req.Status == "" {
			req.Status = tasksmod.TaskStatusOpen
		}
		actor := actorFrom(data)
		t, err := tc.taskSvc.CreateTask(ctx, orgID, actor, req)
		if err != nil {
			return 0, fmt.Errorf("create task: %w", err)
		}
		return t.ID, nil

	case "work_order":
		data = drafts.NormalizeDateFields(data, "due_date")
		data = renameKey(data, "work_description", "description")
		var dto workorder.CreateWorkOrderDTO
		if err := json.Unmarshal(data, &dto); err != nil {
			return 0, fmt.Errorf("unmarshal work order dto: %w", err)
		}
		if dto.PropertyID == 0 {
			return 0, fmt.Errorf("a property is required to approve this work order — edit the draft to select one first")
		}
		// A vendor is deliberately NOT required: work orders start vendor-less
		// and get one via outreach + dispatch.
		switch dto.Status {
		case workorder.WorkOrderStatusNew, workorder.WorkOrderStatusSent,
			workorder.WorkOrderStatusScheduled, workorder.WorkOrderStatusInProgress:
		default:
			dto.Status = workorder.WorkOrderStatusNew
		}
		dto.Priority = drafts.NormalizePriority(dto.Priority)
		actor := actorFrom(data)
		wo, err := tc.woSvc.Create(ctx, orgID, actor, dto)
		if err != nil {
			return 0, fmt.Errorf("create work order: %w", err)
		}
		// Trade category + photos ride along from the AI draft into the
		// outreach columns added by 030_outreach.sql.
		tc.applyOutreachExtras(ctx, orgID, wo.ID, data)
		return wo.ID, nil

	case "fto":
		var dto ftomod.CreateFTODTO
		if err := json.Unmarshal(data, &dto); err != nil {
			return 0, fmt.Errorf("unmarshal FTO dto: %w", err)
		}
		dto.Priority = drafts.NormalizePriority(dto.Priority)
		actor := actorFrom(data)
		created, err := tc.ftoSvc.Create(ctx, orgID, actor, dto)
		if err != nil {
			return 0, fmt.Errorf("create FTO: %w", err)
		}
		return created.ID, nil

	default:
		return 0, fmt.Errorf("unknown entity type: %s", entityType)
	}
}

func (tc *ticketCreator) DeleteEntity(ctx context.Context, orgID int64, entityType string, id int64) error {
	switch entityType {
	case "task":
		return tc.taskSvc.DeleteTask(ctx, orgID, id)
	case "work_order":
		return tc.woSvc.Delete(ctx, orgID, id)
	case "fto":
		return tc.ftoSvc.Delete(ctx, orgID, id)
	}
	return fmt.Errorf("unknown entity type: %s", entityType)
}

// applyOutreachExtras copies category and media_urls from draft data onto
// the created work order row (columns owned by the outreach migration, not
// part of the ported DTO).
func (tc *ticketCreator) applyOutreachExtras(ctx context.Context, orgID, woID int64, data json.RawMessage) {
	var extras struct {
		Category  string   `json:"category"`
		MediaURLs []string `json:"media_urls"`
	}
	if err := json.Unmarshal(data, &extras); err != nil {
		return
	}
	if extras.Category == "" && len(extras.MediaURLs) == 0 {
		return
	}
	media, _ := json.Marshal(extras.MediaURLs)
	if len(extras.MediaURLs) == 0 {
		media = []byte(`[]`)
	}
	if _, err := tc.db.Exec(ctx, `
		UPDATE work_orders SET category = COALESCE(NULLIF($1,''), category), media_urls = $2::jsonb
		WHERE id = $3 AND org_id = $4`, extras.Category, string(media), woID, orgID); err != nil {
		log.Printf("[adapter] failed to set work order %d outreach extras: %v", woID, err)
	}
}

func actorFrom(data json.RawMessage) int64 {
	var m struct {
		RequestedBy *int64 `json:"requested_by"`
		CreatedBy   *int64 `json:"created_by"`
	}
	_ = json.Unmarshal(data, &m)
	if m.RequestedBy != nil {
		return *m.RequestedBy
	}
	if m.CreatedBy != nil {
		return *m.CreatedBy
	}
	return 0
}

func renameKey(data json.RawMessage, from, to string) json.RawMessage {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return data
	}
	if _, hasTo := m[to]; !hasTo {
		if v, hasFrom := m[from]; hasFrom {
			m[to] = v
			delete(m, from)
			if out, err := json.Marshal(m); err == nil {
				return out
			}
		}
	}
	return data
}

// calendarDraftCreator implements calendar.DraftCreator: classify a newly
// imported Outlook event and create review-queue drafts, mirroring the SMS
// pipeline's draft shapes.
type calendarDraftCreator struct {
	classifier *aiservice.AIClassifier
	draftSvc   *drafts.Service
	model      string
}

func (c *calendarDraftCreator) ClassifyAndDraft(ctx context.Context, orgID int64, source, sourceRef, content string) error {
	result, raw, err := c.classifier.ClassifyCalendarEvent(ctx, orgID, aiservice.CalendarEventInput{
		Subject:     content, // content is the event's pre-rendered text block
		BodyPreview: "",
	})
	if err != nil {
		return err
	}

	switch result.EventType {
	case "work_order", "fto_only", "fto_with_proposed_wo":
	default:
		log.Printf("[calendar-drafts] org=%d event %q classified as %s — no draft", orgID, sourceRef, result.EventType)
		return nil
	}

	rawJSON, _ := json.Marshal(map[string]interface{}{"raw_response": raw, "parsed": result})
	params := func(entityType string, data any, parentID *int64) drafts.CreateDraftParams {
		b, _ := json.Marshal(data)
		return drafts.CreateDraftParams{
			OrgID: orgID, Source: source, SourceRef: &sourceRef,
			EntityType: entityType, ExtractedData: b,
			AIConfidence: &result.Confidence, AIReasoning: &result.Reasoning,
			AIModel: &c.model, RawAIResponse: rawJSON, ParentDraftID: parentID,
		}
	}

	taskDraft, err := c.draftSvc.CreateDraft(ctx, params("task", map[string]any{
		"property_id": result.PropertyID,
		"name":        result.TaskName,
		"description": result.Description,
		"priority":    result.Priority,
		"category":    result.Category,
		"status":      "open",
	}, nil))
	if err != nil {
		return err
	}

	ftoName := result.FTOName
	if ftoName == "" {
		ftoName = result.TaskName
	}
	ftoData := map[string]any{
		"name": ftoName, "description": result.Description,
		"property_id": result.PropertyID, "field_team_member_ids": result.FieldTeamMemberIDs,
		"priority": result.Priority,
	}

	if result.EventType == "work_order" || result.EventType == "fto_with_proposed_wo" {
		woData := map[string]any{
			"vendor_id": result.VendorID, "property_id": result.PropertyID,
			"name":             firstNonEmpty(result.WorkOrderName, result.TaskName),
			"work_description": result.Description,
			"priority":         result.Priority, "category": result.Category,
		}
		if _, err := c.draftSvc.CreateDraft(ctx, params("work_order", woData, &taskDraft.ID)); err != nil {
			log.Printf("[calendar-drafts] work_order draft failed: %v", err)
		}
	}
	if _, err := c.draftSvc.CreateDraft(ctx, params("fto", ftoData, &taskDraft.ID)); err != nil {
		log.Printf("[calendar-drafts] fto draft failed: %v", err)
	}
	return nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
