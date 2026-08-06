package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/loadify/backend/internal/domain"
)

type sessionRepository struct{ Base }

// NewSessionRepository builds the session repository.
func NewSessionRepository(db *gorm.DB) domain.SessionRepository {
	return &sessionRepository{Base: NewBase(db)}
}

func (r *sessionRepository) Create(ctx context.Context, session *domain.Session) error {
	return translate(r.DB(ctx).Create(session).Error, "session")
}

func (r *sessionRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*domain.Session, error) {
	var session domain.Session
	if err := r.DB(ctx).First(&session, "refresh_token_hash = ?", tokenHash).Error; err != nil {
		return nil, translate(err, "session")
	}
	return &session, nil
}

func (r *sessionRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Session, error) {
	var session domain.Session
	if err := r.DB(ctx).First(&session, "id = ?", id).Error; err != nil {
		return nil, translate(err, "session")
	}
	return &session, nil
}

func (r *sessionRepository) ListActiveByUser(ctx context.Context, userID uuid.UUID) ([]domain.Session, error) {
	var sessions []domain.Session
	err := r.DB(ctx).
		Where("user_id = ? AND revoked_at IS NULL AND expires_at > now()", userID).
		Order("last_used_at DESC").
		Find(&sessions).Error
	if err != nil {
		return nil, translate(err, "session")
	}
	return sessions, nil
}

func (r *sessionRepository) Revoke(ctx context.Context, id uuid.UUID, reason string) error {
	err := r.DB(ctx).Model(&domain.Session{}).
		Where("id = ? AND revoked_at IS NULL", id).
		Updates(map[string]any{"revoked_at": time.Now().UTC(), "revoked_reason": reason}).Error
	return translate(err, "session")
}

// RevokeFamily kills every token descended from one login. This is the response
// to refresh-token reuse: if a rotated token is presented again, either the
// client or an attacker holds a copy, and the whole chain must die.
func (r *sessionRepository) RevokeFamily(ctx context.Context, familyID uuid.UUID, reason string) error {
	err := r.DB(ctx).Model(&domain.Session{}).
		Where("family_id = ? AND revoked_at IS NULL", familyID).
		Updates(map[string]any{"revoked_at": time.Now().UTC(), "revoked_reason": reason}).Error
	return translate(err, "session")
}

func (r *sessionRepository) RevokeAllOfUser(ctx context.Context, userID uuid.UUID, reason string) error {
	err := r.DB(ctx).Model(&domain.Session{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Updates(map[string]any{"revoked_at": time.Now().UTC(), "revoked_reason": reason}).Error
	return translate(err, "session")
}

func (r *sessionRepository) MarkReplaced(ctx context.Context, oldID, newID uuid.UUID) error {
	err := r.DB(ctx).Model(&domain.Session{}).
		Where("id = ?", oldID).
		Update("replaced_by", newID).Error
	return translate(err, "session")
}

func (r *sessionRepository) TouchLastUsed(ctx context.Context, id uuid.UUID) error {
	err := r.DB(ctx).Model(&domain.Session{}).
		Where("id = ?", id).
		Update("last_used_at", time.Now().UTC()).Error
	return translate(err, "session")
}

func (r *sessionRepository) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	res := r.DB(ctx).
		Where("expires_at < ? OR (revoked_at IS NOT NULL AND revoked_at < ?)", before, before).
		Delete(&domain.Session{})
	if res.Error != nil {
		return 0, translate(res.Error, "session")
	}
	return res.RowsAffected, nil
}
