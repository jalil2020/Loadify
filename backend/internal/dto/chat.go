package dto

import (
	"time"

	"github.com/google/uuid"
)

// SendMessageRequest posts a message to a room.
type SendMessageRequest struct {
	Type      string        `json:"type" validate:"omitempty,oneof=text image file location"`
	Body      string        `json:"body" validate:"omitempty,max=4000"`
	FileID    *uuid.UUID    `json:"file_id" validate:"omitempty"`
	Location  *PointRequest `json:"location" validate:"omitempty"`
	ReplyToID *uuid.UUID    `json:"reply_to_id" validate:"omitempty"`
	// ClientMsgID makes a retry idempotent: a phone that loses its connection
	// mid-send can safely repeat the request without double-posting.
	ClientMsgID string `json:"client_msg_id" validate:"omitempty,max=64"`
}

// MarkReadRequest advances the caller's read pointer.
type MarkReadRequest struct {
	MessageID uuid.UUID `json:"message_id" validate:"required"`
}

// MessageResponse is one chat message.
type MessageResponse struct {
	ID           uuid.UUID      `json:"id"`
	RoomID       uuid.UUID      `json:"room_id"`
	SenderID     *uuid.UUID     `json:"sender_id"`
	SenderName   string         `json:"sender_name,omitempty"`
	SenderAvatar *string        `json:"sender_avatar,omitempty"`
	Type         string         `json:"type"`
	Body         *string        `json:"body"`
	FileID       *uuid.UUID     `json:"file_id,omitempty"`
	FileURL      string         `json:"file_url,omitempty"`
	Location     *PointResponse `json:"location,omitempty"`
	ReplyToID    *uuid.UUID     `json:"reply_to_id,omitempty"`
	ClientMsgID  *string        `json:"client_msg_id,omitempty"`
	IsMine       bool           `json:"is_mine"`
	EditedAt     *time.Time     `json:"edited_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

// ChatRoomResponse is one conversation in the caller's list.
type ChatRoomResponse struct {
	ID            uuid.UUID                 `json:"id"`
	Type          string                    `json:"type"`
	Status        string                    `json:"status"`
	BookingID     *uuid.UUID                `json:"booking_id"`
	LoadID        *uuid.UUID                `json:"load_id"`
	TripID        *uuid.UUID                `json:"trip_id"`
	LoadReference string                    `json:"load_reference,omitempty"`
	LoadTitle     string                    `json:"load_title,omitempty"`
	Participants  []ChatParticipantResponse `json:"participants"`
	LastMessage   *MessageResponse          `json:"last_message,omitempty"`
	LastMessageAt *time.Time                `json:"last_message_at"`
	UnreadCount   int                       `json:"unread_count"`
	IsMuted       bool                      `json:"is_muted"`
	CreatedAt     time.Time                 `json:"created_at"`
}

// ChatParticipantResponse is one member of a room.
type ChatParticipantResponse struct {
	UserID    uuid.UUID `json:"user_id"`
	FullName  string    `json:"full_name"`
	AvatarURL *string   `json:"avatar_url"`
	Role      string    `json:"role"`
	IsOnline  bool      `json:"is_online"`
}

// ─── notifications ──────────────────────────────────────────────────

// RegisterDeviceRequest registers a push target.
type RegisterDeviceRequest struct {
	Token       string `json:"token" validate:"required,min=10,max=512"`
	Platform    string `json:"platform" validate:"required,oneof=android ios web"`
	DeviceID    string `json:"device_id" validate:"omitempty,max=128"`
	DeviceModel string `json:"device_model" validate:"omitempty,max=128"`
	AppVersion  string `json:"app_version" validate:"omitempty,max=32"`
	Locale      string `json:"locale" validate:"omitempty,locale"`
}

// MarkNotificationsReadRequest marks specific notifications read. An empty list
// means "all of them".
type MarkNotificationsReadRequest struct {
	IDs []uuid.UUID `json:"ids" validate:"omitempty,max=200"`
}

// NotificationResponse is one entry in the notification centre.
type NotificationResponse struct {
	ID        uuid.UUID      `json:"id"`
	Type      string         `json:"type"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	Data      map[string]any `json:"data,omitempty"`
	Priority  string         `json:"priority"`
	ImageURL  *string        `json:"image_url"`
	ActionURL *string        `json:"action_url"`
	ReadAt    *time.Time     `json:"read_at"`
	CreatedAt time.Time      `json:"created_at"`
}

// UnreadCountResponse is the badge payload.
type UnreadCountResponse struct {
	Notifications int64 `json:"notifications"`
	Messages      int64 `json:"messages"`
}

// SavePreferenceRequest sets the channels for one notification type.
type SavePreferenceRequest struct {
	Type  string `json:"type" validate:"required,max=48"`
	Push  bool   `json:"push"`
	InApp bool   `json:"in_app"`
	SMS   bool   `json:"sms"`
	Email bool   `json:"email"`
}

// PreferenceResponse is one notification-type preference.
type PreferenceResponse struct {
	Type  string `json:"type"`
	Push  bool   `json:"push"`
	InApp bool   `json:"in_app"`
	SMS   bool   `json:"sms"`
	Email bool   `json:"email"`
}
