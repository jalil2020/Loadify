package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// SubscriptionStatus is the lifecycle state of a paid plan.
type SubscriptionStatus string

const (
	SubscriptionPending   SubscriptionStatus = "pending"
	SubscriptionTrialing  SubscriptionStatus = "trialing"
	SubscriptionActive    SubscriptionStatus = "active"
	SubscriptionExpired   SubscriptionStatus = "expired"
	SubscriptionCancelled SubscriptionStatus = "cancelled"
)

// IsUsable reports whether the plan currently grants its entitlements.
func (s SubscriptionStatus) IsUsable() bool {
	return s == SubscriptionActive || s == SubscriptionTrialing
}

// PaymentProvider is a checkout backend.
type PaymentProvider string

const (
	ProviderClick  PaymentProvider = "click"
	ProviderPayme  PaymentProvider = "payme"
	ProviderUzum   PaymentProvider = "uzum"
	ProviderStripe PaymentProvider = "stripe"
	ProviderWallet PaymentProvider = "wallet"
	ProviderManual PaymentProvider = "manual"
)

// PaymentPurpose is what an order pays for.
type PaymentPurpose string

const (
	PurposeSubscription PaymentPurpose = "subscription"
	PurposeEscrow       PaymentPurpose = "escrow"
	PurposeTopup        PaymentPurpose = "topup"
	PurposeOther        PaymentPurpose = "other"
)

// OrderStatus is the state of a checkout attempt.
type OrderStatus string

const (
	OrderPending    OrderStatus = "pending"
	OrderProcessing OrderStatus = "processing"
	OrderPaid       OrderStatus = "paid"
	OrderFailed     OrderStatus = "failed"
	OrderCancelled  OrderStatus = "cancelled"
	OrderRefunded   OrderStatus = "refunded"
)

// IsFinal reports whether the order can no longer change state.
func (s OrderStatus) IsFinal() bool {
	return s == OrderPaid || s == OrderFailed || s == OrderCancelled || s == OrderRefunded
}

// EscrowStatus is where held funds sit.
type EscrowStatus string

const (
	EscrowPending  EscrowStatus = "pending"
	EscrowFunded   EscrowStatus = "funded"
	EscrowReleased EscrowStatus = "released"
	EscrowRefunded EscrowStatus = "refunded"
	EscrowDisputed EscrowStatus = "disputed"
)

// LedgerDirection is the side of a double-entry pair.
type LedgerDirection string

const (
	Debit  LedgerDirection = "debit"
	Credit LedgerDirection = "credit"
)

// TransactionCategory classifies a ledger movement.
type TransactionCategory string

const (
	TxTopup         TransactionCategory = "topup"
	TxPayout        TransactionCategory = "payout"
	TxEscrowHold    TransactionCategory = "escrow_hold"
	TxEscrowRelease TransactionCategory = "escrow_release"
	TxEscrowRefund  TransactionCategory = "escrow_refund"
	TxCommission    TransactionCategory = "commission"
	TxSubscription  TransactionCategory = "subscription"
	TxRefund        TransactionCategory = "refund"
	TxAdjustment    TransactionCategory = "adjustment"
	TxPenalty       TransactionCategory = "penalty"
)

// WalletOwnerType is who a wallet belongs to.
type WalletOwnerType string

const (
	WalletOwnerUser     WalletOwnerType = "user"
	WalletOwnerCompany  WalletOwnerType = "company"
	WalletOwnerPlatform WalletOwnerType = "platform"
)

