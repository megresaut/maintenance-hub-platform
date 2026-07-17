package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"maintenancehub/modules/maintenance/recurring"
)

type RecurringRepositoryPG struct {
	db *pgxpool.Pool
}

func NewRecurringRepositoryPG(db *pgxpool.Pool) *RecurringRepositoryPG {
	return &RecurringRepositoryPG{db: db}
}

// NOTE: the source stored assignees/collaborators as bigint[][]; this port
// uses plain bigint[] (the nested array was an accident of the source schema
// and was always flattened to one dimension on read).

const seriesColumns = `
	id, org_id, property_id, unit_id, location_id, template_task_id,
	name, description, assignees, collaborators, priority, category,
	frequency, interval, by_day, day_of_month, week_of_month, weekday_of_month,
	start_date, end_date, due_after_days, active,
	next_run_at, last_run_at, created_at, updated_at`

func scanSeries(row pgx.Row) (*recurring.RecurringSeries, error) {
	var o recurring.RecurringSeries
	err := row.Scan(
		&o.ID, &o.OrgID, &o.PropertyID, &o.UnitID, &o.LocationID, &o.TemplateTaskID,
		&o.Name, &o.Description, &o.Assignees, &o.Collaborators, &o.Priority, &o.Category,
		&o.Frequency, &o.Interval, &o.ByDay, &o.DayOfMonth, &o.WeekOfMonth, &o.WeekdayOfMonth,
		&o.StartDate, &o.EndDate, &o.DueAfterDays, &o.Active,
		&o.NextRunAt, &o.LastRunAt, &o.CreatedAt, &o.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if o.Assignees == nil {
		o.Assignees = []int64{}
	}
	if o.Collaborators == nil {
		o.Collaborators = []int64{}
	}
	if o.ByDay == nil {
		o.ByDay = []string{}
	}
	return &o, nil
}

//
// CREATE
//

func (r *RecurringRepositoryPG) InsertSeries(ctx context.Context, orgID int64, s recurring.RecurringSeries) (*recurring.RecurringSeries, error) {
	if s.Assignees == nil {
		s.Assignees = []int64{}
	}
	if s.Collaborators == nil {
		s.Collaborators = []int64{}
	}
	if s.ByDay == nil {
		s.ByDay = []string{}
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO recurring_task_series (
			org_id, property_id, unit_id, location_id, template_task_id,
			name, description, assignees, collaborators, priority, category,
			frequency, interval, by_day, day_of_month, week_of_month, weekday_of_month,
			start_date, end_date, due_after_days, active, next_run_at, last_run_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)
		RETURNING `+seriesColumns,
		orgID, s.PropertyID, s.UnitID, s.LocationID, s.TemplateTaskID,
		s.Name, s.Description, s.Assignees, s.Collaborators, s.Priority, s.Category,
		s.Frequency, s.Interval, s.ByDay, s.DayOfMonth, s.WeekOfMonth, s.WeekdayOfMonth,
		s.StartDate, s.EndDate, s.DueAfterDays, s.Active, s.NextRunAt, s.LastRunAt,
	)
	out, err := scanSeries(row)
	if err != nil {
		return nil, fmt.Errorf("insert series: %w", err)
	}
	return out, nil
}

//
// GET ONE
//

func (r *RecurringRepositoryPG) GetSeries(ctx context.Context, orgID, id int64) (*recurring.RecurringSeries, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+seriesColumns+`
		FROM recurring_task_series
		WHERE id = $1 AND org_id = $2`, id, orgID)
	return scanSeries(row)
}

//
// LIST
//

func (r *RecurringRepositoryPG) list(ctx context.Context, query string, args ...any) ([]*recurring.RecurringSeries, error) {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list series: %w", err)
	}
	defer rows.Close()

	list := []*recurring.RecurringSeries{}
	for rows.Next() {
		s, err := scanSeries(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

func (r *RecurringRepositoryPG) ListSeries(ctx context.Context, orgID int64) ([]*recurring.RecurringSeries, error) {
	return r.list(ctx, `
		SELECT `+seriesColumns+`
		FROM recurring_task_series
		WHERE org_id = $1
		ORDER BY next_run_at ASC`, orgID)
}

func (r *RecurringRepositoryPG) ListSeriesByProperty(ctx context.Context, orgID, propertyID int64) ([]*recurring.RecurringSeries, error) {
	return r.list(ctx, `
		SELECT `+seriesColumns+`
		FROM recurring_task_series
		WHERE org_id = $1 AND property_id = $2
		ORDER BY next_run_at ASC`, orgID, propertyID)
}

//
// UPDATE
//

func (r *RecurringRepositoryPG) UpdateSeries(ctx context.Context, orgID, id int64, s recurring.RecurringSeries) (*recurring.RecurringSeries, error) {
	if s.Assignees == nil {
		s.Assignees = []int64{}
	}
	if s.Collaborators == nil {
		s.Collaborators = []int64{}
	}
	if s.ByDay == nil {
		s.ByDay = []string{}
	}
	row := r.db.QueryRow(ctx, `
		UPDATE recurring_task_series SET
			unit_id = $1,
			location_id = $2,
			name = $3,
			description = $4,
			assignees = $5,
			collaborators = $6,
			priority = $7,
			category = $8,
			frequency = $9,
			interval = $10,
			by_day = $11,
			day_of_month = $12,
			week_of_month = $13,
			weekday_of_month = $14,
			start_date = $15,
			end_date = $16,
			due_after_days = $17,
			active = $18,
			next_run_at = $19,
			last_run_at = $20,
			updated_at = NOW()
		WHERE id = $21 AND org_id = $22
		RETURNING `+seriesColumns,
		s.UnitID, s.LocationID, s.Name, s.Description, s.Assignees, s.Collaborators,
		s.Priority, s.Category, s.Frequency, s.Interval, s.ByDay,
		s.DayOfMonth, s.WeekOfMonth, s.WeekdayOfMonth,
		s.StartDate, s.EndDate, s.DueAfterDays, s.Active, s.NextRunAt, s.LastRunAt,
		id, orgID,
	)
	out, err := scanSeries(row)
	if err != nil {
		return nil, fmt.Errorf("update series: %w", err)
	}
	return out, nil
}

//
// PAUSE / RESUME
//

func (r *RecurringRepositoryPG) SetActive(ctx context.Context, orgID, id int64, active bool) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE recurring_task_series SET active=$1, updated_at=NOW() WHERE id=$2 AND org_id=$3`,
		active, id, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

//
// UPDATE next_run_at / last_run_at
//

func (r *RecurringRepositoryPG) UpdateRunTimes(ctx context.Context, orgID, id int64, next time.Time, last *time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE recurring_task_series
		 SET next_run_at=$1, last_run_at=$2, updated_at=NOW()
		 WHERE id=$3 AND org_id=$4`,
		next, last, id, orgID)
	return err
}

//
// DELETE
//

func (r *RecurringRepositoryPG) DeleteSeries(ctx context.Context, orgID, id int64) error {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM recurring_task_series WHERE id=$1 AND org_id=$2`, id, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

//
// POLLER SUPPORT
//

// ListDue returns every active series (all orgs) whose next_run_at has
// passed and whose end_date has not lapsed.
func (r *RecurringRepositoryPG) ListDue(ctx context.Context, now time.Time) ([]*recurring.RecurringSeries, error) {
	return r.list(ctx, `
		SELECT `+seriesColumns+`
		FROM recurring_task_series
		WHERE active = true
		  AND next_run_at <= $1
		  AND (end_date IS NULL OR end_date >= $1)
		ORDER BY next_run_at ASC`, now)
}

// MaterializeTask inserts a task row from the series template. The task
// inherits the series' org, property, assignees, priority and category.
func (r *RecurringRepositoryPG) MaterializeTask(ctx context.Context, s *recurring.RecurringSeries, dueDate *time.Time) (int64, error) {
	priority := "medium"
	if s.Priority != nil && *s.Priority != "" {
		priority = *s.Priority
	}
	category := ""
	if s.Category != nil {
		category = *s.Category
	}
	var taskID int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO tasks (
			org_id, property_id, name, description, status, priority, category,
			due_date, assignees, collaborators, unit_id, location_id
		)
		VALUES ($1,$2,$3,$4,'open',$5,$6,$7,$8,$9,$10,$11)
		RETURNING id`,
		s.OrgID, s.PropertyID, s.Name, s.Description, priority, category,
		dueDate, s.Assignees, s.Collaborators, s.UnitID, s.LocationID,
	).Scan(&taskID)
	if err != nil {
		return 0, fmt.Errorf("materialize task: %w", err)
	}
	return taskID, nil
}
