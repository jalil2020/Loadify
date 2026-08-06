package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// SystemSetting is one operator-tunable value.
//
// Anything a business person might want to change without a deploy lives here —
// commission percentage, load expiry, tracking intervals — rather than as a Go
// constant.
type SystemSetting struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Key         string    `gorm:"size:64;not null;uniqueIndex"`
	Value       JSONRaw   `gorm:"type:jsonb;not null"`
	ValueType   string    `gorm:"size:16;not null;default:'string'"`
	GroupName   string    `gorm:"size:32;not null;default:'general'"`
	Description *string
	IsPublic    bool       `gorm:"not null;default:false"`
	UpdatedBy   *uuid.UUID `gorm:"type:uuid"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TableName pins the settings table.
func (SystemSetting) TableName() string { return "system_settings" }

// FeatureFlag gates a capability without a deploy.
type FeatureFlag struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Key            string    `gorm:"size:64;not null;uniqueIndex"`
	Description    *string
	IsEnabled      bool    `gorm:"not null;default:false"`
	RolloutPercent int16   `gorm:"not null;default:0"`
	Conditions     JSONMap `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// SettingsReader reads typed configuration. Usecases depend on this narrow port
// rather than the full repository, so a usecase cannot accidentally write settings.
type SettingsReader interface {
	String(ctx context.Context, key string) (string, error)
	Int(ctx context.Context, key string) (int64, error)
	Float(ctx context.Context, key string) (float64, error)
	Bool(ctx context.Context, key string) (bool, error)
	IsFeatureEnabled(ctx context.Context, key string) bool
}

// SettingsRepository is the full settings port, including writes for admins.
type SettingsRepository interface {
	SettingsReader
	Get(ctx context.Context, key string) (*SystemSetting, error)
	List(ctx context.Context, group string, publicOnly bool) ([]SystemSetting, error)
	Set(ctx context.Context, key string, value any, updatedBy *uuid.UUID) error
	ListFlags(ctx context.Context) ([]FeatureFlag, error)
	SetFlag(ctx context.Context, key string, enabled bool, rolloutPercent int16) error
}
