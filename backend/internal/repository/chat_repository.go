package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/loadify/backend/internal/domain"
)

type chatRepository struct{ Base }

// NewChatRepository builds the chat repository.
func NewChatRepository(db *gorm.DB) domain.ChatRepository {
	return &chatRepository{Base: NewBase(db)}
}

func (r *chatRepository) CreateRoom(ctx context.Context, room *domain.ChatRoom) error {
	return translate(r.DB(ctx).Create(room).Error, "chat room")
}

func (r *chatRepository) FindRoomByID(ctx context.Context, id uuid.UUID) (*domain.ChatRoom, error) {
	var room domain.ChatRoom
	err := r.DB(ctx).
		Preload("Participants", func(db *gorm.DB) *gorm.DB {
			return db.Where("left_at IS NULL").Preload("User")
		}).
		First(&room, "id = ?", id).Error
	if err != nil {
		return nil, translate(err, "chat room")
	}
	return &room, nil
}

func (r *chatRepository) FindRoomByBooking(ctx context.Context, bookingID uuid.UUID) (*domain.ChatRoom, error) {
	var room domain.ChatRoom
	err := r.DB(ctx).First(&room, "booking_id = ?", bookingID).Error
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, translate(err, "chat room")
	}
	return &room, nil
}

// ListRooms returns the caller's conversations, most recently active first.
func (r *chatRepository) ListRooms(ctx context.Context, userID uuid.UUID, offset, limit int) ([]domain.ChatRoom, int64, error) {
	q := r.DB(ctx).Model(&domain.ChatRoom{}).
		Joins(`JOIN chat_participants cp ON cp.room_id = chat_rooms.id
		       AND cp.user_id = ? AND cp.left_at IS NULL`, userID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "chat room")
	}

	var rooms []domain.ChatRoom
	err := applyPage(q.
		Preload("Participants", func(db *gorm.DB) *gorm.DB {
			return db.Where("left_at IS NULL").Preload("User")
		}).
		Preload("Load").
		Preload("Booking").
		Order("chat_rooms.last_message_at DESC NULLS LAST"), offset, limit).
		Find(&rooms).Error
	if err != nil {
		return nil, 0, translate(err, "chat room")
	}
	return rooms, total, nil
}

func (r *chatRepository) SetRoomStatus(ctx context.Context, roomID uuid.UUID, status domain.ChatRoomStatus) error {
	err := r.DB(ctx).Model(&domain.ChatRoom{}).
		Where("id = ?", roomID).Update("status", status).Error
	return translate(err, "chat room")
}

// ArchiveStaleRooms closes conversations whose trip finished long ago. Archived
// rooms stay readable — they are evidence in a dispute — but accept no new
// messages.
func (r *chatRepository) ArchiveStaleRooms(ctx context.Context, before time.Time) (int64, error) {
	res := r.DB(ctx).Exec(`
		UPDATE chat_rooms SET status = 'archived', updated_at = now()
		WHERE status = 'active'
		  AND id IN (
		      SELECT cr.id FROM chat_rooms cr
		      JOIN trips t ON t.id = cr.trip_id
		      WHERE t.status IN ('completed', 'cancelled')
		        AND COALESCE(t.completed_at, t.cancelled_at) < ?
		  )`, before)
	if res.Error != nil {
		return 0, translate(res.Error, "chat room")
	}
	return res.RowsAffected, nil
}

func (r *chatRepository) AddParticipant(ctx context.Context, participant *domain.ChatParticipant) error {
	// Re-adding somebody who left revives their membership rather than failing
	// on the unique index.
	err := r.DB(ctx).Exec(`
		INSERT INTO chat_participants (id, room_id, user_id, role, joined_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, now(), now(), now())
		ON CONFLICT (room_id, user_id)
		DO UPDATE SET left_at = NULL, updated_at = now()`,
		participant.ID, participant.RoomID, participant.UserID, participant.Role).Error
	return translate(err, "chat participant")
}

func (r *chatRepository) FindParticipant(ctx context.Context, roomID, userID uuid.UUID) (*domain.ChatParticipant, error) {
	var participant domain.ChatParticipant
	err := r.DB(ctx).First(&participant,
		"room_id = ? AND user_id = ? AND left_at IS NULL", roomID, userID).Error
	if err != nil {
		return nil, translate(err, "chat participant")
	}
	return &participant, nil
}

func (r *chatRepository) ListParticipants(ctx context.Context, roomID uuid.UUID) ([]domain.ChatParticipant, error) {
	var participants []domain.ChatParticipant
	err := r.DB(ctx).Preload("User").
		Where("room_id = ? AND left_at IS NULL", roomID).
		Find(&participants).Error
	if err != nil {
		return nil, translate(err, "chat participant")
	}
	return participants, nil
}

