// Package ws implements the WebSocket layer: a per-node hub of live connections,
// plus Redis pub/sub so an event published on one node reaches subscribers on
// every other node.
package ws

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/infrastructure/cache"
	"github.com/loadify/backend/pkg/logger"
)

const (
	// redisChannel is the single fan-out channel. One channel keeps the
	// subscription set constant; routing happens from the envelope.
	redisChannel = "loadify:events"

	// sendBuffer is how many messages may queue for one connection before it is
	// considered too slow and dropped. A stalled mobile client must not be able
	// to block the hub.
	sendBuffer = 64
)

// Envelope is the wire format for every message in both directions.
type Envelope struct {
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload,omitempty"`
	Timestamp time.Time      `json:"ts"`
	// Routing fields are used by the hub and stripped before delivery.
	UserIDs   []uuid.UUID `json:"-"`
	CompanyID *uuid.UUID  `json:"-"`
	TripID    *uuid.UUID  `json:"-"`
	Broadcast bool        `json:"-"`
}

// routedEnvelope is what actually crosses Redis: the payload plus its routing.
type routedEnvelope struct {
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload,omitempty"`
	Timestamp time.Time      `json:"ts"`
	UserIDs   []uuid.UUID    `json:"user_ids,omitempty"`
	CompanyID *uuid.UUID     `json:"company_id,omitempty"`
	TripID    *uuid.UUID     `json:"trip_id,omitempty"`
	Broadcast bool           `json:"broadcast,omitempty"`
	// Origin identifies the publishing node so it does not re-deliver its own
	// message after the round trip.
	Origin string `json:"origin"`
}

// Hub owns the live connections on this node.
type Hub struct {
	mu sync.RWMutex
	// clients is indexed by user so a targeted event is an O(1) lookup; a user
	// with a phone and a browser open has two entries.
	clients map[uuid.UUID]map[*Client]struct{}
	// tripWatchers tracks who asked to follow a specific trip's live position.
	tripWatchers map[uuid.UUID]map[*Client]struct{}

	cache   cache.Cache
	log     logger.Logger
	nodeID  string
	members domain.CompanyMemberLookup

	done chan struct{}
	once sync.Once
}

// NewHub builds a hub. members resolves company fan-out; it may be nil, in which
// case company-scoped events reach nobody rather than everybody.
func NewHub(c cache.Cache, members domain.CompanyMemberLookup, log logger.Logger) *Hub {
	return &Hub{
		clients:      make(map[uuid.UUID]map[*Client]struct{}),
		tripWatchers: make(map[uuid.UUID]map[*Client]struct{}),
		cache:        c,
		log:          log,
		nodeID:       uuid.NewString(),
		members:      members,
		done:         make(chan struct{}),
	}
}

// Run starts the Redis subscription that carries cross-node events. It blocks
// until Close is called.
func (h *Hub) Run(ctx context.Context) {
	if h.cache == nil {
		h.log.Warn("realtime hub running without redis: events stay on this node only")
		<-h.done
		return
	}

	sub := h.cache.Subscribe(ctx, redisChannel)
	defer func() { _ = sub.Close() }()

	channel := sub.Channel()
	h.log.Info("realtime hub started", logger.String("node_id", h.nodeID))

	for {
		select {
		case <-h.done:
			return
		case <-ctx.Done():
			return
		case msg, ok := <-channel:
			if !ok {
				return
			}
			var routed routedEnvelope
			if err := json.Unmarshal([]byte(msg.Payload), &routed); err != nil {
				h.log.Warn("dropping malformed realtime message", logger.Err(err))
				continue
			}
			// A node already delivered locally before publishing.
			if routed.Origin == h.nodeID {
				continue
			}
			h.deliverLocal(ctx, routed)
		}
	}
}

// Close stops the hub and disconnects every client.
func (h *Hub) Close() {
	h.once.Do(func() {
		close(h.done)
		h.mu.Lock()
		for _, set := range h.clients {
			for client := range set {
				client.close()
			}
		}
		h.clients = make(map[uuid.UUID]map[*Client]struct{})
		h.tripWatchers = make(map[uuid.UUID]map[*Client]struct{})
		h.mu.Unlock()
	})
}

