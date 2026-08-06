package repository

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/logger"
)

// settingsCacheTTL is short: an operator changing the commission expects it to
// take effect promptly, but every request must not hit the database either.
const settingsCacheTTL = 60 * time.Second

type settingsRepository struct {
	Base
	log logger.Logger

	// An in-process cache, not Redis: these values are read on nearly every
	// write path and the payload is a handful of scalars per node.
	mu      sync.RWMutex
	values  map[string]cachedSetting
	flags   map[string]bool
	flagsAt time.Time
}

type cachedSetting struct {
	raw       []byte
	expiresAt time.Time
}

// NewSettingsRepository builds the settings repository.
func NewSettingsRepository(db *gorm.DB, log logger.Logger) domain.SettingsRepository {
	return &settingsRepository{
		Base:   NewBase(db),
		log:    log,
		values: make(map[string]cachedSetting),
		flags:  make(map[string]bool),
	}
}

func (r *settingsRepository) Get(ctx context.Context, key string) (*domain.SystemSetting, error) {
	var setting domain.SystemSetting
	if err := r.DB(ctx).First(&setting, "key = ?", key).Error; err != nil {
		return nil, translate(err, "setting")
	}
	return &setting, nil
}

func (r *settingsRepository) List(ctx context.Context, group string, publicOnly bool) ([]domain.SystemSetting, error) {
	q := r.DB(ctx).Model(&domain.SystemSetting{})
	if group != "" {
		q = q.Where("group_name = ?", group)
	}
	if publicOnly {
		q = q.Where("is_public")
	}
	var settings []domain.SystemSetting
	if err := q.Order("group_name, key").Find(&settings).Error; err != nil {
		return nil, translate(err, "setting")
	}
	return settings, nil
}

// Set writes a value and drops it from the cache so the change is visible to
// this node immediately; other nodes pick it up within the TTL.
func (r *settingsRepository) Set(ctx context.Context, key string, value any, updatedBy *uuid.UUID) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return apperror.ErrValidation.WithMessage("The setting value is not serialisable").Wrap(err)
	}

	res := r.DB(ctx).Model(&domain.SystemSetting{}).
		Where("key = ?", key).
		Updates(map[string]any{"value": string(raw), "updated_by": updatedBy})
	if res.Error != nil {
		return translate(res.Error, "setting")
	}
	if res.RowsAffected == 0 {
		return apperror.ErrNotFound.WithMessage("Unknown setting: " + key)
	}

	r.mu.Lock()
	delete(r.values, key)
	r.mu.Unlock()
	return nil
}

// raw returns the cached JSON for a key, refreshing from the database on miss.
func (r *settingsRepository) raw(ctx context.Context, key string) ([]byte, error) {
	r.mu.RLock()
	cached, ok := r.values[key]
	r.mu.RUnlock()
	if ok && time.Now().Before(cached.expiresAt) {
		return cached.raw, nil
	}

	setting, err := r.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.values[key] = cachedSetting{
		raw:       []byte(setting.Value),
		expiresAt: time.Now().Add(settingsCacheTTL),
	}
	r.mu.Unlock()
	return []byte(setting.Value), nil
}

func (r *settingsRepository) String(ctx context.Context, key string) (string, error) {
	raw, err := r.raw(ctx, key)
	if err != nil {
		return "", err
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	// A value stored as a bare scalar rather than a JSON string still resolves.
	return strings.Trim(string(raw), `"`), nil
}

func (r *settingsRepository) Int(ctx context.Context, key string) (int64, error) {
	raw, err := r.raw(ctx, key)
	if err != nil {
		return 0, err
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, nil
	}
	parsed, err := strconv.ParseInt(strings.Trim(string(raw), `"`), 10, 64)
	if err != nil {
		return 0, apperror.ErrInternal.WithMessage("Setting " + key + " is not an integer").Wrap(err)
	}
	return parsed, nil
}

func (r *settingsRepository) Float(ctx context.Context, key string) (float64, error) {
	raw, err := r.raw(ctx, key)
	if err != nil {
		return 0, err
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return f, nil
	}
	parsed, err := strconv.ParseFloat(strings.Trim(string(raw), `"`), 64)
	if err != nil {
		return 0, apperror.ErrInternal.WithMessage("Setting " + key + " is not a number").Wrap(err)
	}
	return parsed, nil
}

func (r *settingsRepository) Bool(ctx context.Context, key string) (bool, error) {
	raw, err := r.raw(ctx, key)
	if err != nil {
		return false, err
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return b, nil
	}
	parsed, err := strconv.ParseBool(strings.Trim(string(raw), `"`))
	if err != nil {
		return false, apperror.ErrInternal.WithMessage("Setting " + key + " is not a boolean").Wrap(err)
	}
	return parsed, nil
}

// IsFeatureEnabled answers the flag question without an error return: a flag
// lookup failure must never break the feature check at a call site, so an
// unknown or unreadable flag is simply off.
func (r *settingsRepository) IsFeatureEnabled(ctx context.Context, key string) bool {
	r.mu.RLock()
	fresh := time.Now().Before(r.flagsAt.Add(settingsCacheTTL))
	enabled, known := r.flags[key]
	r.mu.RUnlock()
	if fresh && known {
		return enabled
	}

	flags, err := r.ListFlags(ctx)
	if err != nil {
		r.log.Warn("reading feature flags failed", logger.Err(err))
		return false
	}

	next := make(map[string]bool, len(flags))
	for _, f := range flags {
		// A flag is on when it is enabled and fully rolled out. Percentage
		// rollout is evaluated per-user by the caller, not here.
		next[f.Key] = f.IsEnabled && f.RolloutPercent >= 100
	}

	r.mu.Lock()
	r.flags = next
	r.flagsAt = time.Now()
	r.mu.Unlock()
	return next[key]
}

func (r *settingsRepository) ListFlags(ctx context.Context) ([]domain.FeatureFlag, error) {
	var flags []domain.FeatureFlag
	if err := r.DB(ctx).Order("key").Find(&flags).Error; err != nil {
		return nil, translate(err, "feature flag")
	}
	return flags, nil
}

func (r *settingsRepository) SetFlag(ctx context.Context, key string, enabled bool, rolloutPercent int16) error {
	if rolloutPercent < 0 {
		rolloutPercent = 0
	}
	if rolloutPercent > 100 {
		rolloutPercent = 100
	}
	res := r.DB(ctx).Model(&domain.FeatureFlag{}).
		Where("key = ?", key).
		Updates(map[string]any{"is_enabled": enabled, "rollout_percent": rolloutPercent})
	if res.Error != nil {
		return translate(res.Error, "feature flag")
	}
	if res.RowsAffected == 0 {
		return apperror.ErrNotFound.WithMessage("Unknown feature flag: " + key)
	}

	r.mu.Lock()
	r.flagsAt = time.Time{} // force a refresh on the next read
	r.mu.Unlock()
	return nil
}
