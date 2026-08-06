package ws

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/geo"
	"github.com/loadify/backend/pkg/logger"
)

// Inbound message types a client may send.
const (
	msgPing        = "ping"
	msgLocation    = "location.update"
	msgWatchTrip   = "trip.watch"
	msgUnwatchTrip = "trip.unwatch"
	msgChatSend    = "chat.send"
	msgChatRead    = "chat.read"
	msgChatTyping  = "chat.typing"
)

// ChatService is the slice of the chat usecase the socket needs. Depending on an
// interface rather than the concrete usecase keeps the import direction clean.
type ChatService interface {
	SendMessage(ctx context.Context, userID uuid.UUID, roomID uuid.UUID, req dto.SendMessageRequest) (*dto.MessageResponse, error)
	MarkRead(ctx context.Context, userID, roomID, messageID uuid.UUID) error
	AssertParticipant(ctx context.Context, userID, roomID uuid.UUID) error
	ParticipantUserIDs(ctx context.Context, roomID uuid.UUID) ([]uuid.UUID, error)
}

// LocationService is the slice of the driver usecase the socket needs.
type LocationService interface {
	RecordLocation(ctx context.Context, userID uuid.UUID, sample domain.DriverLocation) error
}

// TripAccess authorises a client to follow a trip.
type TripAccess interface {
	CanWatchTrip(ctx context.Context, userID, tripID uuid.UUID) error
}

// Router dispatches inbound socket messages.
type Router struct {
	hub       *Hub
	chat      ChatService
	locations LocationService
	trips     TripAccess
	log       logger.Logger
}

// NewRouter builds the inbound message router.
func NewRouter(hub *Hub, chat ChatService, locations LocationService, trips TripAccess, log logger.Logger) *Router {
	return &Router{hub: hub, chat: chat, locations: locations, trips: trips, log: log}
}

// handle processes one inbound message.
func (r *Router) handle(ctx context.Context, c *Client, msg InboundMessage) {
	switch msg.Type {
	case msgPing:
		c.sendEvent("pong", nil)

	case msgLocation:
		r.handleLocation(ctx, c, msg)

	case msgWatchTrip:
		r.handleWatchTrip(ctx, c, msg)

	case msgUnwatchTrip:
		if tripID, ok := uuidFrom(msg.Payload, "trip_id"); ok {
			r.hub.unwatchTrip(c, tripID)
			c.sendEvent("trip.unwatched", map[string]any{"trip_id": tripID})
		}

	case msgChatSend:
		r.handleChatSend(ctx, c, msg)

	case msgChatRead:
		r.handleChatRead(ctx, c, msg)

	case msgChatTyping:
		r.handleTyping(ctx, c, msg)

	default:
		c.sendError("UNKNOWN_TYPE", "Unsupported message type: "+msg.Type)
	}
}

// handleLocation ingests a position report from a driver.
//
// This is the hot path: a driver on a trip sends one of these every 15 seconds,
// which is why it goes over the socket rather than as an HTTP request.
func (r *Router) handleLocation(ctx context.Context, c *Client, msg InboundMessage) {
	if c.driverID == nil {
		c.sendError("FORBIDDEN", "Only drivers report location")
		return
	}
	if r.locations == nil {
		return
	}

	lat, latOK := floatFrom(msg.Payload, "lat")
	lng, lngOK := floatFrom(msg.Payload, "lng")
	if !latOK || !lngOK {
		c.sendError("VALIDATION_ERROR", "lat and lng are required")
		return
	}
	point := geo.NewPoint(lat, lng)
	if !point.Valid() {
		c.sendError("VALIDATION_ERROR", "Invalid coordinates")
		return
	}

	recordedAt := time.Now().UTC()
	if ts, ok := timeFrom(msg.Payload, "recorded_at"); ok {
		// Accept a back-dated point from an offline queue, but never a future
		// one: a device with a wrong clock would otherwise block later updates.
		if ts.After(recordedAt.Add(time.Minute)) {
			c.sendError("VALIDATION_ERROR", "recorded_at is in the future")
			return
		}
		recordedAt = ts
	}

	sample := domain.DriverLocation{
		DriverID:   *c.driverID,
		Location:   point,
		RecordedAt: recordedAt,
	}
	if speed, ok := floatFrom(msg.Payload, "speed_kmh"); ok {
		sample.SpeedKmh = &speed
		sample.IsMoving = speed > 1
	}
	if heading, ok := floatFrom(msg.Payload, "heading"); ok {
		sample.Heading = &heading
	}
	if accuracy, ok := floatFrom(msg.Payload, "accuracy_m"); ok {
		sample.AccuracyM = &accuracy
	}
	if battery, ok := floatFrom(msg.Payload, "battery_level"); ok {
		level := int16(battery)
		sample.BatteryLevel = &level
	}
	if tripID, ok := uuidFrom(msg.Payload, "trip_id"); ok {
		sample.TripID = &tripID
	}

	if err := r.locations.RecordLocation(ctx, c.userID, sample); err != nil {
		r.log.Warn("recording location over websocket failed",
			logger.String("user_id", c.userID.String()), logger.Err(err))
		c.sendError(apperror.As(err).Code, apperror.As(err).Message)
		return
	}

	// Fan the position out to whoever is watching this trip.
	payload := map[string]any{
		"driver_id":   c.driverID,
		"lat":         point.Lat,
		"lng":         point.Lng,
		"recorded_at": recordedAt,
	}
	if sample.SpeedKmh != nil {
		payload["speed_kmh"] = *sample.SpeedKmh
	}
	if sample.Heading != nil {
		payload["heading"] = *sample.Heading
	}
	env := Envelope{Type: string(domain.EventTripLocation), Payload: payload}
	if sample.TripID != nil {
		payload["trip_id"] = *sample.TripID
		env.TripID = sample.TripID
	}
	if c.companyID != nil {
		// An employed driver's own company always sees them on the fleet map.
		env.CompanyID = c.companyID
	}
	if err := r.hub.Publish(ctx, env); err != nil {
		r.log.Warn("publishing location failed", logger.Err(err))
	}
}

