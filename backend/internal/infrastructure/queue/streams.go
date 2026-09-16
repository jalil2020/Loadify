// Package queue carries job ids from the relay to the workers over a Redis
// stream.
//
// It is delivery and nothing more. Postgres holds every job and its state, so a
// lost stream costs latency rather than work: the relay finds due jobs again
// and hands them over again. What the stream adds is a consumer group — several
// workers reading one stream without coordinating — and a record of what each
// took and never acknowledged, which is how a crashed worker's jobs get picked up.
package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/loadify/backend/pkg/logger"
)

const (
	streamKey = "posting:jobs"
	groupName = "posters"
	fieldJob  = "job_id"

	// maxStreamLength bounds memory. Acknowledged entries are history the
	// database already keeps, so trimming them loses nothing.
	maxStreamLength = 100_000
	batchSize       = 10
)

// Streams implements domain.JobQueue.
type Streams struct {
	client    *redis.Client
	consumer  string
	block     time.Duration
	claimIdle time.Duration
	// handlerBudget bounds one job. It is detached from the consumer's own
	// context on purpose: shutting down must let a send that has started
	// finish and be recorded, not cut it off halfway.
	handlerBudget time.Duration
	log           logger.Logger
}

// NewStreams builds the queue for one consumer. claimIdle is how long an entry
// may sit unacknowledged before another consumer takes it over; it should be
// longer than a job's lease, so a slow worker is not raced by a healthy one.
func NewStreams(client *redis.Client, consumer string, claimIdle, handlerBudget time.Duration, log logger.Logger) *Streams {
	return &Streams{
		client:        client,
		consumer:      consumer,
		block:         5 * time.Second,
		claimIdle:     claimIdle,
		handlerBudget: handlerBudget,
		log:           log,
	}
}

// Publish hands one job to the workers.
func (s *Streams) Publish(ctx context.Context, jobID uuid.UUID) error {
	err := s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey,
		MaxLen: maxStreamLength,
		Approx: true,
		Values: map[string]any{fieldJob: jobID.String()},
	}).Err()
	if err != nil {
		return fmt.Errorf("queue: publish: %w", err)
	}
	return nil
}

// Consume reads jobs until ctx ends.
//
// An entry is acknowledged only after its handler returns without error. One
// that fails stays pending and is reclaimed after claimIdle — by this consumer
// or another — which is the retry for a worker that crashed rather than one
// that decided to wait. Deciding to wait is the database's business, not the
// stream's.
func (s *Streams) Consume(ctx context.Context, handler func(context.Context, uuid.UUID) error) error {
	if err := s.ensureGroup(ctx); err != nil {
		return err
	}

	for ctx.Err() == nil {
		s.reclaim(ctx, handler)

		streams, err := s.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    groupName,
			Consumer: s.consumer,
			Streams:  []string{streamKey, ">"},
			Count:    batchSize,
			Block:    s.block,
		}).Result()
		switch {
		case errors.Is(err, redis.Nil):
			continue
		case err != nil:
			if ctx.Err() != nil {
				return nil
			}
			s.log.Warn("queue read failed, retrying", logger.Err(err))
			sleep(ctx, time.Second)
			continue
		}

		for _, stream := range streams {
			for _, message := range stream.Messages {
				s.handle(ctx, message, handler)
			}
		}
	}
	return nil
}

func (s *Streams) ensureGroup(ctx context.Context) error {
	err := s.client.XGroupCreateMkStream(ctx, streamKey, groupName, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("queue: create consumer group: %w", err)
	}
	return nil
}

// reclaim takes over entries another consumer read and never acknowledged.
func (s *Streams) reclaim(ctx context.Context, handler func(context.Context, uuid.UUID) error) {
	messages, _, err := s.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   streamKey,
		Group:    groupName,
		Consumer: s.consumer,
		MinIdle:  s.claimIdle,
		Start:    "0-0",
		Count:    batchSize,
	}).Result()
	if err != nil {
		if ctx.Err() == nil && !errors.Is(err, redis.Nil) {
			s.log.Warn("queue reclaim failed", logger.Err(err))
		}
		return
	}
	for _, message := range messages {
		s.handle(ctx, message, handler)
	}
}

func (s *Streams) handle(ctx context.Context, message redis.XMessage, handler func(context.Context, uuid.UUID) error) {
	raw, _ := message.Values[fieldJob].(string)
	jobID, err := uuid.Parse(raw)
	if err != nil {
		// Nothing can ever process this entry, so leaving it pending would only
		// have it reclaimed forever.
		s.log.Error("dropping a malformed queue entry", logger.String("entry", message.ID))
		s.ack(message.ID)
		return
	}

	jobCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.handlerBudget)
	defer cancel()

	if err := handler(jobCtx, jobID); err != nil {
		s.log.Warn("job left pending for another attempt",
			logger.String("job_id", jobID.String()), logger.Err(err))
		return
	}
	s.ack(message.ID)
}

// ack uses its own short context: an acknowledgement for work that finished
// must still be written while the process is shutting down.
func (s *Streams) ack(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.client.XAck(ctx, streamKey, groupName, id).Err(); err != nil {
		s.log.Warn("queue acknowledgement failed", logger.String("entry", id), logger.Err(err))
	}
}

func sleep(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
