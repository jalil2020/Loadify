package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/loadify/backend/pkg/geo"
)

// ChatRoomType is what a conversation is attached to.
type ChatRoomType string

const (
	ChatRoomBooking ChatRoomType = "booking"
	ChatRoomSupport ChatRoomType = "support"
	ChatRoomDirect  ChatRoomType = "direct"
)

// ChatRoomStatus controls whether a room still accepts messages.
type ChatRoomStatus string

const (
	ChatRoomActive   ChatRoomStatus = "active"
	ChatRoomArchived ChatRoomStatus = "archived"
	ChatRoomClosed   ChatRoomStatus = "closed"
)

// MessageType is the payload kind of a chat message.
type MessageType string

const (
	MessageText     MessageType = "text"
	MessageImage    MessageType = "image"
	MessageFile     MessageType = "file"
	MessageLocation MessageType = "location"
	MessageSystem   MessageType = "system"
)

// ChatRoom is a conversation. Rooms are always scoped to something concrete —
// a booking or a support ticket — never a free-form contact list, so people
// cannot cold-message each other through the platform.
type ChatRoom struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	BookingID     *uuid.UUID     `gorm:"type:uuid"`
	LoadID        *uuid.UUID     `gorm:"type:uuid"`
	TripID        *uuid.UUID     `gorm:"type:uuid"`
	Type          ChatRoomType   `gorm:"size:16;not null;default:'booking'"`
	Status        ChatRoomStatus `gorm:"size:16;not null;default:'active'"`
	LastMessageAt *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time

	Booking      *Booking          `gorm:"foreignKey:BookingID"`
	Load         *Load             `gorm:"foreignKey:LoadID"`
	Participants []ChatParticipant `gorm:"foreignKey:RoomID"`
}

// CanPost reports whether new messages are accepted.
func (r *ChatRoom) CanPost() bool { return r.Status == ChatRoomActive }

// ChatParticipant is one member of a room, with their read position.
type ChatParticipant struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	RoomID            uuid.UUID  `gorm:"type:uuid;not null;index"`
	UserID            uuid.UUID  `gorm:"type:uuid;not null;index"`
	Role              string     `gorm:"size:16;not null;default:'member'"`
	LastReadMessageID *uuid.UUID `gorm:"type:uuid"`
	UnreadCount       int        `gorm:"not null;default:0"`
	IsMuted           bool       `gorm:"not null;default:false"`
	JoinedAt          time.Time
	LeftAt            *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time

	User *User `gorm:"foreignKey:UserID"`
}

// ChatMessage is one message in a room.
type ChatMessage struct {
	ID           uuid.UUID   `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	RoomID       uuid.UUID   `gorm:"type:uuid;not null;index"`
	SenderUserID *uuid.UUID  `gorm:"type:uuid"`
	Type         MessageType `gorm:"size:16;not null;default:'text'"`
	Body         *string
	FileID       *uuid.UUID `gorm:"type:uuid"`
	Location     *geo.Point `gorm:"type:geography(Point,4326)"`
	ReplyToID    *uuid.UUID `gorm:"type:uuid"`
	// ClientMsgID lets a phone retry a send without duplicating the message.
	ClientMsgID *string `gorm:"size:64"`
	DeliveredAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	EditedAt    *time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`

	Sender *User `gorm:"foreignKey:SenderUserID"`
	File   *File `gorm:"foreignKey:FileID"`
}

// NotificationChannel is how a notification reaches the user.
type NotificationChannel string

const (
	ChannelPush  NotificationChannel = "push"
	ChannelInApp NotificationChannel = "in_app"
	ChannelSMS   NotificationChannel = "sms"
	ChannelEmail NotificationChannel = "email"
)

