package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/logger"
)

// notificationRetention is how long the in-app centre keeps history.
const notificationRetention = 90 * 24 * time.Hour

// NotificationUsecase persists notifications and delivers them over the
// available channels. It implements domain.Notifier.
type NotificationUsecase struct {
	notifications domain.NotificationRepository
	users         domain.UserRepository
	push          domain.PushSender
	events        domain.EventPublisher
	log           logger.Logger
}

// NewNotificationUsecase wires the notification usecase.
func NewNotificationUsecase(
	notifications domain.NotificationRepository,
	users domain.UserRepository,
	push domain.PushSender,
	events domain.EventPublisher,
	log logger.Logger,
) *NotificationUsecase {
	return &NotificationUsecase{
		notifications: notifications, users: users,
		push: push, events: events, log: log,
	}
}

// Notify stores one notification and delivers it.
func (u *NotificationUsecase) Notify(ctx context.Context, n domain.Notification) error {
	return u.NotifyMany(ctx, []domain.Notification{n})
}

// NotifyMany stores and delivers a batch.
//
// The in-app row is written first and is the source of truth: if the push
// provider is down the user still sees the notification when they open the app.
func (u *NotificationUsecase) NotifyMany(ctx context.Context, notifications []domain.Notification) error {
	if len(notifications) == 0 {
		return nil
	}

	stored := make([]domain.StoredNotification, 0, len(notifications))
	allowPush := make(map[uuid.UUID]bool, len(notifications))

	for _, n := range notifications {
		pref, err := u.notifications.GetPreference(ctx, n.UserID, n.Type)
		if err != nil {
			u.log.Warn("reading notification preference failed",
				logger.String("user_id", n.UserID.String()), logger.Err(err))
			// A preference lookup failure must not silence an important message.
			pref = &domain.NotificationPreference{Push: true, InApp: true}
		}
		if !pref.InApp && !pref.Push {
			continue
		}
		allowPush[n.UserID] = pref.Push

		row := domain.StoredNotification{
			ID:       uuid.New(),
			UserID:   n.UserID,
			Type:     n.Type,
			Title:    n.Title,
			Body:     n.Body,
			Data:     domain.JSONMap(n.Data),
			Channel:  domain.ChannelInApp,
			Priority: n.Priority,
		}
		if row.Priority == "" {
			row.Priority = "normal"
		}
		if n.ActionURL != "" {
			row.ActionURL = &n.ActionURL
		}
		stored = append(stored, row)
	}

	if len(stored) == 0 {
		return nil
	}
	if err := u.notifications.CreateMany(ctx, stored); err != nil {
		return err
	}

	u.deliverRealtime(ctx, stored)
	u.deliverPush(ctx, stored, allowPush)
	return nil
}

// deliverRealtime pushes the notification down any open socket so the badge
// updates without a poll.
func (u *NotificationUsecase) deliverRealtime(ctx context.Context, stored []domain.StoredNotification) {
	if u.events == nil {
		return
	}
	for i := range stored {
		n := stored[i]
		payload := map[string]any{
			"notification_id": n.ID,
			"type":            n.Type,
			"title":           n.Title,
			"body":            n.Body,
			"priority":        n.Priority,
			"created_at":      n.CreatedAt,
		}
		if len(n.Data) > 0 {
			payload["data"] = map[string]any(n.Data)
		}
		if err := u.events.PublishToUser(ctx, n.UserID, domain.EventNotification, payload); err != nil {
			u.log.Warn("publishing notification event failed", logger.Err(err))
		}
	}
}

// deliverPush sends to FCM, grouping by user so one person with three devices
// is one provider call.
func (u *NotificationUsecase) deliverPush(ctx context.Context, stored []domain.StoredNotification, allowPush map[uuid.UUID]bool) {
	if u.push == nil {
		return
	}

	userIDs := make([]uuid.UUID, 0, len(stored))
	for i := range stored {
		if allowPush[stored[i].UserID] {
			userIDs = append(userIDs, stored[i].UserID)
		}
	}
	if len(userIDs) == 0 {
		return
	}

	tokens, err := u.notifications.ListDeviceTokensForUsers(ctx, userIDs)
	if err != nil {
		u.log.Warn("loading device tokens failed", logger.Err(err))
		return
	}
	byUser := make(map[uuid.UUID][]string, len(tokens))
	for _, t := range tokens {
		byUser[t.UserID] = append(byUser[t.UserID], t.Token)
	}

	for i := range stored {
		n := stored[i]
		if !allowPush[n.UserID] {
			continue
		}
		deviceTokens := byUser[n.UserID]
		if len(deviceTokens) == 0 {
			continue
		}

		data := map[string]string{"type": n.Type, "notification_id": n.ID.String()}
		for k, v := range n.Data {
			if s, ok := v.(string); ok {
				data[k] = s
			}
		}

		result, err := u.push.Send(ctx, domain.PushMessage{
			Tokens:   deviceTokens,
			Title:    n.Title,
			Body:     n.Body,
			Data:     data,
			Priority: n.Priority,
		})
		if err != nil {
			reason := err.Error()
			if markErr := u.notifications.MarkSent(ctx, n.ID, &reason); markErr != nil {
				u.log.Warn("recording push failure failed", logger.Err(markErr))
			}
			continue
		}
		if markErr := u.notifications.MarkSent(ctx, n.ID, nil); markErr != nil {
			u.log.Warn("recording push success failed", logger.Err(markErr))
		}
		// Retire tokens the provider says are dead, or every future send repeats
		// the same wasted call.
		for _, dead := range result.InvalidTokens {
			if err := u.notifications.DeactivateDeviceToken(ctx, dead); err != nil {
				u.log.Warn("deactivating dead device token failed", logger.Err(err))
			}
		}
	}
}

