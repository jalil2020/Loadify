package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/loadify/backend/internal/delivery/http/middleware"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/internal/usecase"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/pagination"
	"github.com/loadify/backend/pkg/response"
	"github.com/loadify/backend/pkg/validator"
)

// NotificationHandler exposes the notification centre and device registration.
type NotificationHandler struct {
	notifications *usecase.NotificationUsecase
	chat          *usecase.ChatUsecase
	validator     validator.Validator
}

// NewNotificationHandler builds the notification handler.
func NewNotificationHandler(notifications *usecase.NotificationUsecase, chat *usecase.ChatUsecase, v validator.Validator) *NotificationHandler {
	return &NotificationHandler{notifications: notifications, chat: chat, validator: v}
}

// List returns the caller's notifications.
// GET /api/v1/notifications?unread_only=true
func (h *NotificationHandler) List(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	page := pagination.Parse(c)
	items, total, err := h.notifications.List(c.UserContext(), userID,
		c.QueryBool("unread_only", false), page.Offset(), page.Limit())
	if err != nil {
		return err
	}
	return paginated(c, items, total, page)
}

// MarkRead marks the given notifications read, or all of them when no ids are sent.
// POST /api/v1/notifications/read
func (h *NotificationHandler) MarkRead(c *fiber.Ctx) error {
	var req dto.MarkNotificationsReadRequest
	if err := bindOptional(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	if err := h.notifications.MarkRead(c.UserContext(), userID, req.IDs); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"read": true})
}

// UnreadCount returns the badge counts the app shows on its tab bar.
// GET /api/v1/notifications/unread-count
func (h *NotificationHandler) UnreadCount(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	notifications, err := h.notifications.UnreadCount(c.UserContext(), userID)
	if err != nil {
		return err
	}
	messages, err := h.chat.UnreadCount(c.UserContext(), userID)
	if err != nil {
		return err
	}
	return response.OK(c, dto.UnreadCountResponse{
		Notifications: notifications,
		Messages:      messages,
	})
}

// RegisterDevice stores a push target for the caller.
// POST /api/v1/notifications/devices
func (h *NotificationHandler) RegisterDevice(c *fiber.Ctx) error {
	var req dto.RegisterDeviceRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	if err := h.notifications.RegisterDevice(c.UserContext(), userID, req); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"registered": true})
}

// UnregisterDevice retires a push target, normally on logout.
// DELETE /api/v1/notifications/devices
func (h *NotificationHandler) UnregisterDevice(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	token := c.Query("token")
	if token == "" {
		return apperror.ErrBadRequest.WithMessage("Query parameter 'token' is required")
	}
	if err := h.notifications.UnregisterDevice(c.UserContext(), userID, token); err != nil {
		return err
	}
	return response.NoContent(c)
}

// ListPreferences returns the caller's channel choices.
// GET /api/v1/notifications/preferences
func (h *NotificationHandler) ListPreferences(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	items, err := h.notifications.ListPreferences(c.UserContext(), userID)
	if err != nil {
		return err
	}
	return response.OK(c, items)
}

// SavePreference sets the channels for one notification type.
// PUT /api/v1/notifications/preferences
func (h *NotificationHandler) SavePreference(c *fiber.Ctx) error {
	var req dto.SavePreferenceRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	if err := h.notifications.SavePreference(c.UserContext(), userID, req); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"saved": true})
}