// SubscriptionPlan is a purchasable tier.
type SubscriptionPlan struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Code        string    `gorm:"size:32;not null;uniqueIndex"`
	NameUZ      string    `gorm:"size:100;not null"`
	NameRU      string    `gorm:"size:100;not null"`
	NameEN      string    `gorm:"size:100;not null"`
	Description *string
	TargetRole  string  `gorm:"size:16;not null"`
	PriceAmount int64   `gorm:"not null"`
	Currency    string  `gorm:"size:3;not null;default:'UZS'"`
	Period      string  `gorm:"size:8;not null"`
	TrialDays   int16   `gorm:"not null;default:0"`
	Features    JSONRaw `gorm:"type:jsonb;not null;default:'[]'"`
	Limits      JSONMap `gorm:"type:jsonb;not null;default:'{}'"`
	IsActive    bool    `gorm:"not null;default:true"`
	IsDefault   bool    `gorm:"not null;default:false"`
	SortOrder   int16   `gorm:"not null;default:0"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Duration is how long one billing period lasts.
//
// Months are added with AddDate rather than a fixed number of days, so a
// subscription bought on the 31st renews on a sensible date.
func (p *SubscriptionPlan) Duration(from time.Time) time.Time {
	if p.Period == "year" {
		return from.AddDate(1, 0, 0)
	}
	return from.AddDate(0, 1, 0)
}

// Limit reads a numeric entitlement. A missing key means unlimited, and a stored
// -1 means the same — both are reported as -1.
func (p *SubscriptionPlan) Limit(key string) int64 {
	raw, ok := p.Limits[key]
	if !ok {
		return -1
	}
	switch v := raw.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	default:
		return -1
	}
}

// Subscription is one purchased plan, owned by a user or a company.
type Subscription struct {
	ID             uuid.UUID          `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID         *uuid.UUID         `gorm:"type:uuid"`
	CompanyID      *uuid.UUID         `gorm:"type:uuid"`
	PlanID         uuid.UUID          `gorm:"type:uuid;not null"`
	Status         SubscriptionStatus `gorm:"size:16;not null;default:'pending'"`
	StartedAt      *time.Time
	ExpiresAt      *time.Time
	CancelledAt    *time.Time
	AutoRenew      bool       `gorm:"not null;default:false"`
	PaymentOrderID *uuid.UUID `gorm:"type:uuid"`
	CreatedAt      time.Time
	UpdatedAt      time.Time

	Plan *SubscriptionPlan `gorm:"foreignKey:PlanID"`
}

// IsUsable reports whether the subscription grants entitlements right now.
func (s *Subscription) IsUsable(now time.Time) bool {
	if !s.Status.IsUsable() {
		return false
	}
	return s.ExpiresAt == nil || s.ExpiresAt.After(now)
}

