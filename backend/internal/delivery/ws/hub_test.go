package ws

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/loadify/backend/pkg/logger"
)

// stubMembers is a CompanyMemberLookup that answers from a fixed map.
type stubMembers struct {
	byCompany map[uuid.UUID][]uuid.UUID
	err       error
	calls     int
}

func (s *stubMembers) ActiveMemberUserIDs(_ context.Context, companyID uuid.UUID) ([]uuid.UUID, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.byCompany[companyID], nil
}

// newTestHub builds a hub with no Redis; routing is a pure in-memory concern.
func newTestHub(members *stubMembers) *Hub {
	return NewHub(nil, members, logger.New("test", "error"))
}

// attach registers a client for a user without needing a real socket, since
// routing never touches the connection.
func attach(h *Hub, userID uuid.UUID) *Client {
	c := &Client{hub: h, userID: userID, send_: make(chan []byte, sendBuffer)}
	h.register(c)
	return c
}

func TestResolveTargetsByUser(t *testing.T) {
	h := newTestHub(&stubMembers{})
	alice, bob := uuid.New(), uuid.New()
	aliceClient := attach(h, alice)
	bobClient := attach(h, bob)

	targets := h.resolveTargets(context.Background(), routedEnvelope{
		UserIDs: []uuid.UUID{alice},
	})

	if _, ok := targets[aliceClient]; !ok {
		t.Error("the addressed user was not targeted")
	}
	if _, ok := targets[bobClient]; ok {
		t.Error("an unaddressed user was targeted")
	}
}

func TestResolveTargetsReachesEveryDeviceOfAUser(t *testing.T) {
	h := newTestHub(&stubMembers{})
	alice := uuid.New()
	phone := attach(h, alice)
	web := attach(h, alice)

	targets := h.resolveTargets(context.Background(), routedEnvelope{
		UserIDs: []uuid.UUID{alice},
	})

	if len(targets) != 2 {
		t.Fatalf("targets = %d, want 2 (phone and web)", len(targets))
	}
	if _, ok := targets[phone]; !ok {
		t.Error("the phone connection was missed")
	}
	if _, ok := targets[web]; !ok {
		t.Error("the web connection was missed")
	}
}

func TestResolveTargetsByCompany(t *testing.T) {
	companyID := uuid.New()
	dispatcher, manager, outsider := uuid.New(), uuid.New(), uuid.New()

	members := &stubMembers{byCompany: map[uuid.UUID][]uuid.UUID{
		companyID: {dispatcher, manager},
	}}
	h := newTestHub(members)
	dispatcherClient := attach(h, dispatcher)
	managerClient := attach(h, manager)
	outsiderClient := attach(h, outsider)

	targets := h.resolveTargets(context.Background(), routedEnvelope{CompanyID: &companyID})

	if _, ok := targets[dispatcherClient]; !ok {
		t.Error("the dispatcher was not targeted")
	}
	if _, ok := targets[managerClient]; !ok {
		t.Error("the manager was not targeted")
	}
	if _, ok := targets[outsiderClient]; ok {
		t.Error("someone outside the company was targeted")
	}
}

// A failed member lookup must not silently turn a company-scoped event into a
// broadcast — the fallback has to be "nobody", not "everybody".
func TestResolveTargetsCompanyLookupFailureTargetsNobody(t *testing.T) {
	companyID := uuid.New()
	members := &stubMembers{err: errors.New("database is down")}
	h := newTestHub(members)
	attach(h, uuid.New())
	attach(h, uuid.New())

	targets := h.resolveTargets(context.Background(), routedEnvelope{CompanyID: &companyID})

	if len(targets) != 0 {
		t.Errorf("targets = %d, want 0 when the member lookup fails", len(targets))
	}
}

func TestResolveTargetsByTrip(t *testing.T) {
	h := newTestHub(&stubMembers{})
	tripID := uuid.New()
	watcher := attach(h, uuid.New())
	bystander := attach(h, uuid.New())
	h.watchTrip(watcher, tripID)

	targets := h.resolveTargets(context.Background(), routedEnvelope{TripID: &tripID})

	if _, ok := targets[watcher]; !ok {
		t.Error("the trip watcher was not targeted")
	}
	if _, ok := targets[bystander]; ok {
		t.Error("a non-watcher received a trip event")
	}
}

