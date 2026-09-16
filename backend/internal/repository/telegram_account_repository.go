package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/apperror"
)

type telegramAccountRepository struct{ Base }

// NewTelegramAccountRepository builds the connected-account repository.
func NewTelegramAccountRepository(db *gorm.DB) domain.TelegramAccountRepository {
	return &telegramAccountRepository{Base: NewBase(db)}
}

func (r *telegramAccountRepository) Create(ctx context.Context, account *domain.TelegramAccount) error {
	if err := r.DB(ctx).Create(account).Error; err != nil {
		if isUniqueViolation(err) {
			// The unique index on tg_user_id is what stops the same Telegram
			// account being connected twice, which is the straightforward way
			// to double an account's allowance.
			return apperror.ErrConflict.
				WithMessage("This Telegram account is already connected.").Wrap(err)
		}
		return translate(err, "telegram account")
	}
	return nil
}

func (r *telegramAccountRepository) FindByID(ctx context.Context, companyID, id uuid.UUID) (*domain.TelegramAccount, error) {
	var account domain.TelegramAccount
	err := r.DB(ctx).Where("id = ? AND company_id = ?", id, companyID).First(&account).Error
	if err != nil {
		return nil, translate(err, "telegram account")
	}
	return &account, nil
}

// FindForUser returns the account a person may actually post with: their own.
//
// The ownership check is part of the query rather than something a usecase
// remembers to do, because the consent that connected this account was one
// person's, and Telegram's terms are explicit that acting for someone without
// their knowledge is not allowed.
func (r *telegramAccountRepository) FindForUser(ctx context.Context, companyID, userID, id uuid.UUID) (*domain.TelegramAccount, error) {
	var account domain.TelegramAccount
	err := r.DB(ctx).
		Where("id = ? AND company_id = ? AND user_id = ?", id, companyID, userID).
		First(&account).Error
	if err != nil {
		return nil, translate(err, "telegram account")
	}
	return &account, nil
}

func (r *telegramAccountRepository) ListByCompany(ctx context.Context, companyID uuid.UUID) ([]domain.TelegramAccount, error) {
	var accounts []domain.TelegramAccount
	err := r.DB(ctx).Where("company_id = ?", companyID).
		Order("created_at").Find(&accounts).Error
	if err != nil {
		return nil, translate(err, "telegram account")
	}
	return accounts, nil
}

func (r *telegramAccountRepository) ListByUser(ctx context.Context, companyID, userID uuid.UUID) ([]domain.TelegramAccount, error) {
	var accounts []domain.TelegramAccount
	err := r.DB(ctx).Where("company_id = ? AND user_id = ?", companyID, userID).
		Order("created_at").Find(&accounts).Error
	if err != nil {
		return nil, translate(err, "telegram account")
	}
	return accounts, nil
}

// StoreSession writes the sealed session and turns the account on.
//
// The session and its key version move together because either alone is
// useless: a ciphertext without its version cannot be opened, and a version
// without a ciphertext is a leftover.
func (r *telegramAccountRepository) StoreSession(
	ctx context.Context,
	id uuid.UUID,
	ciphertext []byte,
	keyVersion int16,
	tgUserID int64,
) error {
	res := r.DB(ctx).Model(&domain.TelegramAccount{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"session_ciphertext":  ciphertext,
			"session_key_version": keyVersion,
			"tg_user_id":          tgUserID,
			"status":              domain.TelegramAccountActive,
			"status_reason":       nil,
			"paused_until":        nil,
		})
	if res.Error != nil {
		if isUniqueViolation(res.Error) {
			return apperror.ErrConflict.
				WithMessage("This Telegram account is already connected.").Wrap(res.Error)
		}
		return translate(res.Error, "telegram account")
	}
	if res.RowsAffected == 0 {
		return apperror.ErrNotFound.WithMessage("telegram account not found")
	}
	return nil
}

func (r *telegramAccountRepository) SetStatus(
	ctx context.Context,
	id uuid.UUID,
	status domain.TelegramAccountStatus,
	reason *string,
	pausedUntil *time.Time,
) error {
	res := r.DB(ctx).Model(&domain.TelegramAccount{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":        status,
			"status_reason": reason,
			"paused_until":  pausedUntil,
		})
	if res.Error != nil {
		return translate(res.Error, "telegram account")
	}
	if res.RowsAffected == 0 {
		return apperror.ErrNotFound.WithMessage("telegram account not found")
	}
	return nil
}

func (r *telegramAccountRepository) TouchUsed(ctx context.Context, id uuid.UUID, at time.Time) error {
	err := r.DB(ctx).Model(&domain.TelegramAccount{}).
		Where("id = ?", id).
		UpdateColumn("last_used_at", at).Error
	if err != nil {
		return translate(err, "telegram account")
	}
	return nil
}

// Disconnect wipes the session and retires the row in one statement.
//
// One statement matters: any gap between "this account is disconnected" and
// "its session is gone" is a window in which a worker could still open it. The
// row itself stays, because the posts that went out point at it and a history
// entry whose account vanished explains nothing.
func (r *telegramAccountRepository) Disconnect(ctx context.Context, companyID, id uuid.UUID) error {
	now := time.Now()
	reason := "disconnected by the owner"

	res := r.DB(ctx).Model(&domain.TelegramAccount{}).
		Where("id = ? AND company_id = ?", id, companyID).
		Updates(map[string]any{
			"session_ciphertext":  nil,
			"session_key_version": nil,
			"status":              domain.TelegramAccountDisabled,
			"status_reason":       reason,
			"paused_until":        nil,
			"deleted_at":          now,
		})
	if res.Error != nil {
		return translate(res.Error, "telegram account")
	}
	if res.RowsAffected == 0 {
		return apperror.ErrNotFound.WithMessage("telegram account not found")
	}
	return nil
}
