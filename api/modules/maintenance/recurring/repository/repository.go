package repository

import (
	"context"
	"errors"
	"time"

	"maintenancehub/modules/maintenance/recurring"
)

var ErrNotFound = errors.New("recurring series not found")

type RecurringRepository interface {
	InsertSeries(ctx context.Context, orgID int64, s recurring.RecurringSeries) (*recurring.RecurringSeries, error)
	GetSeries(ctx context.Context, orgID, id int64) (*recurring.RecurringSeries, error)
	ListSeries(ctx context.Context, orgID int64) ([]*recurring.RecurringSeries, error)
	ListSeriesByProperty(ctx context.Context, orgID, propertyID int64) ([]*recurring.RecurringSeries, error)
	UpdateSeries(ctx context.Context, orgID, id int64, s recurring.RecurringSeries) (*recurring.RecurringSeries, error)
	SetActive(ctx context.Context, orgID, id int64, active bool) error
	UpdateRunTimes(ctx context.Context, orgID, id int64, next time.Time, last *time.Time) error
	DeleteSeries(ctx context.Context, orgID, id int64) error

	// ListDue is used by the poller: all active series (across every org)
	// whose next_run_at is in the past and end_date has not lapsed.
	ListDue(ctx context.Context, now time.Time) ([]*recurring.RecurringSeries, error)

	// MaterializeTask inserts a task from a due series occurrence and
	// returns its id. dueDate is nil when due_after_days is 0.
	MaterializeTask(ctx context.Context, s *recurring.RecurringSeries, dueDate *time.Time) (int64, error)
}
