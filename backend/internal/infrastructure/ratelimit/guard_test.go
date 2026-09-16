package ratelimit_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/infrastructure/ratelimit"
)

// The limits are deliberately small here so a test can reach them; the shape of
// the rules is what is under test, not the numbers a deployment ships with.
func testLimits() config.TelegramConfig {
	return config.TelegramConfig{
		AccountGap:      2 * time.Second,
		AccountHourly:   3,
		AccountDaily:    5,
		GroupCooldown:   time.Hour,
		GroupDaily:      2,
		DuplicateWindow: 24 * time.Hour,
		GlobalPerSecond: 100,
	}
}

func newGuard(t *testing.T) (*ratelimit.Guard, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return ratelimit.NewGuard(client, testLimits()), server
}

func target() domain.GuardTarget {
	return domain.GuardTarget{
		CompanyID:   uuid.New(),
		UserID:      uuid.New(),
		AccountID:   uuid.New(),
		GroupID:     uuid.New(),
		PeerID:      -1001234567890,
		Fingerprint: "b5f1c0de",
		Cooldown:    30 * time.Minute,
		GroupDaily:  2,
	}
}

func reserve(t *testing.T, guard *ratelimit.Guard, tgt domain.GuardTarget) domain.GuardDecision {
	t.Helper()
	decision, err := guard.Reserve(context.Background(), tgt, time.Now())
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	return decision
}

func TestFirstPostIsAllowed(t *testing.T) {
	guard, _ := newGuard(t)

	if decision := reserve(t, guard, target()); !decision.Allowed {
		t.Errorf("first post refused: %+v", decision)
	}
}

// The same advert in the same group is the thing the duplicate check exists
// for, and it holds across companies because the group sees one message.
func TestTheSameAdvertTwiceInOneGroup(t *testing.T) {
	guard, _ := newGuard(t)
	first := target()

	reserve(t, guard, first)

	second := target()
	second.PeerID = first.PeerID
	second.Fingerprint = first.Fingerprint
	decision := reserve(t, guard, second)

	if decision.Allowed {
		t.Fatal("the same advert went to the same group twice")
	}
	if decision.Reason != domain.SkipDuplicate {
		t.Errorf("reason = %q, want duplicate", decision.Reason)
	}
	if decision.RetryAfter <= 0 {
		t.Error("a refusal that is a wait must say how long")
	}
}

// A different advert to the same group is not a duplicate, but it still has to
// wait out the cooldown.
func TestCooldownHoldsTheGroup(t *testing.T) {
	guard, _ := newGuard(t)
	first := target()
	reserve(t, guard, first)

	second := first
	second.Fingerprint = "another-advert"
	decision := reserve(t, guard, second)

	if decision.Allowed {
		t.Fatal("two posts went into one group back to back")
	}
	if decision.Reason != domain.SkipCooldown {
		t.Errorf("reason = %q, want cooldown", decision.Reason)
	}
	if decision.RetryAfter > 30*time.Minute || decision.RetryAfter < 29*time.Minute {
		t.Errorf("retry after = %v, want roughly the cooldown", decision.RetryAfter)
	}
}

// Without a cooldown the daily ceiling is what stops a group being flooded.
func TestGroupDailyCeiling(t *testing.T) {
	guard, server := newGuard(t)
	tgt := target()
	tgt.Cooldown = 0

	for i := range 2 {
		tgt.Fingerprint = uuid.NewString()
		if decision := reserve(t, guard, tgt); !decision.Allowed {
			t.Fatalf("post %d refused: %+v", i+1, decision)
		}
		server.FastForward(3 * time.Second) // clear the account gap
	}

	tgt.Fingerprint = uuid.NewString()
	decision := reserve(t, guard, tgt)
	if decision.Allowed {
		t.Fatal("the group took a third post on a limit of two")
	}
	if decision.Reason != domain.SkipGroupLimit {
		t.Errorf("reason = %q, want the group limit", decision.Reason)
	}
}

