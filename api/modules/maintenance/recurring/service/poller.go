package service

import (
	"context"
	"log"
	"time"

	"maintenancehub/modules/maintenance/recurring/repository"
)

// Poller materializes every active recurring series whose scheduled run time
// is in the past into a real task and advances `next_run_at`. Without this,
// series stay frozen until someone manually hits POST /recurring/{id}/run.
type Poller struct {
	repo     repository.RecurringRepository
	svc      *recurringService
	interval time.Duration
}

func NewPoller(repo repository.RecurringRepository, svc *recurringService, interval time.Duration) *Poller {
	return &Poller{repo: repo, svc: svc, interval: interval}
}

// Start runs the poller loop. Initial sweep on boot, then every interval.
// Run it in a goroutine: `go poller.Start(ctx)`.
func (p *Poller) Start(ctx context.Context) {
	log.Printf("[recurring-poller] started (interval=%s)", p.interval)
	p.runOnce(ctx)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[recurring-poller] stopped")
			return
		case <-ticker.C:
			p.runOnce(ctx)
		}
	}
}

func (p *Poller) runOnce(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[recurring-poller] panic recovered: %v", r)
		}
	}()

	due, err := p.repo.ListDue(ctx, time.Now())
	if err != nil {
		log.Printf("[recurring-poller] list due series error: %v", err)
		return
	}

	advanced := 0
	for _, s := range due {
		if s == nil {
			continue
		}
		if err := p.svc.runSeries(ctx, s); err != nil {
			log.Printf("[recurring-poller] RunSeries(%d) error: %v", s.ID, err)
			continue
		}
		advanced++
	}
	if advanced > 0 {
		log.Printf("[recurring-poller] materialized %d series occurrence(s)", advanced)
	}
}