func TestResolveTargetsDeduplicatesOverlappingRules(t *testing.T) {
	companyID, tripID := uuid.New(), uuid.New()
	userID := uuid.New()
	members := &stubMembers{byCompany: map[uuid.UUID][]uuid.UUID{companyID: {userID}}}
	h := newTestHub(members)
	client := attach(h, userID)
	h.watchTrip(client, tripID)

	// The same connection matches all three routing rules at once.
	targets := h.resolveTargets(context.Background(), routedEnvelope{
		UserIDs:   []uuid.UUID{userID},
		CompanyID: &companyID,
		TripID:    &tripID,
	})

	if len(targets) != 1 {
		t.Errorf("targets = %d, want 1 — the client must not be sent duplicates", len(targets))
	}
}

func TestResolveTargetsBroadcast(t *testing.T) {
	h := newTestHub(&stubMembers{})
	attach(h, uuid.New())
	attach(h, uuid.New())
	attach(h, uuid.New())

	targets := h.resolveTargets(context.Background(), routedEnvelope{Broadcast: true})

	if len(targets) != 3 {
		t.Errorf("targets = %d, want 3", len(targets))
	}
}

// A broadcast must not pay for a company lookup it does not use.
func TestBroadcastSkipsCompanyLookup(t *testing.T) {
	companyID := uuid.New()
	members := &stubMembers{byCompany: map[uuid.UUID][]uuid.UUID{companyID: {uuid.New()}}}
	h := newTestHub(members)
	attach(h, uuid.New())

	h.resolveTargets(context.Background(), routedEnvelope{Broadcast: true})

	if members.calls != 0 {
		t.Errorf("member lookup called %d times during a broadcast, want 0", members.calls)
	}
}

func TestUnregisterRemovesClientAndTripWatch(t *testing.T) {
	h := newTestHub(&stubMembers{})
	userID, tripID := uuid.New(), uuid.New()
	client := attach(h, userID)
	h.watchTrip(client, tripID)

	if !h.IsOnline(userID) {
		t.Fatal("the user should be online after registering")
	}

	h.unregister(client)

	if h.IsOnline(userID) {
		t.Error("the user is still online after unregistering")
	}
	if h.Connections() != 0 {
		t.Errorf("Connections() = %d, want 0", h.Connections())
	}
	// The watcher entry must go too, or a disconnected client leaks and every
	// future event for that trip walks a dead reference.
	if got := len(h.resolveTargets(context.Background(), routedEnvelope{TripID: &tripID})); got != 0 {
		t.Errorf("trip still has %d watchers after the client disconnected", got)
	}
	h.mu.RLock()
	watcherEntries := len(h.tripWatchers)
	h.mu.RUnlock()
	if watcherEntries != 0 {
		t.Errorf("tripWatchers still holds %d entries", watcherEntries)
	}
}

func TestUnwatchTripLeavesConnectionRegistered(t *testing.T) {
	h := newTestHub(&stubMembers{})
	userID, tripID := uuid.New(), uuid.New()
	client := attach(h, userID)
	h.watchTrip(client, tripID)

	h.unwatchTrip(client, tripID)

	if !h.IsOnline(userID) {
		t.Error("unwatching a trip must not disconnect the client")
	}
	if got := len(h.resolveTargets(context.Background(), routedEnvelope{TripID: &tripID})); got != 0 {
		t.Errorf("trip still has %d watchers after unwatch", got)
	}
}

func TestConnectionsCountsEveryDevice(t *testing.T) {
	h := newTestHub(&stubMembers{})
	alice := uuid.New()
	attach(h, alice)
	attach(h, alice)
	attach(h, uuid.New())

	if got := h.Connections(); got != 3 {
		t.Errorf("Connections() = %d, want 3", got)
	}
}

// Publish without Redis must still deliver to connections on this node, so a
// single-node deployment or a Redis outage does not break realtime locally.
func TestPublishDeliversLocallyWithoutRedis(t *testing.T) {
	h := newTestHub(&stubMembers{})
	userID := uuid.New()
	client := attach(h, userID)

	err := h.Publish(context.Background(), Envelope{
		Type:    "test.event",
		Payload: map[string]any{"hello": "world"},
		UserIDs: []uuid.UUID{userID},
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	select {
	case data := <-client.send_:
		if len(data) == 0 {
			t.Error("an empty frame was queued")
		}
	default:
		t.Error("nothing was queued for the addressed client")
	}
}
