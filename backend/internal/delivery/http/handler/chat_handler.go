package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/loadify/backend/internal/delivery/http/middleware"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/internal/usecase"
	"github.com/loadify/backend/pkg/pagination"
	"github.com/loadify/backend/pkg/response"
	"github.com/loadify/backend/pkg/validator"
)

// ChatHandler exposes the chat endpoints. The high-frequency path is the
// WebSocket; these endpoints cover history, room listing and clients that cannot
// hold a socket open.
type ChatHandler struct {
	chat      *usecase.ChatUsecase
	validator validator.Validator
}

// NewChatHandler builds the chat handler.
func NewChatHandler(chat *usecase.ChatUsecase, v validator.Validator) *ChatHandler {
	return &ChatHandler{chat: chat, validator: v}
}

// ListRooms returns the caller's conversations.
// GET /api/v1/chats
func (h *ChatHandler) ListRooms(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	page := pagination.Parse(c)
	items, total, err := h.chat.ListRooms(c.UserContext(), userID, page.Offset(), page.Limit())
	if err != nil {
		return err
	}
	return paginated(c, items, total, page)
}

// EnsureBookingRoom opens (or reopens) the conversation for a booking.
// POST /api/v1/chats/booking/:booking_id
func (h *ChatHandler) EnsureBookingRoom(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	bookingID, err := paramUUID(c, "booking_id")
	if err != nil {
		return err
	}
	res, err := h.chat.EnsureBookingRoom(c.UserContext(), userID, bookingID)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Get returns one conversation.
// GET /api/v1/chats/:id
func (h *ChatHandler) Get(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	roomID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.chat.Get(c.UserContext(), userID, roomID)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// ListMessages returns a page of history, newest first.
//
// Paging uses a `before` timestamp cursor rather than an offset, so messages
// arriving mid-scroll cannot shift the page.
// GET /api/v1/chats/:id/messages?before=RFC3339&limit=50
func (h *ChatHandler) ListMessages(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	roomID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}

	var before *time.Time
	if raw := c.Query("before"); raw != "" {
		ts, parseErr := time.Parse(time.RFC3339, raw)
		if parseErr == nil {
			before = &ts
		}
	}

	items, err := h.chat.ListMessages(c.UserContext(), userID, roomID, before, c.QueryInt("limit", 50))
	if err != nil {
		return err
	}
	return response.OK(c, items)
}

// SendMessage posts a message over HTTP.
// POST /api/v1/chats/:id/messages
func (h *ChatHandler) SendMessage(c *fiber.Ctx) error {
	var req dto.SendMessageRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	roomID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.chat.SendMessage(c.UserContext(), userID, roomID, req)
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// MarkRead advances the caller's read pointer.
// POST /api/v1/chats/:id/read
func (h *ChatHandler) MarkRead(c *fiber.Ctx) error {
	var req dto.MarkReadRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	roomID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.chat.MarkRead(c.UserContext(), userID, roomID, req.MessageID); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"read": true})
}

// SetMuted silences or unsilences one conversation.
// PATCH /api/v1/chats/:id/mute
func (h *ChatHandler) SetMuted(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	roomID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	muted := c.QueryBool("muted", true)
	if err := h.chat.SetMuted(c.UserContext(), userID, roomID, muted); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"muted": muted})
}
