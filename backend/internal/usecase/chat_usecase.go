package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/geo"
	"github.com/loadify/backend/pkg/logger"
)

// archiveChatAfter is how long a room stays writable after its trip closes.
// Rooms are never deleted — they are evidence if a dispute is reopened.
const archiveChatAfter = 30 * 24 * time.Hour

// ChatUsecase owns conversations between a driver and a shipper.
type ChatUsecase struct {
	chat      domain.ChatRepository
	bookings  domain.BookingRepository
	drivers   domain.DriverRepository
	companies domain.CompanyRepository
	users     domain.UserRepository
	files     *FileUsecase
	tx        domain.TxManager
	events    domain.EventPublisher
	notifier  domain.Notifier
	presence  PresenceChecker
	log       logger.Logger
}

// PresenceChecker reports whether a user currently holds a live socket. The chat
// usecase uses it to decide between a silent realtime delivery and a push.
type PresenceChecker interface {
	IsOnline(userID uuid.UUID) bool
}

// NewChatUsecase wires the chat usecase.
func NewChatUsecase(
	chat domain.ChatRepository,
	bookings domain.BookingRepository,
	drivers domain.DriverRepository,
	companies domain.CompanyRepository,
	users domain.UserRepository,
	files *FileUsecase,
	tx domain.TxManager,
	events domain.EventPublisher,
	notifier domain.Notifier,
	presence PresenceChecker,
	log logger.Logger,
) *ChatUsecase {
	return &ChatUsecase{
		chat: chat, bookings: bookings, drivers: drivers, companies: companies,
		users: users, files: files, tx: tx, events: events, notifier: notifier,
		presence: presence, log: log,
	}
}