// StoredNotification is a notification persisted for the in-app centre.
type StoredNotification struct {
	ID           uuid.UUID           `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID       uuid.UUID           `gorm:"type:uuid;not null;index"`
	Type         string              `gorm:"size:48;not null"`
	Title        string              `gorm:"size:255;not null"`
	Body         string              `gorm:"not null"`
	Data         JSONMap             `gorm:"type:jsonb;not null;default:'{}'"`
	Channel      NotificationChannel `gorm:"size:16;not null;default:'in_app'"`
	Priority     string              `gorm:"size:8;not null;default:'normal'"`
	ImageURL     *string             `gorm:"size:512"`
	ActionURL    *string             `gorm:"size:512"`
	ReadAt       *time.Time
	SentAt       *time.Time
	FailedReason *string
	CreatedAt    time.Time
}

// TableName pins the notifications table.
func (StoredNotification) TableName() string { return "notifications" }

// DeviceToken is one push target belonging to a user.
type DeviceToken struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID      uuid.UUID `gorm:"type:uuid;not null;index"`
	Token       string    `gorm:"size:512;not null"`
	Platform    Platform  `gorm:"size:16;not null"`
	DeviceID    *string   `gorm:"size:128"`
	DeviceModel *string   `gorm:"size:128"`
	AppVersion  *string   `gorm:"size:32"`
	Locale      *string   `gorm:"size:5"`
	IsActive    bool      `gorm:"not null;default:true"`
	LastUsedAt  time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NotificationPreference is a per-type channel opt-in.
type NotificationPreference struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index"`
	Type      string    `gorm:"size:48;not null"`
	Push      bool      `gorm:"not null;default:true"`
	InApp     bool      `gorm:"not null;default:true"`
	SMS       bool      `gorm:"column:sms;not null;default:false"`
	Email     bool      `gorm:"not null;default:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ─── ports ──────────────────────────────────────────────────────────

// ChatRepository persists rooms, membership and messages.
type ChatRepository interface {
	CreateRoom(ctx context.Context, room *ChatRoom) error
	FindRoomByID(ctx context.Context, id uuid.UUID) (*ChatRoom, error)
	FindRoomByBooking(ctx context.Context, bookingID uuid.UUID) (*ChatRoom, error)
	ListRooms(ctx context.Context, userID uuid.UUID, offset, limit int) ([]ChatRoom, int64, error)
	SetRoomStatus(ctx context.Context, roomID uuid.UUID, status ChatRoomStatus) error
	ArchiveStaleRooms(ctx context.Context, before time.Time) (int64, error)

	AddParticipant(ctx context.Context, participant *ChatParticipant) error
	FindParticipant(ctx context.Context, roomID, userID uuid.UUID) (*ChatParticipant, error)
	ListParticipants(ctx context.Context, roomID uuid.UUID) ([]ChatParticipant, error)
	SetMuted(ctx context.Context, roomID, userID uuid.UUID, muted bool) error

	CreateMessage(ctx context.Context, message *ChatMessage) error
	FindMessageByID(ctx context.Context, id uuid.UUID) (*ChatMessage, error)
	FindMessageByClientID(ctx context.Context, roomID, senderUserID uuid.UUID, clientMsgID string) (*ChatMessage, error)
	// ListMessages pages backwards from `before` — chat is read newest-first.
	ListMessages(ctx context.Context, roomID uuid.UUID, before *time.Time, limit int) ([]ChatMessage, error)
	SoftDeleteMessage(ctx context.Context, id uuid.UUID) error

	// MarkRead advances one participant's read pointer and clears their unread
	// count, while incrementing everyone else's on a new message.
	MarkRead(ctx context.Context, roomID, userID, upToMessageID uuid.UUID) error
	IncrementUnread(ctx context.Context, roomID uuid.UUID, exceptUserID uuid.UUID) error
	TotalUnread(ctx context.Context, userID uuid.UUID) (int64, error)
}

// NotificationRepository persists the notification centre and push targets.
type NotificationRepository interface {
	Create(ctx context.Context, notification *StoredNotification) error
	CreateMany(ctx context.Context, notifications []StoredNotification) error
	List(ctx context.Context, userID uuid.UUID, unreadOnly bool, offset, limit int) ([]StoredNotification, int64, error)
	MarkRead(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) error
	MarkAllRead(ctx context.Context, userID uuid.UUID) error
	CountUnread(ctx context.Context, userID uuid.UUID) (int64, error)
	MarkSent(ctx context.Context, id uuid.UUID, failedReason *string) error
	DeleteOlderThan(ctx context.Context, before time.Time) (int64, error)

	UpsertDeviceToken(ctx context.Context, token *DeviceToken) error
	ListDeviceTokens(ctx context.Context, userID uuid.UUID) ([]DeviceToken, error)
	ListDeviceTokensForUsers(ctx context.Context, userIDs []uuid.UUID) ([]DeviceToken, error)
	DeactivateDeviceToken(ctx context.Context, token string) error

	GetPreference(ctx context.Context, userID uuid.UUID, notificationType string) (*NotificationPreference, error)
	SavePreference(ctx context.Context, pref *NotificationPreference) error
	ListPreferences(ctx context.Context, userID uuid.UUID) ([]NotificationPreference, error)
}

// PushMessage is one payload handed to the push provider.
type PushMessage struct {
	Tokens   []string
	Title    string
	Body     string
	Data     map[string]string
	ImageURL string
	Priority string
}

// PushResult reports what the provider did, including the tokens it rejected as
// permanently invalid so they can be deactivated.
type PushResult struct {
	SuccessCount  int
	FailureCount  int
	InvalidTokens []string
}

// PushSender delivers push notifications.
type PushSender interface {
	Send(ctx context.Context, msg PushMessage) (*PushResult, error)
}
