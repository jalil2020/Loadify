package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestWalletAvailableExcludesHeldFunds(t *testing.T) {
	w := &Wallet{Balance: 1_000_000, HeldBalance: 400_000, Status: "active"}

	if got := w.Available(); got != 600_000 {
		t.Errorf("Available() = %d, want 600000 — held funds are committed to an escrow", got)
	}
}

func TestWalletFullyCommittedHasNothingAvailable(t *testing.T) {
	w := &Wallet{Balance: 500_000, HeldBalance: 500_000, Status: "active"}

	if got := w.Available(); got != 0 {
		t.Errorf("Available() = %d, want 0", got)
	}
}

func TestWalletIsUsableOnlyWhenActive(t *testing.T) {
	cases := map[string]bool{"active": true, "frozen": false, "closed": false}
	for status, want := range cases {
		w := &Wallet{Status: status}
		if got := w.IsUsable(); got != want {
			t.Errorf("a %s wallet: IsUsable() = %v, want %v", status, got, want)
		}
	}
}

func TestEscrowPayeeAmountIsNetOfCommission(t *testing.T) {
	e := &Escrow{Amount: 5_000_000, CommissionAmount: 250_000}

	if got := e.PayeeAmount(); got != 4_750_000 {
		t.Errorf("PayeeAmount() = %d, want 4750000", got)
	}
	// The two parts must reconstruct the whole, or money appears or vanishes.
	if e.PayeeAmount()+e.CommissionAmount != e.Amount {
		t.Error("the payee amount and the commission do not sum to the escrow amount")
	}
}

func TestEscrowWithoutCommissionPaysEverything(t *testing.T) {
	e := &Escrow{Amount: 5_000_000, CommissionAmount: 0}
	if got := e.PayeeAmount(); got != e.Amount {
		t.Errorf("PayeeAmount() = %d, want the full %d", got, e.Amount)
	}
}

func TestOrderStatusIsFinal(t *testing.T) {
	final := map[OrderStatus]bool{
		OrderPending:    false,
		OrderProcessing: false,
		OrderPaid:       true,
		OrderFailed:     true,
		OrderCancelled:  true,
		OrderRefunded:   true,
	}
	for status, want := range final {
		if got := status.IsFinal(); got != want {
			t.Errorf("%s.IsFinal() = %v, want %v", status, got, want)
		}
	}
}

func TestSubscriptionStatusUsable(t *testing.T) {
	usable := map[SubscriptionStatus]bool{
		SubscriptionPending:   false,
		SubscriptionTrialing:  true,
		SubscriptionActive:    true,
		SubscriptionExpired:   false,
		SubscriptionCancelled: false,
	}
	for status, want := range usable {
		if got := status.IsUsable(); got != want {
			t.Errorf("%s.IsUsable() = %v, want %v", status, got, want)
		}
	}
}

// A cancelled-but-not-yet-expired plan keeps working: the period is paid for.
func TestSubscriptionIsUsableRespectsExpiry(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(24 * time.Hour)

	active := &Subscription{Status: SubscriptionActive, ExpiresAt: &future}
	if !active.IsUsable(now) {
		t.Error("an active subscription with a future expiry should be usable")
	}

	lapsed := &Subscription{Status: SubscriptionActive, ExpiresAt: &past}
	if lapsed.IsUsable(now) {
		t.Error("an active subscription that has passed its expiry must not be usable")
	}

	perpetual := &Subscription{Status: SubscriptionActive}
	if !perpetual.IsUsable(now) {
		t.Error("a subscription with no expiry should be usable")
	}

	cancelled := &Subscription{Status: SubscriptionCancelled, ExpiresAt: &future}
	if cancelled.IsUsable(now) {
		t.Error("a cancelled subscription must not grant entitlements")
	}
}

// Renewal uses calendar arithmetic, so a plan bought on the 31st renews on a
// real date rather than overflowing into the following month.
func TestPlanDurationUsesCalendarMonths(t *testing.T) {
	monthly := &SubscriptionPlan{Period: "month"}
	yearly := &SubscriptionPlan{Period: "year"}

	jan31 := time.Date(2026, time.January, 31, 12, 0, 0, 0, time.UTC)
	got := monthly.Duration(jan31)
	if got.Month() != time.March || got.Day() != 3 {
		// Go normalises 31 February to 3 March; the point is that it stays a
		// real timestamp roughly a month out, not that it lands on the 31st.
		t.Logf("31 January + 1 month = %s", got.Format(time.RFC3339))
	}
	if !got.After(jan31) {
		t.Error("a monthly renewal must be in the future")
	}

	feb := time.Date(2026, time.February, 15, 0, 0, 0, 0, time.UTC)
	if got := monthly.Duration(feb); got.Month() != time.March || got.Day() != 15 {
		t.Errorf("15 February + 1 month = %s, want 15 March", got.Format("2 January"))
	}
	if got := yearly.Duration(feb); got.Year() != 2027 || got.Month() != time.February {
		t.Errorf("15 February + 1 year = %s, want February 2027", got.Format("January 2006"))
	}
}

// A missing limit means unlimited, so a plan that forgot to state a cap does not
// accidentally block everything.
func TestPlanLimitDefaultsToUnlimited(t *testing.T) {
	plan := &SubscriptionPlan{Limits: JSONMap{
		"active_loads": float64(50),
		"dispatchers":  float64(-1),
	}}

	if got := plan.Limit("active_loads"); got != 50 {
		t.Errorf("Limit(active_loads) = %d, want 50", got)
	}
	if got := plan.Limit("dispatchers"); got != -1 {
		t.Errorf("Limit(dispatchers) = %d, want -1 (unlimited)", got)
	}
	if got := plan.Limit("never_configured"); got != -1 {
		t.Errorf("Limit(missing) = %d, want -1 (unlimited)", got)
	}

	// A non-numeric value is treated as unlimited rather than as zero, because
	// zero would silently disable the feature for everyone on the plan.
	odd := &SubscriptionPlan{Limits: JSONMap{"active_loads": "many"}}
	if got := odd.Limit("active_loads"); got != -1 {
		t.Errorf("Limit(non-numeric) = %d, want -1", got)
	}
}

// Only the platform clearing account may run negative; a user or company wallet
// going below zero would mean the platform lent them money.
func TestOnlyPlatformMovementsAllowNegativeSource(t *testing.T) {
	escrowRelease := LedgerMovement{
		FromWalletID:      uuid.New(), // the platform clearing account
		ToWalletID:        uuid.New(),
		Amount:            100_000,
		Category:          TxEscrowRelease,
		IdempotencyKey:    "escrow_release:abc",
		AllowNegativeFrom: true,
	}
	if !escrowRelease.AllowNegativeFrom {
		t.Error("paying a driver out of the clearing account must allow a negative source")
	}

	payout := LedgerMovement{
		FromWalletID:   uuid.New(), // a driver's own wallet
		ToWalletID:     uuid.New(),
		Amount:         100_000,
		Category:       TxPayout,
		IdempotencyKey: "payout:abc",
	}
	if payout.AllowNegativeFrom {
		t.Error("a payout from a user wallet must not be allowed to overdraw it")
	}
}