// Spacing is per account, so a second group does not get an instant post just
// because it is a different group.
func TestAccountGapSpacesEveryGroup(t *testing.T) {
	guard, server := newGuard(t)
	first := target()
	reserve(t, guard, first)

	second := first
	second.GroupID = uuid.New()
	second.PeerID = -1009876543210
	second.Fingerprint = uuid.NewString()

	decision := reserve(t, guard, second)
	if decision.Allowed {
		t.Fatal("two groups were posted to in the same instant from one account")
	}
	if decision.Reason != domain.SkipAccountLimit {
		t.Errorf("reason = %q, want the account limit", decision.Reason)
	}

	server.FastForward(3 * time.Second)
	if decision := reserve(t, guard, second); !decision.Allowed {
		t.Errorf("still refused after the gap elapsed: %+v", decision)
	}
}

func TestAccountHourlyCeiling(t *testing.T) {
	guard, server := newGuard(t)
	base := target()
	base.Cooldown = 0

	for i := range 3 {
		tgt := base
		tgt.GroupID = uuid.New()
		tgt.PeerID = int64(-100100 - i)
		tgt.Fingerprint = uuid.NewString()
		if decision := reserve(t, guard, tgt); !decision.Allowed {
			t.Fatalf("post %d refused: %+v", i+1, decision)
		}
		server.FastForward(3 * time.Second)
	}

	tgt := base
	tgt.GroupID = uuid.New()
	tgt.PeerID = -100999
	tgt.Fingerprint = uuid.NewString()
	if decision := reserve(t, guard, tgt); decision.Allowed {
		t.Fatal("the account sent a fourth post on an hourly limit of three")
	}
}

// A pause is Telegram asking for a wait. Nothing from that account goes out
// while it stands, whatever the counters say.
func TestAPausedAccountSendsNothing(t *testing.T) {
	guard, _ := newGuard(t)
	tgt := target()

	if err := guard.PauseAccount(context.Background(), tgt.AccountID, time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("PauseAccount: %v", err)
	}

	until, paused, err := guard.AccountPausedUntil(context.Background(), tgt.AccountID)
	if err != nil {
		t.Fatalf("AccountPausedUntil: %v", err)
	}
	if !paused || time.Until(until) <= 0 {
		t.Fatalf("pause not recorded: %v %v", until, paused)
	}

	decision := reserve(t, guard, tgt)
	if decision.Allowed {
		t.Fatal("a paused account was allowed to post")
	}
	if decision.Reason != domain.SkipAccountLimit {
		t.Errorf("reason = %q", decision.Reason)
	}
}

// Telegram refusing a message means it never arrived, so the allowance it
// consumed goes back.
func TestReleaseGivesTheAllowanceBack(t *testing.T) {
	guard, _ := newGuard(t)
	tgt := target()
	reserve(t, guard, tgt)

	if err := guard.Release(context.Background(), tgt); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if decision := reserve(t, guard, tgt); !decision.Allowed {
		t.Errorf("the same advert is still blocked after a release: %+v", decision)
	}
}

// Plan explains without consuming: a preview a person looks at twice must not
// use up what it is describing.
func TestPlanDoesNotConsume(t *testing.T) {
	guard, _ := newGuard(t)
	tgt := target()

	for range 3 {
		decisions, err := guard.Plan(context.Background(), []domain.GuardTarget{tgt}, time.Now())
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}
		if len(decisions) != 1 || !decisions[0].Allowed {
			t.Fatalf("plan refused a fresh target: %+v", decisions)
		}
	}
	if decision := reserve(t, guard, tgt); !decision.Allowed {
		t.Error("planning consumed the allowance it was only meant to report on")
	}
}

// The guard is only worth having if two workers cannot both pass it.
func TestConcurrentReservationsRespectTheCeiling(t *testing.T) {
	guard, _ := newGuard(t)
	base := target()
	base.Cooldown = 0
	base.GroupDaily = 2

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		allowed int
	)
	for range 20 {
		wg.Go(func() {
			tgt := base
			tgt.Fingerprint = uuid.NewString()
			decision, err := guard.Reserve(context.Background(), tgt, time.Now())
			if err != nil {
				t.Errorf("Reserve: %v", err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if decision.Allowed {
				allowed++
			}
		})
	}
	wg.Wait()

	// The account gap alone caps this at one; what matters is that twenty
	// simultaneous attempts cannot exceed the ceiling between them.
	if allowed > 1 {
		t.Errorf("%d of 20 concurrent attempts were allowed, want at most 1", allowed)
	}
	if allowed == 0 {
		t.Error("every attempt was refused, so the guard blocks even the first post")
	}
}