// Wallet holds a balance for a user, a company or the platform.
//
// The balance column is a cached projection of the ledger; wallet_transactions
// is the source of truth. Reconcile recomputes it.
type Wallet struct {
	ID          uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OwnerType   WalletOwnerType `gorm:"size:16;not null"`
	OwnerID     *uuid.UUID      `gorm:"type:uuid"`
	Currency    string          `gorm:"size:3;not null;default:'UZS'"`
	Balance     int64           `gorm:"not null;default:0"`
	HeldBalance int64           `gorm:"not null;default:0"`
	Status      string          `gorm:"size:16;not null;default:'active'"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Available is the balance that can actually be spent — held funds are committed
// to an escrow and are not the owner's to move.
func (w *Wallet) Available() int64 { return w.Balance - w.HeldBalance }

// IsUsable reports whether the wallet accepts movements.
func (w *Wallet) IsUsable() bool { return w.Status == "active" }

// WalletTransaction is one side of a double-entry pair.
type WalletTransaction struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	// GroupID ties the debit and credit of one movement together.
	GroupID        uuid.UUID           `gorm:"type:uuid;not null;index"`
	WalletID       uuid.UUID           `gorm:"type:uuid;not null;index"`
	Direction      LedgerDirection     `gorm:"size:8;not null"`
	Amount         int64               `gorm:"not null"`
	BalanceAfter   int64               `gorm:"not null"`
	Currency       string              `gorm:"size:3;not null;default:'UZS'"`
	Category       TransactionCategory `gorm:"size:24;not null"`
	ReferenceType  *string             `gorm:"size:32"`
	ReferenceID    *uuid.UUID          `gorm:"type:uuid"`
	Description    *string             `gorm:"size:255"`
	IdempotencyKey string              `gorm:"size:128;not null;uniqueIndex"`
	CreatedBy      *uuid.UUID          `gorm:"type:uuid"`
	CreatedAt      time.Time
}

// PaymentOrder is one checkout attempt against a provider.
type PaymentOrder struct {
	ID              uuid.UUID       `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID          *uuid.UUID      `gorm:"type:uuid"`
	CompanyID       *uuid.UUID      `gorm:"type:uuid"`
	Provider        PaymentProvider `gorm:"size:16;not null"`
	ProviderOrderID *string         `gorm:"size:128"`
	Purpose         PaymentPurpose  `gorm:"size:16;not null"`
	ReferenceType   *string         `gorm:"size:32"`
	ReferenceID     *uuid.UUID      `gorm:"type:uuid"`
	Amount          int64           `gorm:"not null"`
	Currency        string          `gorm:"size:3;not null;default:'UZS'"`
	Status          OrderStatus     `gorm:"size:16;not null;default:'pending'"`
	IdempotencyKey  string          `gorm:"size:128;not null;uniqueIndex"`
	CheckoutURL     *string         `gorm:"size:1024"`
	Payload         JSONMap         `gorm:"type:jsonb;not null;default:'{}'"`
	FailedReason    *string
	PaidAt          *time.Time
	ExpiresAt       *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Escrow holds a shipper's funds until delivery is confirmed.
type Escrow struct {
	ID               uuid.UUID    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	TripID           uuid.UUID    `gorm:"type:uuid;not null;uniqueIndex"`
	PayerWalletID    uuid.UUID    `gorm:"type:uuid;not null"`
	PayeeWalletID    uuid.UUID    `gorm:"type:uuid;not null"`
	Amount           int64        `gorm:"not null"`
	CommissionAmount int64        `gorm:"not null;default:0"`
	Currency         string       `gorm:"size:3;not null;default:'UZS'"`
	Status           EscrowStatus `gorm:"size:16;not null;default:'pending'"`
	FundedAt         *time.Time
	ReleasedAt       *time.Time
	RefundedAt       *time.Time
	ReleasedBy       *uuid.UUID `gorm:"type:uuid"`
	Notes            *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// PayeeAmount is what the driver receives after commission.
func (e *Escrow) PayeeAmount() int64 { return e.Amount - e.CommissionAmount }

// PaymentWebhook is a recorded provider callback, kept for replay safety and audit.
type PaymentWebhook struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Provider       string     `gorm:"size:16;not null"`
	EventID        *string    `gorm:"size:128"`
	EventType      *string    `gorm:"size:64"`
	OrderID        *uuid.UUID `gorm:"type:uuid"`
	SignatureValid bool       `gorm:"not null;default:false"`
	Payload        JSONMap    `gorm:"type:jsonb;not null"`
	ProcessedAt    *time.Time
	Error          *string
	CreatedAt      time.Time
}

// Invoice is a billing document.
type Invoice struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Number    string     `gorm:"size:32;not null"`
	CompanyID *uuid.UUID `gorm:"type:uuid"`
	UserID    *uuid.UUID `gorm:"type:uuid"`
	TripID    *uuid.UUID `gorm:"type:uuid"`
	Amount    int64      `gorm:"not null"`
	TaxAmount int64      `gorm:"not null;default:0"`
	Currency  string     `gorm:"size:3;not null;default:'UZS'"`
	Status    string     `gorm:"size:16;not null;default:'issued'"`
	Items     JSONRaw    `gorm:"type:jsonb;not null;default:'[]'"`
	FileID    *uuid.UUID `gorm:"type:uuid"`
	IssuedAt  time.Time
	DueAt     *time.Time
	PaidAt    *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Refund is a request to return funds, subject to admin approval.
type Refund struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	PaymentOrderID uuid.UUID  `gorm:"type:uuid;not null;index"`
	Amount         int64      `gorm:"not null"`
	Currency       string     `gorm:"size:3;not null;default:'UZS'"`
	Reason         string     `gorm:"not null"`
	Status         string     `gorm:"size:16;not null;default:'pending'"`
	RequestedBy    *uuid.UUID `gorm:"type:uuid"`
	ApprovedBy     *uuid.UUID `gorm:"type:uuid"`
	ProcessedAt    *time.Time
	ProviderRef    *string `gorm:"size:128"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ─── ports ──────────────────────────────────────────────────────────

// LedgerMovement describes one balance transfer between two wallets.
//
// Every movement is symmetric: the same amount leaves one wallet and enters
// another, so the ledger always sums to zero across all wallets.
type LedgerMovement struct {
	FromWalletID   uuid.UUID
	ToWalletID     uuid.UUID
	Amount         int64
	Currency       string
	Category       TransactionCategory
	ReferenceType  string
	ReferenceID    *uuid.UUID
	Description    string
	IdempotencyKey string
	CreatedBy      *uuid.UUID
	// AllowNegativeFrom permits the source wallet to go below zero. Only the
	// platform wallet is allowed this, because commission is credited to it
	// before any settlement has funded it.
	AllowNegativeFrom bool
}

// WalletRepository persists wallets and the double-entry ledger.
type WalletRepository interface {
	// EnsureWallet returns the owner's wallet, creating it on first use.
	EnsureWallet(ctx context.Context, ownerType WalletOwnerType, ownerID *uuid.UUID, currency string) (*Wallet, error)
	FindByID(ctx context.Context, id uuid.UUID) (*Wallet, error)
	FindByOwner(ctx context.Context, ownerType WalletOwnerType, ownerID uuid.UUID, currency string) (*Wallet, error)
	PlatformWallet(ctx context.Context, currency string) (*Wallet, error)
	SetStatus(ctx context.Context, walletID uuid.UUID, status string) error

	// Transfer writes both legs of a movement and updates both balances inside
	// one transaction, locking the wallet rows in a deterministic order.
	// Repeating the same idempotency key is a no-op.
	Transfer(ctx context.Context, movement LedgerMovement) error
	// Hold moves funds into the held portion of a wallet without changing the
	// balance: the money is still the payer's, but no longer spendable.
	Hold(ctx context.Context, walletID uuid.UUID, amount int64, idempotencyKey string) error
	// ReleaseHold undoes a hold without transferring anything.
	ReleaseHold(ctx context.Context, walletID uuid.UUID, amount int64, idempotencyKey string) error

	ListTransactions(ctx context.Context, walletID uuid.UUID, offset, limit int) ([]WalletTransaction, int64, error)
	FindTransactionByKey(ctx context.Context, idempotencyKey string) (*WalletTransaction, error)
	// Reconcile recomputes a cached balance from the ledger and reports the drift.
	Reconcile(ctx context.Context, walletID uuid.UUID) (drift int64, err error)
}

// SubscriptionRepository persists plans and subscriptions.
type SubscriptionRepository interface {
	ListPlans(ctx context.Context, targetRole string) ([]SubscriptionPlan, error)
	FindPlanByID(ctx context.Context, id uuid.UUID) (*SubscriptionPlan, error)
	FindPlanByCode(ctx context.Context, code string) (*SubscriptionPlan, error)
	DefaultPlan(ctx context.Context, targetRole string) (*SubscriptionPlan, error)

	Create(ctx context.Context, subscription *Subscription) error
	FindByID(ctx context.Context, id uuid.UUID) (*Subscription, error)
	// ActiveForUser and ActiveForCompany return nil rather than an error when
	// there is no paid plan: the free tier is the normal case.
	ActiveForUser(ctx context.Context, userID uuid.UUID) (*Subscription, error)
	ActiveForCompany(ctx context.Context, companyID uuid.UUID) (*Subscription, error)
	UpdateColumns(ctx context.Context, id uuid.UUID, columns map[string]any) error
	ListHistoryForUser(ctx context.Context, userID uuid.UUID, offset, limit int) ([]Subscription, int64, error)
	ListHistoryForCompany(ctx context.Context, companyID uuid.UUID, offset, limit int) ([]Subscription, int64, error)
	ExpireOverdue(ctx context.Context, now time.Time) (int64, error)
	ListRenewable(ctx context.Context, before time.Time, limit int) ([]Subscription, error)
}

// PaymentRepository persists orders, webhooks, escrow, invoices and refunds.
type PaymentRepository interface {
	CreateOrder(ctx context.Context, order *PaymentOrder) error
	FindOrderByID(ctx context.Context, id uuid.UUID) (*PaymentOrder, error)
	FindOrderByKey(ctx context.Context, idempotencyKey string) (*PaymentOrder, error)
	FindOrderByProviderRef(ctx context.Context, provider PaymentProvider, providerOrderID string) (*PaymentOrder, error)
	UpdateOrderColumns(ctx context.Context, id uuid.UUID, columns map[string]any) error
	// TransitionOrder guards on the current status so a duplicate webhook cannot
	// mark an order paid twice.
	TransitionOrder(ctx context.Context, id uuid.UUID, from []OrderStatus, to OrderStatus, columns map[string]any) error
	ListOrders(ctx context.Context, userID, companyID *uuid.UUID, offset, limit int) ([]PaymentOrder, int64, error)
	ExpireStaleOrders(ctx context.Context, before time.Time) (int64, error)

	RecordWebhook(ctx context.Context, hook *PaymentWebhook) error
	// FindWebhookByEvent detects a replayed callback.
	FindWebhookByEvent(ctx context.Context, provider, eventID string) (*PaymentWebhook, error)
	MarkWebhookProcessed(ctx context.Context, id uuid.UUID, processErr *string) error

	CreateEscrow(ctx context.Context, escrow *Escrow) error
	FindEscrowByTrip(ctx context.Context, tripID uuid.UUID) (*Escrow, error)
	TransitionEscrow(ctx context.Context, id uuid.UUID, from []EscrowStatus, to EscrowStatus, columns map[string]any) error

	CreateInvoice(ctx context.Context, invoice *Invoice) error
	ListInvoices(ctx context.Context, companyID, userID *uuid.UUID, offset, limit int) ([]Invoice, int64, error)
	FindInvoiceByID(ctx context.Context, id uuid.UUID) (*Invoice, error)

	CreateRefund(ctx context.Context, refund *Refund) error
	FindRefundByID(ctx context.Context, id uuid.UUID) (*Refund, error)
	ListRefunds(ctx context.Context, status string, offset, limit int) ([]Refund, int64, error)
	UpdateRefundColumns(ctx context.Context, id uuid.UUID, columns map[string]any) error
}

// CheckoutRequest is what a provider adapter needs to start a payment.
type CheckoutRequest struct {
	OrderID     uuid.UUID
	Amount      int64
	Currency    string
	Description string
	ReturnURL   string
}

// CheckoutSession is what the adapter hands back.
type CheckoutSession struct {
	ProviderOrderID string
	CheckoutURL     string
	Payload         map[string]any
}

// WebhookResult is a verified provider callback, normalised across providers.
type WebhookResult struct {
	EventID         string
	EventType       string
	ProviderOrderID string
	OrderID         *uuid.UUID
	Amount          int64
	Paid            bool
	Cancelled       bool
	FailureReason   string
	SignatureValid  bool
	Raw             map[string]any
}

// PaymentGateway is one checkout provider.
//
// Each provider has its own signature scheme and callback shape; the adapter
// normalises both so the usecase handles one flow rather than three.
type PaymentGateway interface {
	Provider() PaymentProvider
	CreateCheckout(ctx context.Context, req CheckoutRequest) (*CheckoutSession, error)
	// ParseWebhook verifies the signature and normalises the callback. It must
	// return SignatureValid=false rather than an error for a bad signature, so
	// the attempt is still recorded.
	ParseWebhook(ctx context.Context, headers map[string]string, body []byte) (*WebhookResult, error)
	// WebhookResponse renders the acknowledgement shape the provider expects.
	WebhookResponse(result *WebhookResult, processErr error) any
}
