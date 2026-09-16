package worker

import (
	"context"
	"time"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/logger"
)

// Relay moves due jobs from the database onto the queue.
//
// It is the only producer, and it is deliberately dumb: Postgres decides when a
// job is due, and the relay only carries the decision across. That keeps one
// place for scheduling — next_attempt_at — rather than a delay in the database
// and another in Redis that could disagree.
type Relay struct {
	posting  domain.PostingRepository
	queue    domain.JobQueue
	interval time.Duration
	batch    int
	log      logger.Logger
}

// NewRelay builds the relay.
func NewRelay(posting domain.PostingRepository, queue domain.JobQueue, interval time.Duration, batch int, log logger.Logger) *Relay {
	if interval <= 0 {
		interval = time.Second
	}
	if batch <= 0 {
		batch = 100
	}
	return &Relay{posting: posting, queue: queue, interval: interval, batch: batch, log: log}
}

// Run relays until ctx ends.
func (r *Relay) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.Tick(ctx)
		}
	}
}

// Tick relays one batch, and keeps going while batches come back full so a
// backlog drains without waiting a second per hundred jobs.
func (r *Relay) Tick(ctx context.Context) int {
	total := 0
	for ctx.Err() == nil {
		jobs, err := r.posting.DueJobs(ctx, r.batch)
		if err != nil {
			r.log.Warn("due jobs could not be read", logger.Err(err))
			return total
		}

		for _, job := range jobs {
			// A job that fails to publish is not lost: it is stamped as
			// handed over, and the stamp expires, after which the next tick
			// finds it again.
			if err := r.queue.Publish(ctx, job.ID); err != nil {
				r.log.Warn("job not handed to the queue",
					logger.String("job_id", job.ID.String()), logger.Err(err))
				continue
			}
			total++
		}

		if len(jobs) < r.batch {
			return total
		}
	}
	return total
}
