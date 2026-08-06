package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/loadify/backend/internal/domain"
)

type otpRepository struct{ Base }

// NewOTPRepository builds the one-time-code repository.
func NewOTPRepository(db *gorm.DB) domain.OTPRepository {
	return &otpRepository{Base: NewBase(db)}
}

func (r *otpRepository) Create(ctx context.Context, code *domain.OTPCode) error {
	return translate(r.DB(ctx).Create(code).Error, "confirmation code")
}

// FindLatestActive returns the newest unconsumed, unexpired code for the phone
// and purpose. Older codes for the same pair are invalidated on issue, so at
// most one is ever live.
func (r *otpRepository) FindLatestActive(ctx context.Context, phone string, purpose domain.OTPPurpose) (*domain.OTPCode, error) {
	var code domain.OTPCode
	err := r.DB(ctx).
		Where("phone = ? AND purpose = ? AND consumed_at IS NULL AND expires_at > now()", phone, purpose).
		Order("created_at DESC").
		First(&code).Error
	if err != nil {
		return nil, translate(err, "confirmation code")
	}
	return &code, nil
}

func (r *otpRepository) IncrementAttempts(ctx context.Context, id uuid.UUID) error {
	err := r.DB(ctx).Model(&domain.OTPCode{}).
		Where("id = ?", id).
		UpdateColumn("attempts", gorm.Expr("attempts + 1")).Error
	return translate(err, "confirmation code")
}

func (r *otpRepository) Consume(ctx context.Context, id uuid.UUID) error {
	err := r.DB(ctx).Model(&domain.OTPCode{}).
		Where("id = ? AND consumed_at IS NULL", id).
		Update("consumed_at", time.Now().UTC()).Error
	return translate(err, "confirmation code")
}

func (r *otpRepository) InvalidateAll(ctx context.Context, phone string, purpose domain.OTPPurpose) error {
	err := r.DB(ctx).Model(&domain.OTPCode{}).
		Where("phone = ? AND purpose = ? AND consumed_at IS NULL", phone, purpose).
		Update("consumed_at", time.Now().UTC()).Error
	return translate(err, "confirmation code")
}

func (r *otpRepository) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	res := r.DB(ctx).Where("expires_at < ?", before).Delete(&domain.OTPCode{})
	if res.Error != nil {
		return 0, translate(res.Error, "confirmation code")
	}
	return res.RowsAffected, nil
}

type passwordResetRepository struct{ Base }

// NewPasswordResetRepository builds the password-reset repository.
func NewPasswordResetRepository(db *gorm.DB) domain.PasswordResetRepository {
	return &passwordResetRepository{Base: NewBase(db)}
}

func (r *passwordResetRepository) Create(ctx context.Context, reset *domain.PasswordReset) error {
	return translate(r.DB(ctx).Create(reset).Error, "password reset")
}

func (r *passwordResetRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*domain.PasswordReset, error) {
	var reset domain.PasswordReset
	err := r.DB(ctx).First(&reset, "token_hash = ?", tokenHash).Error
	if err != nil {
		return nil, translate(err, "password reset")
	}
	return &reset, nil
}

func (r *passwordResetRepository) MarkUsed(ctx context.Context, id uuid.UUID) error {
	err := r.DB(ctx).Model(&domain.PasswordReset{}).
		Where("id = ? AND used_at IS NULL", id).
		Update("used_at", time.Now().UTC()).Error
	return translate(err, "password reset")
}

func (r *passwordResetRepository) InvalidateForUser(ctx context.Context, userID uuid.UUID) error {
	err := r.DB(ctx).Model(&domain.PasswordReset{}).
		Where("user_id = ? AND used_at IS NULL", userID).
		Update("used_at", time.Now().UTC()).Error
	return translate(err, "password reset")
}