// List returns the caller's notification centre.
func (u *NotificationUsecase) List(ctx context.Context, userID uuid.UUID, unreadOnly bool, offset, limit int) ([]dto.NotificationResponse, int64, error) {
	items, total, err := u.notifications.List(ctx, userID, unreadOnly, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.NotificationResponse, 0, len(items))
	for i := range items {
		out = append(out, notificationResponse(&items[i]))
	}
	return out, total, nil
}

// MarkRead marks the given notifications read, or all of them when no ids are given.
func (u *NotificationUsecase) MarkRead(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return u.notifications.MarkAllRead(ctx, userID)
	}
	return u.notifications.MarkRead(ctx, userID, ids)
}

// UnreadCount returns the notification badge count.
func (u *NotificationUsecase) UnreadCount(ctx context.Context, userID uuid.UUID) (int64, error) {
	return u.notifications.CountUnread(ctx, userID)
}

// RegisterDevice stores a push target for the caller.
func (u *NotificationUsecase) RegisterDevice(ctx context.Context, userID uuid.UUID, req dto.RegisterDeviceRequest) error {
	token := &domain.DeviceToken{
		ID:       uuid.New(),
		UserID:   userID,
		Token:    req.Token,
		Platform: domain.ParsePlatform(req.Platform),
	}
	if req.DeviceID != "" {
		token.DeviceID = &req.DeviceID
	}
	if req.DeviceModel != "" {
		token.DeviceModel = &req.DeviceModel
	}
	if req.AppVersion != "" {
		token.AppVersion = &req.AppVersion
	}
	if req.Locale != "" {
		token.Locale = &req.Locale
	}
	return u.notifications.UpsertDeviceToken(ctx, token)
}

// UnregisterDevice retires a push target on logout.
func (u *NotificationUsecase) UnregisterDevice(ctx context.Context, userID uuid.UUID, token string) error {
	// Ownership matters: a client must not be able to silence somebody else's
	// device by guessing a token.
	tokens, err := u.notifications.ListDeviceTokens(ctx, userID)
	if err != nil {
		return err
	}
	for _, t := range tokens {
		if t.Token == token {
			return u.notifications.DeactivateDeviceToken(ctx, token)
		}
	}
	return apperror.ErrNotFound.WithMessage("This device is not registered to your account")
}

// ListPreferences returns the caller's channel choices.
func (u *NotificationUsecase) ListPreferences(ctx context.Context, userID uuid.UUID) ([]dto.PreferenceResponse, error) {
	prefs, err := u.notifications.ListPreferences(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.PreferenceResponse, 0, len(prefs))
	for _, p := range prefs {
		out = append(out, dto.PreferenceResponse{
			Type: p.Type, Push: p.Push, InApp: p.InApp, SMS: p.SMS, Email: p.Email,
		})
	}
	return out, nil
}

// SavePreference stores the channels for one notification type.
func (u *NotificationUsecase) SavePreference(ctx context.Context, userID uuid.UUID, req dto.SavePreferenceRequest) error {
	return u.notifications.SavePreference(ctx, &domain.NotificationPreference{
		UserID: userID,
		Type:   req.Type,
		Push:   req.Push,
		InApp:  req.InApp,
		SMS:    req.SMS,
		Email:  req.Email,
	})
}

// PurgeOld is the scheduled job that trims notification history.
func (u *NotificationUsecase) PurgeOld(ctx context.Context) (int64, error) {
	return u.notifications.DeleteOlderThan(ctx, time.Now().UTC().Add(-notificationRetention))
}

func notificationResponse(n *domain.StoredNotification) dto.NotificationResponse {
	return dto.NotificationResponse{
		ID:        n.ID,
		Type:      n.Type,
		Title:     n.Title,
		Body:      n.Body,
		Data:      map[string]any(n.Data),
		Priority:  n.Priority,
		ImageURL:  n.ImageURL,
		ActionURL: n.ActionURL,
		ReadAt:    n.ReadAt,
		CreatedAt: n.CreatedAt,
	}
}
