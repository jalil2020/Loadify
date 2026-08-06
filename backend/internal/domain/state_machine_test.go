package domain

import "testing"

// The happy path a load walks from creation to closure. If this sequence ever
// breaks, the whole marketplace is broken.
func TestLoadHappyPath(t *testing.T) {
	sequence := []LoadStatus{
		LoadDraft, LoadPublished, LoadReserved, LoadAssigned,
		LoadInTransit, LoadDelivered, LoadCompleted,
	}
	for i := 0; i < len(sequence)-1; i++ {
		from, to := sequence[i], sequence[i+1]
		if !from.CanTransitionTo(to) {
			t.Errorf("load cannot go %s → %s, but that is the happy path", from, to)
		}
	}
}

func TestLoadTerminalStatesAreFinal(t *testing.T) {
	for _, terminal := range []LoadStatus{LoadCompleted, LoadCancelled} {
		for _, target := range []LoadStatus{
			LoadDraft, LoadPublished, LoadReserved, LoadAssigned,
			LoadInTransit, LoadDelivered, LoadCompleted, LoadCancelled, LoadExpired,
		} {
			if terminal.CanTransitionTo(target) {
				t.Errorf("a %s load was allowed to move to %s", terminal, target)
			}
		}
	}
}

func TestLoadCannotSkipStages(t *testing.T) {
	forbidden := []struct{ from, to LoadStatus }{
		{LoadDraft, LoadAssigned},     // no driver can be assigned to a draft
		{LoadDraft, LoadInTransit},    // nothing is moving yet
		{LoadPublished, LoadDelivered},// delivery without a trip
		{LoadPublished, LoadCompleted},
		{LoadReserved, LoadInTransit}, // a reservation is not an agreement
		{LoadAssigned, LoadDelivered}, // must pass through in_transit
		{LoadInTransit, LoadPublished},// cargo on a truck cannot be re-listed
		{LoadDelivered, LoadCancelled},// delivered goods cannot be un-delivered
	}
	for _, tc := range forbidden {
		if tc.from.CanTransitionTo(tc.to) {
			t.Errorf("load was allowed to skip from %s to %s", tc.from, tc.to)
		}
	}
}

func TestLoadExpiredCanBeRelisted(t *testing.T) {
	// A shipper whose load timed out must be able to put it back without
	// re-entering everything.
	if !LoadExpired.CanTransitionTo(LoadPublished) {
		t.Error("an expired load cannot be republished")
	}
	if !LoadExpired.CanTransitionTo(LoadDraft) {
		t.Error("an expired load cannot be returned to draft")
	}
}

func TestLoadStatusPredicates(t *testing.T) {
	open := map[LoadStatus]bool{LoadPublished: true, LoadReserved: true}
	active := map[LoadStatus]bool{
		LoadPublished: true, LoadReserved: true, LoadAssigned: true, LoadInTransit: true,
	}
	all := []LoadStatus{
		LoadDraft, LoadPublished, LoadReserved, LoadAssigned, LoadInTransit,
		LoadDelivered, LoadCompleted, LoadCancelled, LoadExpired,
	}
	for _, s := range all {
		if s.IsOpen() != open[s] {
			t.Errorf("%s.IsOpen() = %v, want %v", s, s.IsOpen(), open[s])
		}
		if s.IsActive() != active[s] {
			t.Errorf("%s.IsActive() = %v, want %v", s, s.IsActive(), active[s])
		}
	}
}

// ─── trips ──────────────────────────────────────────────────────────

func TestTripHappyPath(t *testing.T) {
	sequence := []TripStatus{
		TripCreated, TripEnRoute, TripAtPickup, TripLoaded,
		TripInTransit, TripAtDelivery, TripDelivered, TripCompleted,
	}
	for i := 0; i < len(sequence)-1; i++ {
		from, to := sequence[i], sequence[i+1]
		if !from.CanTransitionTo(to) {
			t.Errorf("trip cannot go %s → %s, but that is the happy path", from, to)
		}
	}
}

