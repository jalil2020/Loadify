package dto

import (
	"time"

	"github.com/google/uuid"
)

// ─── subscriptions ──────────────────────────────────────────────────

// SubscribeRequest starts a checkout for a plan.
type SubscribeRequest struct {
	PlanID    uuid.UUID `json:"plan_id" validate:"required"`
	Provider  string    `json:"provider" validate:"required,oneof=click payme uzum wallet"`
	AutoRenew bool      `json:"auto_renew"`
	ReturnURL string    `json:"return_url" validate:"omitempty,url,max=512"`
}

// PlanResponse is one purchasable tier.
type PlanResponse struct {
	ID          uuid.UUID      `json:"id"`
	Code        string         `json:"code"`
	Name        string         `json:"name"`
	Description *string        `json:"description"`
	TargetRole  string         `json:"target_role"`
	PriceAmount int64          `json:"price_amount"`
	Currency    string         `json:"currency"`
	Period      string         `json:"period"`
	TrialDays   int16          `json:"trial_days"`
	Features    []string       `json:"features"`
	Limits      map[string]any `json:"limits"`
	IsDefault   bool           `json:"is_default"`
	// IsCurrent marks the plan the caller is already on.
	IsCurrent bool `json:"is_current"`
}

// SubscriptionResponse is the caller's current plan.
type SubscriptionResponse struct {
	ID          uuid.UUID     `json:"id"`
	Plan        *PlanResponse `json:"plan"`
	Status      string        `json:"status"`
	StartedAt   *time.Time    `json:"started_at"`
	ExpiresAt   *time.Time    `json:"expires_at"`
	CancelledAt *time.Time    `json:"cancelled_at"`
	AutoRenew   bool          `json:"auto_renew"`
	DaysLeft    *int          `json:"days_left,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
}

// EntitlementsResponse tells a client what the caller's plan allows, so the UI
// can hide what they cannot use instead of failing on submit.
type EntitlementsResponse struct {
	PlanCode string         `json:"plan_code"`
	PlanName string         `json:"plan_name"`
	IsPaid   bool           `json:"is_paid"`
	Limits   map[string]any `json:"limits"`
	Usage    map[string]any `json:"usage"`
}

// ─── checkout & orders ──────────────────────────────────────────────

// CheckoutResponse is what the client needs to complete a payment.
type CheckoutResponse struct {
	OrderID     uuid.UUID `json:"order_id"`
	Provider    string    `json:"provider"`
	Amount      int64     `json:"amount"`
	Currency    string    `json:"currency"`
	Status      string    `json:"status"`
	CheckoutURL string    `json:"checkout_url,omitempty"`
	// Paid is true when the order settled immediately, which happens for a
	// wallet payment with sufficient balance.
	Paid bool `json:"paid"`
}

// OrderResponse is one checkout attempt.
type OrderResponse struct {
	ID           uuid.UUID  `json:"id"`
	Provider     string     `json:"provider"`
	Purpose      string     `json:"purpose"`
	Amount       int64      `json:"amount"`
	Currency     string     `json:"currency"`
	Status       string     `json:"status"`
	FailedReason *string    `json:"failed_reason"`
	PaidAt       *time.Time `json:"paid_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

// ─── wallet ─────────────────────────────────────────────────────────

// TopupRequest adds funds to the caller's wallet.
type TopupRequest struct {
	Amount    int64  `json:"amount" validate:"required,gt=0"`
	Provider  string `json:"provider" validate:"required,oneof=click payme uzum"`
	ReturnURL string `json:"return_url" validate:"omitempty,url,max=512"`
}

// PayoutRequest withdraws funds from the caller's wallet.
type PayoutRequest struct {
	Amount      int64  `json:"amount" validate:"required,gt=0"`
	CardNumber  string `json:"card_number" validate:"required,min=16,max=19"`
	CardHolder  string `json:"card_holder" validate:"required,max=120"`
	Description string `json:"description" validate:"omitempty,max=255"`
}

// WalletResponse is a balance summary.
type WalletResponse struct {
	ID          uuid.UUID `json:"id"`
	Currency    string    `json:"currency"`
	Balance     int64     `json:"balance"`
	HeldBalance int64     `json:"held_balance"`
	Available   int64     `json:"available"`
	Status      string    `json:"status"`
}

// TransactionResponse is one ledger entry as the owner sees it.
type TransactionResponse struct {
	ID            uuid.UUID  `json:"id"`
	Direction     string     `json:"direction"`
	Amount        int64      `json:"amount"`
	BalanceAfter  int64      `json:"balance_after"`
	Currency      string     `json:"currency"`
	Category      string     `json:"category"`
	Description   *string    `json:"description"`
	ReferenceType *string    `json:"reference_type"`
	ReferenceID   *uuid.UUID `json:"reference_id"`
	CreatedAt     time.Time  `json:"created_at"`
}

// ─── escrow ─────────────────────────────────────────────────────────

// FundEscrowRequest moves a trip's price into escrow.
type FundEscrowRequest struct {
	TripID uuid.UUID `json:"trip_id" validate:"required"`
}

// EscrowResponse is the state of a trip's held funds.
type EscrowResponse struct {
	ID               uuid.UUID  `json:"id"`
	TripID           uuid.UUID  `json:"trip_id"`
	Amount           int64      `json:"amount"`
	CommissionAmount int64      `json:"commission_amount"`
	PayeeAmount      int64      `json:"payee_amount"`
	Currency         string     `json:"currency"`
	Status           string     `json:"status"`
	FundedAt         *time.Time `json:"funded_at"`
	ReleasedAt       *time.Time `json:"released_at"`
	RefundedAt       *time.Time `json:"refunded_at"`
}

// ─── refunds & invoices ─────────────────────────────────────────────

// RequestRefundRequest asks for money back on an order.
type RequestRefundRequest struct {
	OrderID uuid.UUID `json:"order_id" validate:"required"`
	Amount  int64     `json:"amount" validate:"omitempty,gt=0"`
	Reason  string    `json:"reason" validate:"required,min=3,max=1000"`
}

// ReviewRefundRequest is the admin decision on a refund.
type ReviewRefundRequest struct {
	Approve bool   `json:"approve"`
	Note    string `json:"note" validate:"omitempty,max=500"`
}

// RefundResponse is one refund request.
type RefundResponse struct {
	ID          uuid.UUID  `json:"id"`
	OrderID     uuid.UUID  `json:"order_id"`
	Amount      int64      `json:"amount"`
	Currency    string     `json:"currency"`
	Reason      string     `json:"reason"`
	Status      string     `json:"status"`
	ProcessedAt *time.Time `json:"processed_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// InvoiceResponse is a billing document.
type InvoiceResponse struct {
	ID        uuid.UUID  `json:"id"`
	Number    string     `json:"number"`
	Amount    int64      `json:"amount"`
	TaxAmount int64      `json:"tax_amount"`
	Currency  string     `json:"currency"`
	Status    string     `json:"status"`
	FileURL   string     `json:"file_url,omitempty"`
	IssuedAt  time.Time  `json:"issued_at"`
	DueAt     *time.Time `json:"due_at"`
	PaidAt    *time.Time `json:"paid_at"`
}

// PaymentMethodsResponse lists the providers a client may offer.
type PaymentMethodsResponse struct {
	Providers []string `json:"providers"`
}