// EnsureBookingRoom returns the conversation for a booking, creating it on first
// use with both sides as participants.
//
// The room is created lazily rather than at booking time so a booking that is
// rejected in seconds never leaves an empty conversation behind.
func (u *ChatUsecase) EnsureBookingRoom(ctx context.Context, userID, bookingID uuid.UUID) (*dto.ChatRoomResponse, error) {
	booking, err := u.bookings.FindByID(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if err := u.assertBookingParty(ctx, userID, booking); err != nil {
		return nil, err
	}

	existing, err := u.chat.FindRoomByBooking(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return u.Get(ctx, userID, existing.ID)
	}

	driver, err := u.drivers.FindByID(ctx, booking.DriverID)
	if err != nil {
		return nil, err
	}
	members, err := u.companies.ListMembers(ctx, booking.CompanyID, true)
	if err != nil {
		return nil, err
	}

	room := &domain.ChatRoom{
		ID:        uuid.New(),
		BookingID: &booking.LoadID, // placeholder, corrected below
		Type:      domain.ChatRoomBooking,
		Status:    domain.ChatRoomActive,
	}
	room.BookingID = &bookingID
	room.LoadID = &booking.LoadID

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.chat.CreateRoom(txCtx, room); err != nil {
			return err
		}
		if err := u.chat.AddParticipant(txCtx, &domain.ChatParticipant{
			ID: uuid.New(), RoomID: room.ID, UserID: driver.UserID, Role: "driver",
		}); err != nil {
			return err
		}
		// Every active dispatcher joins, so a conversation is not lost when one
		// person goes off shift.
		for _, m := range members {
			if err := u.chat.AddParticipant(txCtx, &domain.ChatParticipant{
				ID: uuid.New(), RoomID: room.ID, UserID: m.UserID, Role: string(m.MemberRole),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return u.Get(ctx, userID, room.ID)
}

// SendMessage persists a message and fans it out.
func (u *ChatUsecase) SendMessage(ctx context.Context, userID, roomID uuid.UUID, req dto.SendMessageRequest) (*dto.MessageResponse, error) {
	room, err := u.chat.FindRoomByID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if _, err := u.chat.FindParticipant(ctx, roomID, userID); err != nil {
		if apperror.Is(err, apperror.ErrNotFound.Code) {
			return nil, apperror.ErrForbidden.WithMessage("You are not in this conversation")
		}
		return nil, err
	}
	if !room.CanPost() {
		return nil, apperror.ErrConflict.WithMessage("This conversation is closed")
	}

	// An idempotent retry returns the message that already landed rather than
	// posting a duplicate.
	if req.ClientMsgID != "" {
		if existing, err := u.chat.FindMessageByClientID(ctx, roomID, userID, req.ClientMsgID); err != nil {
			return nil, err
		} else if existing != nil {
			return u.messageResponse(ctx, existing, userID), nil
		}
	}

	messageType := domain.MessageType(req.Type)
	if messageType == "" {
		messageType = domain.MessageText
	}

	message := &domain.ChatMessage{
		ID:           uuid.New(),
		RoomID:       roomID,
		SenderUserID: &userID,
		Type:         messageType,
		ReplyToID:    req.ReplyToID,
		CreatedAt:    time.Now().UTC(),
	}
	if req.ClientMsgID != "" {
		message.ClientMsgID = &req.ClientMsgID
	}

	switch messageType {
	case domain.MessageText:
		body := strings.TrimSpace(req.Body)
		if body == "" {
			return nil, apperror.ErrValidation.
				WithDetails(apperror.FieldError{Field: "body", Message: "The message is empty"})
		}
		message.Body = &body
	case domain.MessageImage, domain.MessageFile:
		if req.FileID == nil {
			return nil, apperror.ErrValidation.
				WithDetails(apperror.FieldError{Field: "file_id", Message: "Attach a file"})
		}
		if err := u.files.AssertOwnership(ctx, userID, *req.FileID); err != nil {
			return nil, err
		}
		message.FileID = req.FileID
		if body := strings.TrimSpace(req.Body); body != "" {
			message.Body = &body
		}
	case domain.MessageLocation:
		if req.Location == nil {
			return nil, apperror.ErrValidation.
				WithDetails(apperror.FieldError{Field: "location", Message: "Coordinates are required"})
		}
		point := geo.NewPoint(req.Location.Lat, req.Location.Lng)
		if !point.Valid() {
			return nil, apperror.ErrValidation.WithMessage("Invalid coordinates")
		}
		message.Location = &point
	default:
		return nil, apperror.ErrValidation.WithMessage("Unsupported message type")
	}

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.chat.CreateMessage(txCtx, message); err != nil {
			return err
		}
		return u.chat.IncrementUnread(txCtx, roomID, userID)
	})
	if err != nil {
		return nil, err
	}

	stored, err := u.chat.FindMessageByID(ctx, message.ID)
	if err != nil {
		return nil, err
	}
	res := u.messageResponse(ctx, stored, userID)
	u.fanOut(ctx, room, stored, res, userID)
	return res, nil
}

// ListMessages returns a page of history, newest first.
func (u *ChatUsecase) ListMessages(ctx context.Context, userID, roomID uuid.UUID, before *time.Time, limit int) ([]dto.MessageResponse, error) {
	if err := u.AssertParticipant(ctx, userID, roomID); err != nil {
		return nil, err
	}
	messages, err := u.chat.ListMessages(ctx, roomID, before, limit)
	if err != nil {
		return nil, err
	}
	out := make([]dto.MessageResponse, 0, len(messages))
	for i := range messages {
		out = append(out, *u.messageResponse(ctx, &messages[i], userID))
	}
	return out, nil
}

// ListRooms returns the caller's conversations.
func (u *ChatUsecase) ListRooms(ctx context.Context, userID uuid.UUID, offset, limit int) ([]dto.ChatRoomResponse, int64, error) {
	rooms, total, err := u.chat.ListRooms(ctx, userID, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.ChatRoomResponse, 0, len(rooms))
	for i := range rooms {
		out = append(out, *u.roomResponse(ctx, &rooms[i], userID))
	}
	return out, total, nil
}

// Get returns one conversation with its latest message.
func (u *ChatUsecase) Get(ctx context.Context, userID, roomID uuid.UUID) (*dto.ChatRoomResponse, error) {
	if err := u.AssertParticipant(ctx, userID, roomID); err != nil {
		return nil, err
	}
	room, err := u.chat.FindRoomByID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	return u.roomResponse(ctx, room, userID), nil
}

// MarkRead advances the caller's read pointer and tells the other side.
func (u *ChatUsecase) MarkRead(ctx context.Context, userID, roomID, messageID uuid.UUID) error {
	if err := u.AssertParticipant(ctx, userID, roomID); err != nil {
		return err
	}
	if err := u.chat.MarkRead(ctx, roomID, userID, messageID); err != nil {
		return err
	}

	// A read receipt is only interesting to the other participants.
	recipients, err := u.otherParticipants(ctx, roomID, userID)
	if err != nil || len(recipients) == 0 {
		return nil
	}
	if u.events != nil {
		if err := u.events.PublishToUsers(ctx, recipients, domain.EventChatRead, map[string]any{
			"room_id":    roomID,
			"user_id":    userID,
			"message_id": messageID,
		}); err != nil {
			u.log.Warn("publishing read receipt failed", logger.Err(err))
		}
	}
	return nil
}

// SetMuted silences notifications from one conversation.
func (u *ChatUsecase) SetMuted(ctx context.Context, userID, roomID uuid.UUID, muted bool) error {
	if err := u.AssertParticipant(ctx, userID, roomID); err != nil {
		return err
	}
	return u.chat.SetMuted(ctx, roomID, userID, muted)
}

// AssertParticipant rejects a caller who is not in the room.
func (u *ChatUsecase) AssertParticipant(ctx context.Context, userID, roomID uuid.UUID) error {
	if _, err := u.chat.FindParticipant(ctx, roomID, userID); err != nil {
		if apperror.Is(err, apperror.ErrNotFound.Code) {
			return apperror.ErrForbidden.WithMessage("You are not in this conversation")
		}
		return err
	}
	return nil
}

// ParticipantUserIDs lists everyone in a room; used by the socket for typing
// indicators.
func (u *ChatUsecase) ParticipantUserIDs(ctx context.Context, roomID uuid.UUID) ([]uuid.UUID, error) {
	participants, err := u.chat.ListParticipants(ctx, roomID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(participants))
	for _, p := range participants {
		ids = append(ids, p.UserID)
	}
	return ids, nil
}

// UnreadCount returns the caller's total unread messages across all rooms.
func (u *ChatUsecase) UnreadCount(ctx context.Context, userID uuid.UUID) (int64, error) {
	return u.chat.TotalUnread(ctx, userID)
}

// ArchiveStale is the scheduled job that closes conversations for long-finished trips.
func (u *ChatUsecase) ArchiveStale(ctx context.Context) (int64, error) {
	return u.chat.ArchiveStaleRooms(ctx, time.Now().UTC().Add(-archiveChatAfter))
}

// ─── internals ──────────────────────────────────────────────────────

// fanOut delivers a message over the socket and pushes to whoever is offline.
//
// Pushing to a user who already received the message over their socket would
// double-notify them, so presence decides.
func (u *ChatUsecase) fanOut(ctx context.Context, room *domain.ChatRoom, message *domain.ChatMessage, res *dto.MessageResponse, senderID uuid.UUID) {
	participants, err := u.chat.ListParticipants(ctx, room.ID)
	if err != nil {
		u.log.Warn("resolving chat participants failed", logger.Err(err))
		return
	}

	payload := map[string]any{
		"room_id":    room.ID,
		"message_id": message.ID,
		"sender_id":  senderID,
		"type":       string(message.Type),
		"created_at": message.CreatedAt,
	}
	if message.Body != nil {
		payload["body"] = *message.Body
	}

	recipients := make([]uuid.UUID, 0, len(participants))
	var offline []domain.Notification
	preview := messagePreview(message)

	for _, p := range participants {
		if p.UserID == senderID {
			continue
		}
		recipients = append(recipients, p.UserID)
		if p.IsMuted {
			continue
		}
		if u.presence != nil && u.presence.IsOnline(p.UserID) {
			continue
		}
		offline = append(offline, domain.Notification{
			UserID: p.UserID,
			Type:   "chat",
			Title:  res.SenderName,
			Body:   preview,
			Data: map[string]any{
				"room_id":    room.ID.String(),
				"message_id": message.ID.String(),
			},
		})
	}

	if u.events != nil && len(recipients) > 0 {
		if err := u.events.PublishToUsers(ctx, recipients, domain.EventChatMessage, payload); err != nil {
			u.log.Warn("publishing chat message failed", logger.Err(err))
		}
	}
	if u.notifier != nil && len(offline) > 0 {
		if err := u.notifier.NotifyMany(ctx, offline); err != nil {
			u.log.Warn("queueing chat push failed", logger.Err(err))
		}
	}
}

func (u *ChatUsecase) otherParticipants(ctx context.Context, roomID, exceptUserID uuid.UUID) ([]uuid.UUID, error) {
	ids, err := u.ParticipantUserIDs(ctx, roomID)
	if err != nil {
		return nil, err
	}
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id != exceptUserID {
			out = append(out, id)
		}
	}
	return out, nil
}

// assertBookingParty allows the driver or an active member of the shipper company.
func (u *ChatUsecase) assertBookingParty(ctx context.Context, userID uuid.UUID, booking *domain.Booking) error {
	driver, err := u.drivers.FindByUserID(ctx, userID)
	if err == nil && driver.ID == booking.DriverID {
		return nil
	}
	if err != nil && !apperror.Is(err, apperror.ErrNotFound.Code) {
		return err
	}
	member, err := u.companies.FindMember(ctx, booking.CompanyID, userID)
	if err == nil && member.IsActive {
		return nil
	}
	if err != nil && !apperror.Is(err, apperror.ErrNotFound.Code) {
		return err
	}
	return apperror.ErrForbidden.WithMessage("You are not party to this booking")
}

func (u *ChatUsecase) messageResponse(ctx context.Context, m *domain.ChatMessage, viewerID uuid.UUID) *dto.MessageResponse {
	res := &dto.MessageResponse{
		ID:          m.ID,
		RoomID:      m.RoomID,
		SenderID:    m.SenderUserID,
		Type:        string(m.Type),
		Body:        m.Body,
		FileID:      m.FileID,
		ReplyToID:   m.ReplyToID,
		ClientMsgID: m.ClientMsgID,
		IsMine:      m.SenderUserID != nil && *m.SenderUserID == viewerID,
		EditedAt:    m.EditedAt,
		CreatedAt:   m.CreatedAt,
	}
	if m.Sender != nil {
		res.SenderName = m.Sender.FullName()
		res.SenderAvatar = u.files.ResolveURLByID(ctx, m.Sender.AvatarFileID)
	}
	if m.File != nil {
		res.FileURL = u.files.ResolveURL(ctx, m.File)
	} else if m.FileID != nil {
		if url := u.files.ResolveURLByID(ctx, m.FileID); url != nil {
			res.FileURL = *url
		}
	}
	if m.Location != nil && m.Location.Valid() {
		res.Location = &dto.PointResponse{Lat: m.Location.Lat, Lng: m.Location.Lng}
	}
	return res
}

func (u *ChatUsecase) roomResponse(ctx context.Context, room *domain.ChatRoom, viewerID uuid.UUID) *dto.ChatRoomResponse {
	res := &dto.ChatRoomResponse{
		ID:            room.ID,
		Type:          string(room.Type),
		Status:        string(room.Status),
		BookingID:     room.BookingID,
		LoadID:        room.LoadID,
		TripID:        room.TripID,
		LastMessageAt: room.LastMessageAt,
		Participants:  []dto.ChatParticipantResponse{},
		CreatedAt:     room.CreatedAt,
	}
	if room.Load != nil {
		res.LoadReference = room.Load.ReferenceNo
		res.LoadTitle = room.Load.Title
	}

	for i := range room.Participants {
		p := room.Participants[i]
		if p.UserID == viewerID {
			res.UnreadCount = p.UnreadCount
			res.IsMuted = p.IsMuted
		}
		item := dto.ChatParticipantResponse{
			UserID: p.UserID,
			Role:   p.Role,
		}
		if u.presence != nil {
			item.IsOnline = u.presence.IsOnline(p.UserID)
		}
		if p.User != nil {
			item.FullName = p.User.FullName()
			item.AvatarURL = u.files.ResolveURLByID(ctx, p.User.AvatarFileID)
		}
		res.Participants = append(res.Participants, item)
	}

	// One extra query per room, but the conversation list is unusable without a
	// preview of the latest message.
	if messages, err := u.chat.ListMessages(ctx, room.ID, nil, 1); err == nil && len(messages) > 0 {
		res.LastMessage = u.messageResponse(ctx, &messages[0], viewerID)
	}
	return res
}

// messagePreview renders a push-safe one-liner for a message of any type.
func messagePreview(m *domain.ChatMessage) string {
	switch m.Type {
	case domain.MessageImage:
		return "📷 Photo"
	case domain.MessageFile:
		return "📎 File"
	case domain.MessageLocation:
		return "📍 Location"
	case domain.MessageSystem:
		if m.Body != nil {
			return *m.Body
		}
		return "System message"
	default:
		if m.Body == nil {
			return ""
		}
		body := *m.Body
		if len(body) > 120 {
			return body[:117] + "..."
		}
		return body
	}
}
