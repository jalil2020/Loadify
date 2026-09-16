package queue_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/loadify/backend/internal/infrastructure/queue"
	"github.com/loadify/backend/pkg/logger"
)

func newClient(t *testing.T) *redis.Client {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func newStreams(client *redis.Client, consumer string, claimIdle time.Duration) *queue.Streams {
	return queue.NewStreams(client, consumer, claimIdle, 5*time.Second, logger.New("test", "error"))
}

// collector records what a handler saw, and stops the consumer once it has seen
// enough.
type collector struct {
	mu     sync.Mutex
	seen   []uuid.UUID
	want   int
	cancel context.CancelFunc
	fail   func(uuid.UUID) error
}

func (c *collector) handle(_ context.Context, id uuid.UUID) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seen = append(c.seen, id)
	if len(c.seen) >= c.want {
		c.cancel()
	}
	if c.fail != nil {
		return c.fail(id)
	}
	return nil
}

func consume(t *testing.T, streams *queue.Streams, c *collector) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c.cancel = cancel
	if err := streams.Consume(ctx, c.handle); err != nil {
		t.Fatalf("Consume: %v", err)
	}
}

func TestPublishedJobsReachTheWorker(t *testing.T) {
	client := newClient(t)
	streams := newStreams(client, "worker-a", time.Minute)

	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	for _, id := range ids {
		if err := streams.Publish(context.Background(), id); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	c := &collector{want: len(ids)}
	consume(t, streams, c)

	if len(c.seen) != len(ids) {
		t.Fatalf("handled %d jobs, want %d", len(c.seen), len(ids))
	}
	for i, id := range ids {
		if c.seen[i] != id {
			t.Errorf("job %d = %s, want %s: order matters to nobody but it shows nothing was lost", i, c.seen[i], id)
		}
	}

	pending, err := client.XPending(context.Background(), "posting:jobs", "posters").Result()
	if err != nil {
		t.Fatalf("XPending: %v", err)
	}
	if pending.Count != 0 {
		t.Errorf("%d entries left unacknowledged after success", pending.Count)
	}
}

// A handler that fails leaves its entry pending, so the work is not lost with
// the worker that dropped it.
func TestAFailedJobStaysPending(t *testing.T) {
	client := newClient(t)
	streams := newStreams(client, "worker-a", time.Hour)
	if err := streams.Publish(context.Background(), uuid.New()); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	c := &collector{want: 1, fail: func(uuid.UUID) error { return errors.New("database unavailable") }}
	consume(t, streams, c)

	pending, err := client.XPending(context.Background(), "posting:jobs", "posters").Result()
	if err != nil {
		t.Fatalf("XPending: %v", err)
	}
	if pending.Count != 1 {
		t.Errorf("%d entries pending, want the failed one", pending.Count)
	}
}

// A worker that took an entry and died leaves it pending; another consumer
// takes it over once it has been idle long enough.
func TestAnotherWorkerReclaimsAnAbandonedJob(t *testing.T) {
	client := newClient(t)
	abandoned := uuid.New()

	crashed := newStreams(client, "worker-crashed", time.Hour)
	if err := crashed.Publish(context.Background(), abandoned); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	failing := &collector{want: 1, fail: func(uuid.UUID) error { return errors.New("process killed") }}
	consume(t, crashed, failing)

	// Zero idle time stands in for "long enough": the point is that a second
	// consumer picks up what the first never acknowledged.
	survivor := newStreams(client, "worker-survivor", 0)
	c := &collector{want: 1}
	consume(t, survivor, c)

	if len(c.seen) != 1 || c.seen[0] != abandoned {
		t.Errorf("the surviving worker handled %v, want the abandoned job", c.seen)
	}
}

// Shutdown must let a send that has started finish and be recorded. The
// handler therefore runs on a context the consumer's cancellation does not
// reach.
func TestShutdownDoesNotCutAJobOff(t *testing.T) {
	client := newClient(t)
	streams := newStreams(client, "worker-a", time.Minute)
	if err := streams.Publish(context.Background(), uuid.New()); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var handlerErr error
	err := streams.Consume(ctx, func(jobCtx context.Context, _ uuid.UUID) error {
		cancel() // the process is asked to stop mid-job
		select {
		case <-jobCtx.Done():
			handlerErr = jobCtx.Err()
		case <-time.After(50 * time.Millisecond):
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if handlerErr != nil {
		t.Errorf("the job's context was cancelled with the consumer: %v", handlerErr)
	}
}
