package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/logger"
)

// AuditLog is a row of audit_logs.
type AuditLog struct {
	ID           int64      `gorm:"primaryKey"`
	ActorUserID  *uuid.UUID `gorm:"type:uuid"`
	ActorRole    *string    `gorm:"size:32"`
	Action       string     `gorm:"size:64;not null"`
	ResourceType string     `gorm:"size:48;not null"`
	ResourceID   *uuid.UUID `gorm:"type:uuid"`
	OldValues    []byte     `gorm:"type:jsonb"`
	NewValues    []byte     `gorm:"type:jsonb"`
	Description  *string
	IPAddress    *string `gorm:"type:inet"`
	UserAgent    *string
	RequestID    *string `gorm:"size:64"`
	CreatedAt    time.Time
}

// TableName pins the table since the struct name would pluralise to audit_logs
// only by convention.
func (AuditLog) TableName() string { return "audit_logs" }

type auditRepository struct {
	Base
	log logger.Logger
}

// NewAuditLogger builds the audit sink.
func NewAuditLogger(db *gorm.DB, log logger.Logger) domain.AuditLogger {
	return &auditRepository{Base: NewBase(db), log: log}
}

// Record writes one audit row. A failure here must never fail the caller's
// operation, so the error is logged and swallowed — but it is logged loudly.
func (r *auditRepository) Record(ctx context.Context, entry domain.AuditEntry) error {
	row := AuditLog{
		ActorUserID:  entry.ActorUserID,
		Action:       entry.Action,
		ResourceType: entry.ResourceType,
		ResourceID:   entry.ResourceID,
		OldValues:    marshalOrNil(entry.OldValues),
		NewValues:    marshalOrNil(entry.NewValues),
	}
	row.ActorRole = nilIfEmpty(entry.ActorRole)
	row.Description = nilIfEmpty(entry.Description)
	row.IPAddress = nilIfEmpty(entry.IPAddress)
	row.UserAgent = nilIfEmpty(entry.UserAgent)
	row.RequestID = nilIfEmpty(entry.RequestID)

	if err := r.DB(ctx).Create(&row).Error; err != nil {
		r.log.Error("audit write failed",
			logger.String("action", entry.Action),
			logger.String("resource", entry.ResourceType),
			logger.Err(err))
		return nil
	}
	return nil
}

// ListAuditLogs powers the admin audit screen.
func (r *auditRepository) ListAuditLogs(ctx context.Context, resourceType string, resourceID *uuid.UUID, actorID *uuid.UUID, offset, limit int) ([]AuditLog, int64, error) {
	q := r.DB(ctx).Model(&AuditLog{})
	if resourceType != "" {
		q = q.Where("resource_type = ?", resourceType)
	}
	if resourceID != nil {
		q = q.Where("resource_id = ?", *resourceID)
	}
	if actorID != nil {
		q = q.Where("actor_user_id = ?", *actorID)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "audit log")
	}

	var rows []AuditLog
	err := applyPage(q.Order("created_at DESC"), offset, limit).Find(&rows).Error
	if err != nil {
		return nil, 0, translate(err, "audit log")
	}
	return rows, total, nil
}

func marshalOrNil(m map[string]any) []byte {
	if len(m) == 0 {
		return nil
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return raw
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
