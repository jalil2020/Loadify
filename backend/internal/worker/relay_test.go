package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/logger"
)

// duePosting hands out a fixed backlog in batches, the way the database would.
type duePosting struct {
	postingStore
	backlog []domain.PostingJob
}

func (d *duePosting) DueJobs(_ context.Context, limit int) ([]domain.PostingJob, error) {
	n := min(limit, len(d.backlog))
	batch := d.backlog[:n]
	d.backlog = d.backlog[n:]
	return batch, nil
}

type recordingQueue struct {
	published []uuid.UUID
	failFor   uuid.UUID
}

func (q *recordingQueue) Publish(_ context.Context, id uuid.UUID) error {
	if id == q.failFor {
		return errors.New("redis unavailable")
	}
	q.published = append(q.published, id)
	return nil
}

func (q *recordingQueue) Consume(context.Context, func(context.Context, uuid.UUID) error) error {
	return nil
}

func backlog(n int) []domain.PostingJob {
	jobs := make([]domain.PostingJob, n)
	for i := range jobs {
		jobs[i].ID = uuid.New()
	}
	return jobs
}

// A backlog drains in one tick rather than a batch per second.
func TestRelayDrainsABacklogInOneTick(t *testing.T) {
	posting := &duePosting{backlog: backlog(250)}
	q := &recordingQueue{}
	relay := NewRelay(posting, q, time.Second, 100, logger.New("test", "error"))

	if got := relay.Tick(context.Background()); got != 250 {
		t.Errorf("relayed %d jobs, want 250", got)
	}
	if len(q.published) != 250 {
		t.Errorf("queue received %d, want 250", len(q.published))
	}
}

// A job that could not be handed over is not lost: the database stamp expires
// and a later tick finds it again. The rest of the batch still goes.
func TestRelayCarriesOnPastAFailedPublish(t *testing.T) {
	jobs := backlog(3)
	posting := &duePosting{backlog: jobs}
	q := &recordingQueue{failFor: jobs[1].ID}
	relay := NewRelay(posting, q, time.Second, 100, logger.New("test", "error"))

	if got := relay.Tick(context.Background()); got != 2 {
		t.Errorf("relayed %d jobs, want the 2 that published", got)
	}
}
