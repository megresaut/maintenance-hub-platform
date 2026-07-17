package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"maintenancehub/modules/activity"
	"maintenancehub/modules/maintenance/recurring"
	"maintenancehub/modules/maintenance/recurring/repository"
)

const maxSeriesNameLen = 200

type RecurringService interface {
	CreateSeries(ctx context.Context, orgID int64, req recurring.CreateSeriesRequest) (*recurring.RecurringSeries, error)
	UpdateSeries(ctx context.Context, orgID, id int64, req recurring.UpdateSeriesRequest) (*recurring.RecurringSeries, error)
	GetSeries(ctx context.Context, orgID, id int64) (*recurring.RecurringSeries, error)
	ListSeries(ctx context.Context, orgID int64) ([]*recurring.RecurringSeries, error)
	ListSeriesByProperty(ctx context.Context, orgID, propertyID int64) ([]*recurring.RecurringSeries, error)
	SetActive(ctx context.Context, orgID, id int64, active bool) error
	DeleteSeries(ctx context.Context, orgID, id int64) error

	// scheduler trigger: materializes the due occurrence as a task and
	// advances next_run_at.
	RunSeries(ctx context.Context, orgID, seriesID int64) error
}

type recurringService struct {
	repo repository.RecurringRepository
	rec  *activity.Recorder
}

func NewRecurringService(repo repository.RecurringRepository, rec *activity.Recorder) *recurringService {
	return &recurringService{repo: repo, rec: rec}
}

///////////////////////////////////////////////////////////////////////////////
// CREATE SERIES
///////////////////////////////////////////////////////////////////////////////

func (s *recurringService) CreateSeries(ctx context.Context, orgID int64, req recurring.CreateSeriesRequest) (*recurring.RecurringSeries, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if len(name) > maxSeriesNameLen {
		return nil, fmt.Errorf("name too long")
	}
	if req.PropertyID == 0 {
		return nil, fmt.Errorf("property_id is required")
	}
	description := strings.TrimSpace(req.Description)

	normalizedDueAfterDays, endDate, err := normalizeCreateSeriesRequest(req)
	if err != nil {
		return nil, err
	}

	// 1. Compute first run timestamp.
	//
	// `start_date` IS the first run when it lies in the future (or today) —
	// CalculateNextRun is for advancing past an already-run occurrence, so
	// using it on the start date would skip the user's chosen day entirely
	// (e.g. monthly + start_date=May 1 → it returned June 1, missing May).
	// Only call it when start_date is already past, to walk forward to the
	// next valid occurrence.
	var nextRun time.Time
	if !req.StartDate.Before(time.Now()) {
		nextRun = req.StartDate
	} else {
		var err error
		nextRun, err = recurring.CalculateNextRun(
			req.Frequency,
			req.Interval,
			req.StartDate,
			req.ByDay,
			req.DayOfMonth,
			req.WeekOfMonth,
			req.WeekdayOfMonth,
		)
		if err != nil {
			return nil, fmt.Errorf("calculate next run: %w", err)
		}
	}

	// Convert string to *string for nullable fields
	var priorityPtr *string
	if req.Priority != "" {
		priorityPtr = &req.Priority
	}
	var categoryPtr *string
	if req.Category != "" {
		categoryPtr = &req.Category
	}

	series := recurring.RecurringSeries{
		PropertyID:     req.PropertyID,
		UnitID:         req.UnitID,
		LocationID:     req.LocationID,
		TemplateTaskID: nil,
		Name:           name,
		Description:    description,
		Assignees:      req.Assignees,
		Collaborators:  req.Collaborators,
		Priority:       priorityPtr,
		Category:       categoryPtr,
		Frequency:      req.Frequency,
		Interval:       req.Interval,
		ByDay:          req.ByDay,
		DayOfMonth:     req.DayOfMonth,
		WeekOfMonth:    req.WeekOfMonth,
		WeekdayOfMonth: req.WeekdayOfMonth,
		StartDate:      req.StartDate,
		EndDate:        endDate,
		DueAfterDays:   normalizedDueAfterDays,
		Active:         true,
		NextRunAt:      nextRun,
		LastRunAt:      nil,
	}

	out, err := s.repo.InsertSeries(ctx, orgID, series)
	if err != nil {
		log.Printf("[WARN] recurring series insert failed: %v", err)
		return nil, fmt.Errorf("insert series: %w", err)
	}

	return out, nil
}

///////////////////////////////////////////////////////////////////////////////
// UPDATE SERIES
///////////////////////////////////////////////////////////////////////////////