// The core safety rule: once the cargo is on the truck, the trip cannot simply
// be cancelled — it must be delivered or disputed.
func TestTripCannotBeCancelledAfterLoading(t *testing.T) {
	cancellable := []TripStatus{TripCreated, TripEnRoute, TripAtPickup}
	for _, s := range cancellable {
		if !s.CanTransitionTo(TripCancelled) {
			t.Errorf("a trip in %s should still be cancellable", s)
		}
	}
	locked := []TripStatus{TripLoaded, TripInTransit, TripAtDelivery, TripDelivered}
	for _, s := range locked {
		if s.CanTransitionTo(TripCancelled) {
			t.Errorf("a trip in %s must not be cancellable — the cargo is loaded", s)
		}
	}
}

func TestTripDisputePaths(t *testing.T) {
	// A dispute is only meaningful once the cargo is in motion.
	disputable := []TripStatus{TripInTransit, TripAtDelivery, TripDelivered}
	for _, s := range disputable {
		if !s.CanTransitionTo(TripDisputed) {
			t.Errorf("a trip in %s should be disputable", s)
		}
	}
	notDisputable := []TripStatus{TripCreated, TripEnRoute, TripAtPickup, TripCompleted, TripCancelled}
	for _, s := range notDisputable {
		if s.CanTransitionTo(TripDisputed) {
			t.Errorf("a trip in %s must not be disputable", s)
		}
	}
	// An admin resolves a dispute either way.
	if !TripDisputed.CanTransitionTo(TripCompleted) || !TripDisputed.CanTransitionTo(TripCancelled) {
		t.Error("a disputed trip must be resolvable in both directions")
	}
}

func TestTripTerminalStatesAreFinal(t *testing.T) {
	all := []TripStatus{
		TripCreated, TripEnRoute, TripAtPickup, TripLoaded, TripInTransit,
		TripAtDelivery, TripDelivered, TripCompleted, TripCancelled, TripDisputed,
	}
	for _, terminal := range []TripStatus{TripCompleted, TripCancelled} {
		for _, target := range all {
			if terminal.CanTransitionTo(target) {
				t.Errorf("a %s trip was allowed to move to %s", terminal, target)
			}
		}
		if !terminal.IsFinished() {
			t.Errorf("%s should report IsFinished", terminal)
		}
	}
}

func TestTripDriverCannotCompleteOrCancel(t *testing.T) {
	// Completion means "the shipper accepted the delivery" and cancellation has
	// commercial consequences; neither is a driver's call.
	for _, s := range []TripStatus{TripCompleted, TripCancelled, TripDisputed, TripCreated} {
		if s.IsDriverAdvanceable() {
			t.Errorf("a driver must not be able to set status %s", s)
		}
	}
	for _, s := range []TripStatus{
		TripEnRoute, TripAtPickup, TripLoaded, TripInTransit, TripAtDelivery, TripDelivered,
	} {
		if !s.IsDriverAdvanceable() {
			t.Errorf("a driver should be able to set status %s", s)
		}
	}
}

func TestTripActivePredicate(t *testing.T) {
	active := map[TripStatus]bool{
		TripEnRoute: true, TripAtPickup: true, TripLoaded: true,
		TripInTransit: true, TripAtDelivery: true,
	}
	all := []TripStatus{
		TripCreated, TripEnRoute, TripAtPickup, TripLoaded, TripInTransit,
		TripAtDelivery, TripDelivered, TripCompleted, TripCancelled, TripDisputed,
	}
	for _, s := range all {
		if s.IsActive() != active[s] {
			t.Errorf("%s.IsActive() = %v, want %v", s, s.IsActive(), active[s])
		}
	}
}