func (r *chatRepository) SetMuted(ctx context.Context, roomID, userID uuid.UUID, muted bool) error {
	err := r.DB(ctx).Model(&domain.ChatParticipant{}).
		Where("room_id = ? AND user_id = ?", roomID, userID).
		Update("is_muted", muted).Error
	return translate(err, "chat participant")
}

// CreateMessage stores a message and stamps the room's activity time so the
// conversation list ordering stays correct.
func (r *chatRepository) CreateMessage(ctx context.Context, message *domain.ChatMessage) error {
	db := r.DB(ctx)
	if err := db.Create(message).Error; err != nil {
		return translate(err, "chat message")
	}
	err := db.Model(&domain.ChatRoom{}).
		Where("id = ?", message.RoomID).
		Update("last_message_at", message.CreatedAt).Error
	return translate(err, "chat room")
}

func (r *chatRepository) FindMessageByID(ctx context.Context, id uuid.UUID) (*domain.ChatMessage, error) {
	var message domain.ChatMessage
	err := r.DB(ctx).Preload("Sender").Preload("File").First(&message, "id = ?", id).Error
	if err != nil {
		return nil, translate(err, "chat message")
	}
	return &message, nil
}

// FindMessageByClientID resolves an idempotent retry from a mobile client.
func (r *chatRepository) FindMessageByClientID(ctx context.Context, roomID, senderUserID uuid.UUID, clientMsgID string) (*domain.ChatMessage, error) {
	if clientMsgID == "" {
		return nil, nil
	}
	var message domain.ChatMessage
	err := r.DB(ctx).First(&message,
		"room_id = ? AND sender_user_id = ? AND client_msg_id = ?",
		roomID, senderUserID, clientMsgID).Error
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, translate(err, "chat message")
	}
	return &message, nil
}

// ListMessages pages backwards through history using a created_at cursor rather
// than an offset, so new arrivals cannot shift a page under the reader.
func (r *chatRepository) ListMessages(ctx context.Context, roomID uuid.UUID, before *time.Time, limit int) ([]domain.ChatMessage, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	q := r.DB(ctx).Preload("Sender").Preload("File").
		Where("room_id = ?", roomID)
	if before != nil {
		q = q.Where("created_at < ?", *before)
	}

	var messages []domain.ChatMessage
	if err := q.Order("created_at DESC").Limit(limit).Find(&messages).Error; err != nil {
		return nil, translate(err, "chat message")
	}
	return messages, nil
}

func (r *chatRepository) SoftDeleteMessage(ctx context.Context, id uuid.UUID) error {
	err := r.DB(ctx).Where("id = ?", id).Delete(&domain.ChatMessage{}).Error
	return translate(err, "chat message")
}

// MarkRead advances the reader's pointer. The unread count is recomputed from
// the messages table rather than decremented, so it self-heals after any
// missed increment.
func (r *chatRepository) MarkRead(ctx context.Context, roomID, userID, upToMessageID uuid.UUID) error {
	err := r.DB(ctx).Exec(`
		UPDATE chat_participants cp SET
			last_read_message_id = ?,
			unread_count = (
				SELECT COUNT(*) FROM chat_messages m
				WHERE m.room_id = cp.room_id
				  AND m.deleted_at IS NULL
				  AND m.sender_user_id IS DISTINCT FROM cp.user_id
				  AND m.created_at > (SELECT created_at FROM chat_messages WHERE id = ?)
			),
			updated_at = now()
		WHERE cp.room_id = ? AND cp.user_id = ?`,
		upToMessageID, upToMessageID, roomID, userID).Error
	return translate(err, "chat participant")
}

// IncrementUnread bumps every participant except the sender.
func (r *chatRepository) IncrementUnread(ctx context.Context, roomID uuid.UUID, exceptUserID uuid.UUID) error {
	err := r.DB(ctx).Model(&domain.ChatParticipant{}).
		Where("room_id = ? AND user_id <> ? AND left_at IS NULL", roomID, exceptUserID).
		UpdateColumn("unread_count", gorm.Expr("unread_count + 1")).Error
	return translate(err, "chat participant")
}

func (r *chatRepository) TotalUnread(ctx context.Context, userID uuid.UUID) (int64, error) {
	var total *int64
	err := r.DB(ctx).Model(&domain.ChatParticipant{}).
		Select("SUM(unread_count)").
		Where("user_id = ? AND left_at IS NULL", userID).
		Scan(&total).Error
	if err != nil {
		return 0, translate(err, "chat participant")
	}
	if total == nil {
		return 0, nil
	}
	return *total, nil
}
