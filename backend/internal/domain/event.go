package domain

import (
	"context"

	"github.com/google/uuid"
)

// EventType names a realtime event. Clients subscribe by these names, so the
// values are part of the public contract and must not change casually.
type EventType string

const (
	EventLoadPublished    EventType = "load.published"
	EventLoadCancelled    EventType = "load.cancelled"
	EventBookingCreated   EventType = "booking.created"
	EventBookingOffered   EventType = "booking.offered"
	EventBookingAccepted  EventType = "booking.accepted"
	EventBookingRejected  EventType = "booking.rejected"
	EventBookingCancelled EventType = "booking.cancelled"
	EventBookingExpired   EventType = "booking.expired"
	EventTripStatusChange EventType = "trip.status_changed"
	EventTripLocation     EventType = "trip.location"
	EventTripCompleted    EventType = "trip.completed"
	EventDriverLocation   EventType = "driver.location"
	EventChatMessage      EventType = "chat.message"
	EventChatRead         EventType = "chat.read"
	EventNotification     EventType = "notification.new"
)

// EventPublisher pushes events to the realtime layer.
//
// Usecases depend on this port, never on the WebSocket hub. Every method is
// expected to be best-effort: a delivery failure is logged by the caller and
// must not roll back the business action that produced it.
type EventPublisher interface {
	// PublishToUser sends an event to every live connection of one user.
	PublishToUser(ctx context.Context, userID uuid.UUID, event EventType, payload map[string]any) error
	// PublishToUsers fans an event out to several users in one call.
	PublishToUsers(ctx context.Context, userIDs []uuid.UUID, event EventType, payload map[string]any) error
	// PublishToCompany sends an event to every active member of a company.
	PublishToCompany(ctx context.Context, companyID uuid.UUID, event EventType, payload map[string]any) error
	// PublishToTrip sends an event to the parties watching one trip.
	PublishToTrip(ctx context.Context, tripID uuid.UUID, event EventType, payload map[string]any) error
	// PublishLoad broadcasts a marketplace-wide load event; the realtime layer
	// decides which subscribed drivers actually match it.
	PublishLoad(ctx context.Context, event EventType, loadID uuid.UUID, payload map[string]any) error
}

// Notification is a message queued for a user across one or more channels.
type Notification struct {
	UserID    uuid.UUID
	Type      string
	Title     string
	Body      string
	Data      map[string]any
	Priority  string
	ActionURL string
}

// Notifier delivers notifications. Like EventPublisher it is best-effort from
// the caller's point of view; persistence and retries are its own concern.
type Notifier interface {
	Notify(ctx context.Context, n Notification) error
	NotifyMany(ctx context.Context, notifications []Notification) error
}