func TestTimestampColumnForCoversEveryStampedStatus(t *testing.T) {
	expected := map[TripStatus]string{
		TripEnRoute:    "started_at",
		TripAtPickup:   "arrived_pickup_at",
		TripLoaded:     "loaded_at",
		TripAtDelivery: "arrived_delivery_at",
		TripDelivered:  "delivered_at",
		TripCompleted:  "completed_at",
		TripCancelled:  "cancelled_at",
	}
	for status, column := range expected {
		if got := TimestampColumnFor(status); got != column {
			t.Errorf("TimestampColumnFor(%s) = %q, want %q", status, got, column)
		}
	}
	// Statuses without a dedicated column must return empty, not a wrong column.
	for _, s := range []TripStatus{TripCreated, TripInTransit, TripDisputed} {
		if got := TimestampColumnFor(s); got != "" {
			t.Errorf("TimestampColumnFor(%s) = %q, want empty", s, got)
		}
	}
}

func TestLoadStatusForTrip(t *testing.T) {
	expected := map[TripStatus]LoadStatus{
		TripLoaded:    LoadInTransit,
		TripInTransit: LoadInTransit,
		TripDelivered: LoadDelivered,
		TripCompleted: LoadCompleted,
	}
	for tripStatus, want := range expected {
		if got := LoadStatusForTrip(tripStatus); got != want {
			t.Errorf("LoadStatusForTrip(%s) = %q, want %q", tripStatus, got, want)
		}
	}
	// Early trip stages must not disturb the load status.
	for _, s := range []TripStatus{TripCreated, TripEnRoute, TripAtPickup, TripAtDelivery} {
		if got := LoadStatusForTrip(s); got != "" {
			t.Errorf("LoadStatusForTrip(%s) = %q, want no change", s, got)
		}
	}
}

// Every load status a trip maps to must actually be reachable in the load state
// machine, or the trip flow would try an impossible transition at runtime.
func TestTripToLoadMappingsAreReachable(t *testing.T) {
	pairs := []struct {
		fromLoad LoadStatus
		trip     TripStatus
	}{
		{LoadAssigned, TripLoaded},
		{LoadAssigned, TripInTransit},
		{LoadInTransit, TripDelivered},
		{LoadDelivered, TripCompleted},
	}
	for _, tc := range pairs {
		target := LoadStatusForTrip(tc.trip)
		if target == "" {
			t.Fatalf("trip status %s maps to no load status", tc.trip)
		}
		if !tc.fromLoad.CanTransitionTo(target) {
			t.Errorf("trip %s wants load %s → %s, which the load machine forbids",
				tc.trip, tc.fromLoad, target)
		}
	}
}

// ─── bookings ───────────────────────────────────────────────────────

func TestBookingStatusPredicates(t *testing.T) {
	live := map[BookingStatus]bool{
		BookingPending: true, BookingOffered: true, BookingAccepted: true,
	}
	open := map[BookingStatus]bool{BookingPending: true, BookingOffered: true}
	all := []BookingStatus{
		BookingPending, BookingOffered, BookingAccepted, BookingRejected,
		BookingWithdrawn, BookingExpired, BookingCancelled,
	}
	for _, s := range all {
		if s.IsLive() != live[s] {
			t.Errorf("%s.IsLive() = %v, want %v", s, s.IsLive(), live[s])
		}
		if s.IsOpen() != open[s] {
			t.Errorf("%s.IsOpen() = %v, want %v", s, s.IsOpen(), open[s])
		}
	}
}

// LiveBookingStatuses must match IsLive exactly: the partial unique index in the
// schema is built from that slice, so a drift would silently allow double
// bookings.
func TestLiveBookingStatusesMatchPredicate(t *testing.T) {
	all := []BookingStatus{
		BookingPending, BookingOffered, BookingAccepted, BookingRejected,
		BookingWithdrawn, BookingExpired, BookingCancelled,
	}
	inSlice := make(map[BookingStatus]bool, len(LiveBookingStatuses))
	for _, s := range LiveBookingStatuses {
		inSlice[s] = true
	}
	for _, s := range all {
		if s.IsLive() != inSlice[s] {
			t.Errorf("%s: IsLive()=%v but LiveBookingStatuses membership=%v",
				s, s.IsLive(), inSlice[s])
		}
	}
}
