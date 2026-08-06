package ws

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
)

// publisher adapts the hub to domain.EventPublisher so usecases can emit events
// without importing anything from the delivery layer.
type publisher struct{ hub *Hub }

// NewPublisher returns the hub as a domain.EventPublisher.
func NewPublisher(hub *Hub) domain.EventPublisher { return &publisher{hub: hub} }

func (p *publisher) PublishToUser(ctx context.Context, userID uuid.UUID, event domain.EventType, payload map[string]any) error {
	return p.hub.Publish(ctx, Envelope{
		Type:    string(event),
		Payload: payload,
		UserIDs: []uuid.UUID{userID},
	})
}

func (p *publisher) PublishToUsers(ctx context.Context, userIDs []uuid.UUID, event domain.EventType, payload map[string]any) error {
	if len(userIDs) == 0 {
		return nil
	}
	return p.hub.Publish(ctx, Envelope{
		Type:    string(event),
		Payload: payload,
		UserIDs: userIDs,
	})
}

func (p *publisher) PublishToCompany(ctx context.Context, companyID uuid.UUID, event domain.EventType, payload map[string]any) error {
	return p.hub.Publish(ctx, Envelope{
		Type:      string(event),
		Payload:   payload,
		CompanyID: &companyID,
	})
}

func (p *publisher) PublishToTrip(ctx context.Context, tripID uuid.UUID, event domain.EventType, payload map[string]any) error {
	return p.hub.Publish(ctx, Envelope{
		Type:    string(event),
		Payload: payload,
		TripID:  &tripID,
	})
}

// PublishLoad broadcasts a new load to every connected client.
//
// Server-side matching against each driver's saved filters happens in the
// notification path, which can afford a database round trip. The socket
// broadcast is the low-latency "something new is on the board" signal, and
// clients filter it locally; the payload therefore contains no contact details.
func (p *publisher) PublishLoad(ctx context.Context, event domain.EventType, loadID uuid.UUID, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["load_id"] = loadID
	return p.hub.Publish(ctx, Envelope{
		Type:      string(event),
		Payload:   payload,
		Broadcast: true,
	})
}

// nowUTC exists so the handshake payload and envelopes agree on the clock.
func nowUTC() time.Time { return time.Now().UTC() }
