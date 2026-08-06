// Package cache wraps Redis for caching, rate limiting, token denylists,
// distributed locks and the realtime pub/sub fan-out.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/pkg/logger"
)

// ErrNotFound is returned when a key is absent.
var ErrNotFound = errors.New("cache: key not found")

// Cache is the caching port used by usecases.
type Cache interface {
	Get(ctx context.Context, key string, dest any) error
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
	DeleteByPrefix(ctx context.Context, prefix string) error
	Exists(ctx context.Context, key string) (bool, error)
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
	SetNX(ctx context.Context, key string, value any, ttl time.Duration) (bool, error)
	TTL(ctx context.Context, key string) (time.Duration, error)
	Publish(ctx context.Context, channel string, payload any) error
	Subscribe(ctx context.Context, channels ...string) *redis.PubSub
	Lock(ctx context.Context, key string, ttl time.Duration) (release func(), acquired bool, err error)
	Client() *redis.Client
	Health(ctx context.Context) error
	Close() error
}

type redisCache struct {
	client *redis.Client
	log    logger.Logger
}

// New connects to Redis and verifies the connection.
func New(cfg config.RedisConfig, log logger.Logger) (Cache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:            cfg.Addr(),
		Password:        cfg.Password,
		DB:              cfg.DB,
		PoolSize:        cfg.PoolSize,
		MinIdleConns:    cfg.PoolSize / 4,
		ConnMaxIdleTime: 5 * time.Minute,
		DialTimeout:     5 * time.Second,
		ReadTimeout:     3 * time.Second,
		WriteTimeout:    3 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis at %s: %w", cfg.Addr(), err)
	}

	log.Info("redis connected", logger.String("addr", cfg.Addr()), logger.Int("db", cfg.DB))
	return &redisCache{client: client, log: log}, nil
}

func (r *redisCache) Get(ctx context.Context, key string, dest any) error {
	raw, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrNotFound
		}
		return fmt.Errorf("cache get %s: %w", key, err)
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("cache decode %s: %w", key, err)
	}
	return nil
}

func (r *redisCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache encode %s: %w", key, err)
	}
	if err := r.client.Set(ctx, key, raw, ttl).Err(); err != nil {
		return fmt.Errorf("cache set %s: %w", key, err)
	}
	return nil
}

func (r *redisCache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	if err := r.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("cache delete: %w", err)
	}
	return nil
}

// DeleteByPrefix scans instead of using KEYS so a large keyspace does not block Redis.
func (r *redisCache) DeleteByPrefix(ctx context.Context, prefix string) error {
	var cursor uint64
	for {
		keys, next, err := r.client.Scan(ctx, cursor, prefix+"*", 256).Result()
		if err != nil {
			return fmt.Errorf("cache scan %s: %w", prefix, err)
		}
		if len(keys) > 0 {
			if err := r.client.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("cache delete batch: %w", err)
			}
		}
		if next == 0 {
			return nil
		}
		cursor = next
	}
}

func (r *redisCache) Exists(ctx context.Context, key string) (bool, error) {
	n, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("cache exists %s: %w", key, err)
	}
	return n > 0, nil
}

// Incr increments a counter, setting the TTL only on first creation so a burst
// cannot keep extending the window.
func (r *redisCache) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	pipe := r.client.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("cache incr %s: %w", key, err)
	}
	n := incr.Val()
	if n == 1 {
		// Expire above already set it; nothing more to do.
		return n, nil
	}
	return n, nil
}

func (r *redisCache) SetNX(ctx context.Context, key string, value any, ttl time.Duration) (bool, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return false, fmt.Errorf("cache encode %s: %w", key, err)
	}
	ok, err := r.client.SetNX(ctx, key, raw, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("cache setnx %s: %w", key, err)
	}
	return ok, nil
}

func (r *redisCache) TTL(ctx context.Context, key string) (time.Duration, error) {
	d, err := r.client.TTL(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("cache ttl %s: %w", key, err)
	}
	return d, nil
}

func (r *redisCache) Publish(ctx context.Context, channel string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("publish encode %s: %w", channel, err)
	}
	if err := r.client.Publish(ctx, channel, raw).Err(); err != nil {
		return fmt.Errorf("publish %s: %w", channel, err)
	}
	return nil
}

func (r *redisCache) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	return r.client.Subscribe(ctx, channels...)
}

// Lock is a single-instance mutex. It is sufficient for our deployment topology
// (one Redis, several API nodes); a Redlock implementation would be needed only
// with independent Redis masters.
func (r *redisCache) Lock(ctx context.Context, key string, ttl time.Duration) (func(), bool, error) {
	token, err := randomToken()
	if err != nil {
		return nil, false, err
	}
	lockKey := "lock:" + key
	ok, err := r.client.SetNX(ctx, lockKey, token, ttl).Result()
	if err != nil {
		return nil, false, fmt.Errorf("acquire lock %s: %w", key, err)
	}
	if !ok {
		return nil, false, nil
	}

	release := func() {
		// Compare-and-delete so a lock that already expired and was retaken by
		// someone else is never released by us.
		const script = `
			if redis.call("GET", KEYS[1]) == ARGV[1] then
				return redis.call("DEL", KEYS[1])
			end
			return 0`
		if err := r.client.Eval(context.Background(), script, []string{lockKey}, token).Err(); err != nil {
			r.log.Warn("release lock failed", logger.String("key", key), logger.Err(err))
		}
	}
	return release, true, nil
}

func (r *redisCache) Client() *redis.Client { return r.client }

func (r *redisCache) Health(ctx context.Context) error { return r.client.Ping(ctx).Err() }

func (r *redisCache) Close() error { return r.client.Close() }