// register adds a connection.
func (h *Hub) register(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[client.userID] == nil {
		h.clients[client.userID] = make(map[*Client]struct{})
	}
	h.clients[client.userID][client] = struct{}{}
}

// unregister removes a connection and any trip subscriptions it held.
func (h *Hub) unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if set, ok := h.clients[client.userID]; ok {
		delete(set, client)
		if len(set) == 0 {
			delete(h.clients, client.userID)
		}
	}
	for tripID, watchers := range h.tripWatchers {
		if _, ok := watchers[client]; ok {
			delete(watchers, client)
			if len(watchers) == 0 {
				delete(h.tripWatchers, tripID)
			}
		}
	}
}

// watchTrip subscribes a connection to a trip's live events.
func (h *Hub) watchTrip(client *Client, tripID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.tripWatchers[tripID] == nil {
		h.tripWatchers[tripID] = make(map[*Client]struct{})
	}
	h.tripWatchers[tripID][client] = struct{}{}
}

// unwatchTrip removes one trip subscription.
func (h *Hub) unwatchTrip(client *Client, tripID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if watchers, ok := h.tripWatchers[tripID]; ok {
		delete(watchers, client)
		if len(watchers) == 0 {
			delete(h.tripWatchers, tripID)
		}
	}
}

// IsOnline reports whether a user has a live connection on this node.
func (h *Hub) IsOnline(userID uuid.UUID) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[userID]) > 0
}

// Connections returns the current connection count, for the health endpoint.
func (h *Hub) Connections() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	total := 0
	for _, set := range h.clients {
		total += len(set)
	}
	return total
}

// Publish delivers an event locally and forwards it to the other nodes.
//
// Local delivery happens first so a single-node deployment, or a Redis outage,
// still works for connections on this node.
func (h *Hub) Publish(ctx context.Context, env Envelope) error {
	if env.Timestamp.IsZero() {
		env.Timestamp = time.Now().UTC()
	}
	routed := routedEnvelope{
		Type:      env.Type,
		Payload:   env.Payload,
		Timestamp: env.Timestamp,
		UserIDs:   env.UserIDs,
		CompanyID: env.CompanyID,
		TripID:    env.TripID,
		Broadcast: env.Broadcast,
		Origin:    h.nodeID,
	}

	h.deliverLocal(ctx, routed)

	if h.cache == nil {
		return nil
	}
	return h.cache.Publish(ctx, redisChannel, routed)
}

// deliverLocal writes an envelope to the matching connections on this node.
func (h *Hub) deliverLocal(ctx context.Context, routed routedEnvelope) {
	outbound := Envelope{
		Type:      routed.Type,
		Payload:   routed.Payload,
		Timestamp: routed.Timestamp,
	}
	data, err := json.Marshal(outbound)
	if err != nil {
		h.log.Warn("encoding realtime message failed", logger.Err(err))
		return
	}

	targets := h.resolveTargets(ctx, routed)
	for client := range targets {
		client.send(data)
	}
}

// resolveTargets collects the connections an envelope should reach. Using a set
// means a client matching two routing rules still receives one copy.
func (h *Hub) resolveTargets(ctx context.Context, routed routedEnvelope) map[*Client]struct{} {
	targets := make(map[*Client]struct{})

	// Company fan-out is resolved before taking the lock: it may hit the
	// database, and holding the hub lock across a query would stall every send.
	var companyUserIDs []uuid.UUID
	if routed.CompanyID != nil && h.members != nil {
		ids, err := h.members.ActiveMemberUserIDs(ctx, *routed.CompanyID)
		if err != nil {
			h.log.Warn("resolving company members for realtime fan-out failed",
				logger.String("company_id", routed.CompanyID.String()), logger.Err(err))
		} else {
			companyUserIDs = ids
		}
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if routed.Broadcast {
		for _, set := range h.clients {
			for client := range set {
				targets[client] = struct{}{}
			}
		}
		return targets
	}

	for _, userID := range routed.UserIDs {
		for client := range h.clients[userID] {
			targets[client] = struct{}{}
		}
	}
	for _, userID := range companyUserIDs {
		for client := range h.clients[userID] {
			targets[client] = struct{}{}
		}
	}
	if routed.TripID != nil {
		for client := range h.tripWatchers[*routed.TripID] {
			targets[client] = struct{}{}
		}
	}
	return targets
}
