package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/loadify/backend/internal/domain"
)

type notificationRepository struct{ Base }

// NewNotificationRepository builds the notification repository.
func NewNotificationRepository(db *gorm.DB) domain.NotificationRepository {
	return &notificationRepository{Base: NewBase(db)}
}

func (r *notificationRepository) Create(ctx context.Context, n *domain.StoredNotification) error {
	if n.Data == nil {
		n.Data = domain.JSONMap{}
	}
	return translate(r.DB(ctx).Create(n).Error, "notification")
}

func (r *notificationRepository) CreateMany(ctx context.Context, notifications []domain.StoredNotification) error {
	if len(notifications) == 0 {
		return nil
	}
	for i := range notifications {
		if notifications[i].Data == nil {
			notifications[i].Data = domain.JSONMap{}
		}
	}
	// Batched so a broadcast to a large company is one round trip per 200 rows.
	err := r.DB(ctx).CreateInBatches(notifications, 200).Error
	return translate(err, "notification")
}

func (r *notificationRepository) List(ctx context.Context, userID uuid.UUID, unreadOnly bool, offset, limit int) ([]domain.StoredNotification, int64, error) {
	q := r.DB(ctx).Model(&domain.StoredNotification{}).Where("user_id = ?", userID)
	if unreadOnly {
		q = q.Where("read_at IS NULL")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "notification")
	}

	var notifications []domain.StoredNotification
	err := applyPage(q.Order("created_at DESC"), offset, limit).Find(&notifications).Error
	if err != nil {
		return nil, 0, translate(err, "notification")
	}
	return notifications, total, nil
}

// MarkRead marks specific notifications read, scoped to the owner so an id from
// somebody else's inbox has no effect.
func (r *notificationRepository) MarkRead(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	err := r.DB(ctx).Model(&domain.StoredNotification{}).
		Where("user_id = ? AND id IN ? AND read_at IS NULL", userID, ids).
		Update("read_at", time.Now().UTC()).Error
	return translate(err, "notification")
}

func (r *notificationRepository) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	err := r.DB(ctx).Model(&domain.StoredNotification{}).
		Where("user_id = ? AND read_at IS NULL", userID).
		Update("read_at", time.Now().UTC()).Error
	return translate(err, "notification")
}

func (r *notificationRepository) CountUnread(ctx context.Context, userID uuid.UUID) (int64, error) {
	var count int64
	err := r.DB(ctx).Model(&domain.StoredNotification{}).
		Where("user_id = ? AND read_at IS NULL", userID).
		Count(&count).Error
	if err != nil {
		return 0, translate(err, "notification")
	}
	return count, nil
}

// MarkSent records the delivery outcome so a failed push is visible in support.
func (r *notificationRepository) MarkSent(ctx context.Context, id uuid.UUID, failedReason *string) error {
	columns := map[string]any{"failed_reason": failedReason}
	if failedReason == nil {
		columns["sent_at"] = time.Now().UTC()
	}
	err := r.DB(ctx).Model(&domain.StoredNotification{}).
		Where("id = ?", id).Updates(columns).Error
	return translate(err, "notification")
}

func (r *notificationRepository) DeleteOlderThan(ctx context.Context, before time.Time) (int64, error) {
	res := r.DB(ctx).Where("created_at < ?", before).Delete(&domain.StoredNotification{})
	if res.Error != nil {
		return 0, translate(res.Error, "notification")
	}
	return res.RowsAffected, nil
}

// ─── device tokens ──────────────────────────────────────────────────

// UpsertDeviceToken registers a push target.
//
// FCM reissues the same token to whichever app instance holds it, so a token
// can legitimately move between users — the upsert reassigns ownership rather
// than colliding.
func (r *notificationRepository) UpsertDeviceToken(ctx context.Context, token *domain.DeviceToken) error {
	err := r.DB(ctx).Exec(`
		INSERT INTO device_tokens
			(id, user_id, token, platform, device_id, device_model, app_version,
			 locale, is_active, last_used_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, TRUE, now(), now(), now())
		ON CONFLICT (token) DO UPDATE SET
			user_id      = EXCLUDED.user_id,
			platform     = EXCLUDED.platform,
			device_id    = EXCLUDED.device_id,
			device_model = EXCLUDED.device_model,
			app_version  = EXCLUDED.app_version,
			locale       = EXCLUDED.locale,
			is_active    = TRUE,
			last_used_at = now(),
			updated_at   = now()`,
		token.ID, token.UserID, token.Token, token.Platform, token.DeviceID,
		token.DeviceModel, token.AppVersion, token.Locale).Error
	return translate(err, "device token")
}

func (r *notificationRepository) ListDeviceTokens(ctx context.Context, userID uuid.UUID) ([]domain.DeviceToken, error) {
	var tokens []domain.DeviceToken
	err := r.DB(ctx).Where("user_id = ? AND is_active", userID).Find(&tokens).Error
	if err != nil {
		return nil, translate(err, "device token")
	}
	return tokens, nil
}

func (r *notificationRepository) ListDeviceTokensForUsers(ctx context.Context, userIDs []uuid.UUID) ([]domain.DeviceToken, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	var tokens []domain.DeviceToken
	err := r.DB(ctx).Where("user_id IN ? AND is_active", userIDs).Find(&tokens).Error
	if err != nil {
		return nil, translate(err, "device token")
	}
	return tokens, nil
}

// DeactivateDeviceToken retires a token the provider rejected as permanently
// invalid, so it is not retried on every future send.
func (r *notificationRepository) DeactivateDeviceToken(ctx context.Context, token string) error {
	err := r.DB(ctx).Model(&domain.DeviceToken{}).
		Where("token = ?", token).Update("is_active", false).Error
	return translate(err, "device token")
}

// ─── preferences ────────────────────────────────────────────────────

// GetPreference returns the user's channel choices for a type, defaulting to
// push and in-app when they have never configured it.
func (r *notificationRepository) GetPreference(ctx context.Context, userID uuid.UUID, notificationType string) (*domain.NotificationPreference, error) {
	var pref domain.NotificationPreference
	err := r.DB(ctx).First(&pref, "user_id = ? AND type = ?", userID, notificationType).Error
	if err != nil {
		if isNotFound(err) {
			return &domain.NotificationPreference{
				UserID: userID, Type: notificationType,
				Push: true, InApp: true, SMS: false, Email: false,
			}, nil
		}
		return nil, translate(err, "notification preference")
	}
	return &pref, nil
}

func (r *notificationRepository) SavePreference(ctx context.Context, pref *domain.NotificationPreference) error {
	err := r.DB(ctx).Exec(`
		INSERT INTO notification_preferences
			(id, user_id, type, push, in_app, sms, email, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, now(), now())
		ON CONFLICT (user_id, type) DO UPDATE SET
			push = EXCLUDED.push, in_app = EXCLUDED.in_app,
			sms = EXCLUDED.sms, email = EXCLUDED.email, updated_at = now()`,
		uuid.New(), pref.UserID, pref.Type, pref.Push, pref.InApp, pref.SMS, pref.Email).Error
	return translate(err, "notification preference")
}

func (r *notificationRepository) ListPreferences(ctx context.Context, userID uuid.UUID) ([]domain.NotificationPreference, error) {
	var prefs []domain.NotificationPreference
	err := r.DB(ctx).Where("user_id = ?", userID).Order("type").Find(&prefs).Error
	if err != nil {
		return nil, translate(err, "notification preference")
	}
	return prefs, nil
}