func (s *recurringService) UpdateSeries(ctx context.Context, orgID, id int64, req recurring.UpdateSeriesRequest) (*recurring.RecurringSeries, error) {

	// Load existing series
	existing, err := s.repo.GetSeries(ctx, orgID, id)
	if err != nil {
		return nil, fmt.Errorf("get series: %w", err)
	}

	// Apply edits (pointer fields)
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			return nil, fmt.Errorf("name is required")
		}
		if len(trimmed) > maxSeriesNameLen {
			return nil, fmt.Errorf("name too long")
		}
		existing.Name = trimmed
	}
	if req.Description != nil {
		existing.Description = strings.TrimSpace(*req.Description)
	}
	if req.Frequency != nil {
		existing.Frequency = *req.Frequency
	}
	if req.Interval != nil {
		existing.Interval = *req.Interval
	}
	if req.ByDay != nil {
		existing.ByDay = req.ByDay
	}
	if req.DayOfMonth != nil {
		existing.DayOfMonth = req.DayOfMonth
	}
	if req.WeekOfMonth != nil {
		existing.WeekOfMonth = req.WeekOfMonth
	}
	if req.WeekdayOfMonth != nil {
		existing.WeekdayOfMonth = req.WeekdayOfMonth
	}
	if req.StartDate != nil {
		existing.StartDate = *req.StartDate
	}
	if req.EndDate != nil {
		existing.EndDate = req.EndDate
	}
	if req.DueAfterDays != nil {
		existing.DueAfterDays = *req.DueAfterDays
	}
	if req.Assignees != nil {
		existing.Assignees = req.Assignees
	}
	if req.Collaborators != nil {
		existing.Collaborators = req.Collaborators
	}
	if req.Priority != nil {
		existing.Priority = req.Priority
	}
	if req.Category != nil {
		existing.Category = req.Category
	}

	// Recompute next run. Same rule as create: if start_date is in the
	// future, that IS the next run — don't advance past it.
	var nextRun time.Time
	if !existing.StartDate.Before(time.Now()) {
		nextRun = existing.StartDate
	} else {
		var err error
		nextRun, err = recurring.CalculateNextRun(
			existing.Frequency,
			existing.Interval,
			existing.StartDate,
			existing.ByDay,
			existing.DayOfMonth,
			existing.WeekOfMonth,
			existing.WeekdayOfMonth,
		)
		if err != nil {
			return nil, err
		}
	}

	existing.NextRunAt = nextRun

	updated, err := s.repo.UpdateSeries(ctx, orgID, id, *existing)
	if err != nil {
		return nil, fmt.Errorf("update series: %w", err)
	}

	return updated, nil
}

///////////////////////////////////////////////////////////////////////////////
// GET / LIST / DELETE
///////////////////////////////////////////////////////////////////////////////

func (s *recurringService) GetSeries(ctx context.Context, orgID, id int64) (*recurring.RecurringSeries, error) {
	return s.repo.GetSeries(ctx, orgID, id)
}

func (s *recurringService) ListSeries(ctx context.Context, orgID int64) ([]*recurring.RecurringSeries, error) {
	return s.repo.ListSeries(ctx, orgID)
}

func (s *recurringService) ListSeriesByProperty(ctx context.Context, orgID, propertyID int64) ([]*recurring.RecurringSeries, error) {
	return s.repo.ListSeriesByProperty(ctx, orgID, propertyID)
}

func (s *recurringService) DeleteSeries(ctx context.Context, orgID, id int64) error {
	return s.repo.DeleteSeries(ctx, orgID, id)
}

///////////////////////////////////////////////////////////////////////////////
// PAUSE / RESUME
///////////////////////////////////////////////////////////////////////////////

func (s *recurringService) SetActive(ctx context.Context, orgID, id int64, active bool) error {
	return s.repo.SetActive(ctx, orgID, id, active)
}

///////////////////////////////////////////////////////////////////////////////
// RUN SERIES (scheduler-caller)
///////////////////////////////////////////////////////////////////////////////

func (s *recurringService) RunSeries(ctx context.Context, orgID, seriesID int64) error {
	series, err := s.repo.GetSeries(ctx, orgID, seriesID)
	if err != nil {
		return fmt.Errorf("get series: %w", err)
	}
	return s.runSeries(ctx, series)
}

// runSeries materializes the due occurrence as a task (a change from the
// source, whose RunSeries only advanced timestamps) and advances
// next_run_at/last_run_at.
func (s *recurringService) runSeries(ctx context.Context, series *recurring.RecurringSeries) error {
	// Skip disabled
	if !series.Active {
		return nil
	}

	now := time.Now()

	// Skip if expired by end date
	if series.EndDate != nil && now.After(*series.EndDate) {
		return nil
	}

	// Materialize the occurrence as a task.
	var dueDate *time.Time
	if series.DueAfterDays > 0 {
		d := now.AddDate(0, 0, series.DueAfterDays)
		dueDate = &d
	}
	taskID, err := s.repo.MaterializeTask(ctx, series, dueDate)
	if err != nil {
		return err
	}
	s.rec.Record(ctx, activity.Entry{
		OrgID:      series.OrgID,
		TicketType: "task",
		TicketID:   taskID,
		Kind:       activity.KindCreated,
		ActorType:  "system",
		Body:       fmt.Sprintf("Task %q created from recurring series #%d", series.Name, series.ID),
	})

	// Compute next occurrence
	nextRun, err := recurring.CalculateNextRun(
		series.Frequency,
		series.Interval,
		now,
		series.ByDay,
		series.DayOfMonth,
		series.WeekOfMonth,
		series.WeekdayOfMonth,
	)
	if err != nil {
		return err
	}

	// Write next runtime + lastrun back to DB
	if err := s.repo.UpdateRunTimes(ctx, series.OrgID, series.ID, nextRun, &now); err != nil {
		return fmt.Errorf("update runtimes: %w", err)
	}

	return nil
}