// handleWatchTrip subscribes a client to a trip after checking they may see it.
func (r *Router) handleWatchTrip(ctx context.Context, c *Client, msg InboundMessage) {
	tripID, ok := uuidFrom(msg.Payload, "trip_id")
	if !ok {
		c.sendError("VALIDATION_ERROR", "trip_id is required")
		return
	}
	if r.trips != nil {
		if err := r.trips.CanWatchTrip(ctx, c.userID, tripID); err != nil {
			appErr := apperror.As(err)
			c.sendError(appErr.Code, appErr.Message)
			return
		}
	}
	r.hub.watchTrip(c, tripID)
	c.sendEvent("trip.watching", map[string]any{"trip_id": tripID})
}

// handleChatSend persists and fans out a chat message.
func (r *Router) handleChatSend(ctx context.Context, c *Client, msg InboundMessage) {
	if r.chat == nil {
		c.sendError("SERVICE_UNAVAILABLE", "Chat is not available")
		return
	}
	roomID, ok := uuidFrom(msg.Payload, "room_id")
	if !ok {
		c.sendError("VALIDATION_ERROR", "room_id is required")
		return
	}

	req := dto.SendMessageRequest{
		Type: stringFrom(msg.Payload, "type"),
		Body: stringFrom(msg.Payload, "body"),
	}
	if req.Type == "" {
		req.Type = string(domain.MessageText)
	}
	req.ClientMsgID = stringFrom(msg.Payload, "client_msg_id")
	if fileID, ok := uuidFrom(msg.Payload, "file_id"); ok {
		req.FileID = &fileID
	}
	if lat, latOK := floatFrom(msg.Payload, "lat"); latOK {
		if lng, lngOK := floatFrom(msg.Payload, "lng"); lngOK {
			req.Location = &dto.PointRequest{Lat: lat, Lng: lng}
		}
	}
	if replyTo, ok := uuidFrom(msg.Payload, "reply_to_id"); ok {
		req.ReplyToID = &replyTo
	}

	// The usecase owns membership, room status and fan-out; the socket only
	// carries the frame.
	if _, err := r.chat.SendMessage(ctx, c.userID, roomID, req); err != nil {
		appErr := apperror.As(err)
		c.sendError(appErr.Code, appErr.Message)
	}
}

// handleChatRead advances the caller's read pointer and tells the room.
func (r *Router) handleChatRead(ctx context.Context, c *Client, msg InboundMessage) {
	if r.chat == nil {
		return
	}
	roomID, ok := uuidFrom(msg.Payload, "room_id")
	if !ok {
		c.sendError("VALIDATION_ERROR", "room_id is required")
		return
	}
	messageID, ok := uuidFrom(msg.Payload, "message_id")
	if !ok {
		c.sendError("VALIDATION_ERROR", "message_id is required")
		return
	}
	if err := r.chat.MarkRead(ctx, c.userID, roomID, messageID); err != nil {
		appErr := apperror.As(err)
		c.sendError(appErr.Code, appErr.Message)
	}
}

// handleTyping relays a typing indicator. It is deliberately not persisted:
// the signal is worthless a second later.
func (r *Router) handleTyping(ctx context.Context, c *Client, msg InboundMessage) {
	roomID, ok := uuidFrom(msg.Payload, "room_id")
	if !ok {
		return
	}
	if r.chat == nil {
		return
	}
	if err := r.chat.AssertParticipant(ctx, c.userID, roomID); err != nil {
		return
	}

	// Send it to the other participants, never back to the typist.
	recipients, err := r.chat.ParticipantUserIDs(ctx, roomID)
	if err != nil {
		return
	}
	targets := make([]uuid.UUID, 0, len(recipients))
	for _, id := range recipients {
		if id != c.userID {
			targets = append(targets, id)
		}
	}
	if len(targets) == 0 {
		return
	}

	if err := r.hub.Publish(ctx, Envelope{
		Type: "chat.typing",
		Payload: map[string]any{
			"room_id": roomID,
			"user_id": c.userID,
		},
		UserIDs: targets,
	}); err != nil {
		r.log.Warn("publishing typing indicator failed", logger.Err(err))
	}
}

// ─── payload helpers ────────────────────────────────────────────────
//
// Inbound payloads are decoded into map[string]any, so numbers arrive as
// float64 and ids as strings. These helpers do the narrowing in one place.

func stringFrom(payload map[string]any, key string) string {
	if v, ok := payload[key].(string); ok {
		return v
	}
	return ""
}

func floatFrom(payload map[string]any, key string) (float64, bool) {
	switch v := payload[key].(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}

func uuidFrom(payload map[string]any, key string) (uuid.UUID, bool) {
	raw, ok := payload[key].(string)
	if !ok || raw == "" {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

func timeFrom(payload map[string]any, key string) (time.Time, bool) {
	raw, ok := payload[key].(string)
	if !ok || raw == "" {
		return time.Time{}, false
	}
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	return ts.UTC(), true
}