func normalizeCreateSeriesRequest(req recurring.CreateSeriesRequest) (int, *time.Time, error) {
	if err := validateRecurringConfig(req); err != nil {
		return 0, nil, err
	}

	dueAfterDays, err := normalizeDueAfterDays(req)
	if err != nil {
		return 0, nil, err
	}

	endDate, err := resolveEndDate(req)
	if err != nil {
		return 0, nil, err
	}

	return dueAfterDays, endDate, nil
}

func validateRecurringConfig(req recurring.CreateSeriesRequest) error {
	switch req.Frequency {
	case recurring.FrequencyDaily, recurring.FrequencyWeekly, recurring.FrequencyMonthly, recurring.FrequencyYearly:
	default:
		return fmt.Errorf("invalid frequency: %s", req.Frequency)
	}

	if req.Interval < 1 {
		return fmt.Errorf("interval must be >= 1")
	}

	if req.Frequency == recurring.FrequencyWeekly && len(req.ByDay) == 0 {
		return fmt.Errorf("weekly recurrence requires by_day")
	}

	if req.Frequency == recurring.FrequencyMonthly {
		hasDayOfMonth := req.DayOfMonth != nil
		hasWeekPattern := req.WeekOfMonth != nil || req.WeekdayOfMonth != nil

		if hasDayOfMonth && hasWeekPattern {
			return fmt.Errorf("monthly recurrence requires only one of day_of_month or week_of_month+weekday_of_month")
		}
		if !hasDayOfMonth && !hasWeekPattern {
			return fmt.Errorf("monthly recurrence requires day_of_month or week_of_month+weekday_of_month")
		}
		if hasWeekPattern && (req.WeekOfMonth == nil || req.WeekdayOfMonth == nil) {
			return fmt.Errorf("monthly recurrence requires both week_of_month and weekday_of_month")
		}
	}

	return nil
}

func normalizeDueAfterDays(req recurring.CreateSeriesRequest) (int, error) {
	if req.DueAfter != nil {
		if req.DueAfter.Count < 0 {
			return 0, fmt.Errorf("due_after.count must be >= 0")
		}
		switch req.DueAfter.Period {
		case "days":
			return req.DueAfter.Count, nil
		case "weeks":
			return req.DueAfter.Count * 7, nil
		case "months":
			return req.DueAfter.Count * 30, nil
		default:
			return 0, fmt.Errorf("due_after.period must be days, weeks, or months")
		}
	}

	if req.DueAfterDays < 0 {
		return 0, fmt.Errorf("due_after_days must be >= 0")
	}

	return req.DueAfterDays, nil
}

func resolveEndDate(req recurring.CreateSeriesRequest) (*time.Time, error) {
	endMode := req.EndMode
	if endMode == "" {
		endMode = "never"
	}

	switch endMode {
	case "never":
		return nil, nil
	case "on_date":
		if req.EndDate == nil {
			return nil, fmt.Errorf("end_date is required when end_mode is on_date")
		}
		return req.EndDate, nil
	case "after_n_times":
		if req.EndAfterOccurrences == nil || *req.EndAfterOccurrences < 1 {
			return nil, fmt.Errorf("end_after_occurrences must be >= 1 when end_mode is after_n_times")
		}
		endDate, err := calculateEndDateByOccurrences(
			req.StartDate,
			req.Frequency,
			req.Interval,
			req.ByDay,
			req.DayOfMonth,
			req.WeekOfMonth,
			req.WeekdayOfMonth,
			*req.EndAfterOccurrences,
		)
		if err != nil {
			return nil, fmt.Errorf("calculate end date: %w", err)
		}
		return &endDate, nil
	default:
		return nil, fmt.Errorf("invalid end_mode: %s", endMode)
	}
}

func calculateEndDateByOccurrences(
	startDate time.Time,
	frequency recurring.RecurringFrequency,
	interval int,
	byDay []string,
	dayOfMonth *int,
	weekOfMonth *int,
	weekdayOfMonth *string,
	occurrences int,
) (time.Time, error) {
	if occurrences < 1 {
		return time.Time{}, fmt.Errorf("occurrences must be >= 1")
	}

	current := startDate
	for i := 0; i < occurrences; i++ {
		nextRun, err := recurring.CalculateNextRun(
			frequency,
			interval,
			current,
			byDay,
			dayOfMonth,
			weekOfMonth,
			weekdayOfMonth,
		)
		if err != nil {
			return time.Time{}, err
		}
		current = nextRun
	}

	return current, nil
}
