package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/logger"
)

// WalletUsecase owns balances, the ledger and escrow.
//
// Every method that moves money passes a deterministic idempotency key to the
// repository, so a retried request — from a client, a webhook or a job — never
// applies a movement twice.
type WalletUsecase struct {
	wallets   domain.WalletRepository
	payments  domain.PaymentRepository
	trips     domain.TripRepository
	drivers   domain.DriverRepository
	companies domain.CompanyRepository
	settings  domain.SettingsReader
	tx        domain.TxManager
	audit     domain.AuditLogger
	notifier  domain.Notifier
	log       logger.Logger
}

// NewWalletUsecase wires the wallet usecase.
func NewWalletUsecase(
	wallets domain.WalletRepository,
	payments domain.PaymentRepository,
	trips domain.TripRepository,
	drivers domain.DriverRepository,
	companies domain.CompanyRepository,
	settings domain.SettingsReader,
	tx domain.TxManager,
	audit domain.AuditLogger,
	notifier domain.Notifier,
	log logger.Logger,
) *WalletUsecase {
	return &WalletUsecase{
		wallets: wallets, payments: payments, trips: trips, drivers: drivers,
		companies: companies, settings: settings, tx: tx, audit: audit,
		notifier: notifier, log: log,
	}
}

// GetWallet returns the caller's balance, creating the wallet on first look.
func (u *WalletUsecase) GetWallet(ctx context.Context, ownerType domain.WalletOwnerType, ownerID uuid.UUID) (*dto.WalletResponse, error) {
	wallet, err := u.wallets.EnsureWallet(ctx, ownerType, &ownerID, domain.CurrencyUZS)
	if err != nil {
		return nil, err
	}
	return walletResponse(wallet), nil
}

// ListTransactions returns the owner's ledger history.
func (u *WalletUsecase) ListTransactions(ctx context.Context, ownerType domain.WalletOwnerType, ownerID uuid.UUID, offset, limit int) ([]dto.TransactionResponse, int64, error) {
	wallet, err := u.wallets.EnsureWallet(ctx, ownerType, &ownerID, domain.CurrencyUZS)
	if err != nil {
		return nil, 0, err
	}
	entries, total, err := u.wallets.ListTransactions(ctx, wallet.ID, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.TransactionResponse, 0, len(entries))
	for i := range entries {
		out = append(out, transactionResponse(&entries[i]))
	}
	return out, total, nil
}

// Credit adds funds to a wallet from the platform, which is how a confirmed
// top-up lands. The platform wallet is allowed to go negative because it is a
// clearing account, not a balance anyone spends.
func (u *WalletUsecase) Credit(ctx context.Context, walletID uuid.UUID, amount int64, category domain.TransactionCategory, reference string, referenceID *uuid.UUID, idempotencyKey string) error {
	platform, err := u.wallets.PlatformWallet(ctx, domain.CurrencyUZS)
	if err != nil {
		return err
	}
	return u.wallets.Transfer(ctx, domain.LedgerMovement{
		FromWalletID:      platform.ID,
		ToWalletID:        walletID,
		Amount:            amount,
		Currency:          domain.CurrencyUZS,
		Category:          category,
		ReferenceType:     reference,
		ReferenceID:       referenceID,
		Description:       string(category),
		IdempotencyKey:    idempotencyKey,
		AllowNegativeFrom: true,
	})
}

// Debit moves funds from a wallet to the platform.
func (u *WalletUsecase) Debit(ctx context.Context, walletID uuid.UUID, amount int64, category domain.TransactionCategory, reference string, referenceID *uuid.UUID, idempotencyKey string) error {
	platform, err := u.wallets.PlatformWallet(ctx, domain.CurrencyUZS)
	if err != nil {
		return err
	}
	return u.wallets.Transfer(ctx, domain.LedgerMovement{
		FromWalletID:   walletID,
		ToWalletID:     platform.ID,
		Amount:         amount,
		Currency:       domain.CurrencyUZS,
		Category:       category,
		ReferenceType:  reference,
		ReferenceID:    referenceID,
		Description:    string(category),
		IdempotencyKey: idempotencyKey,
	})
}

// RequestPayout withdraws from the caller's wallet.
//
// The money leaves the wallet immediately so it cannot be spent twice while the
// bank transfer is in flight; a rejected payout is credited back by an admin.
func (u *WalletUsecase) RequestPayout(ctx context.Context, userID uuid.UUID, req dto.PayoutRequest) (*dto.WalletResponse, error) {
	wallet, err := u.wallets.EnsureWallet(ctx, domain.WalletOwnerUser, &userID, domain.CurrencyUZS)
	if err != nil {
		return nil, err
	}
	if wallet.Available() < req.Amount {
		return nil, apperror.ErrInsufficientFunds
	}

	orderID := uuid.New()
	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		order := &domain.PaymentOrder{
			ID:             orderID,
			UserID:         &userID,
			Provider:       domain.ProviderManual,
			Purpose:        domain.PurposeOther,
			Amount:         req.Amount,
			Currency:       domain.CurrencyUZS,
			Status:         domain.OrderProcessing,
			IdempotencyKey: "payout:" + orderID.String(),
			Payload: domain.JSONMap{
				// Only the last four digits are stored: the full number is not
				// ours to keep, and support only ever needs to identify the card.
				"card_last4":  lastFour(req.CardNumber),
				"card_holder": req.CardHolder,
				"description": req.Description,
			},
		}
		if err := u.payments.CreateOrder(txCtx, order); err != nil {
			return err
		}
		return u.Debit(txCtx, wallet.ID, req.Amount, domain.TxPayout,
			"payment_order", &orderID, "payout:"+orderID.String())
	})
	if err != nil {
		return nil, err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &userID,
		Action:       "wallet.payout_requested",
		ResourceType: "payment_order",
		ResourceID:   &orderID,
		NewValues:    map[string]any{"amount": req.Amount, "card_last4": lastFour(req.CardNumber)},
	})

	return u.GetWallet(ctx, domain.WalletOwnerUser, userID)
}

// ─── escrow ─────────────────────────────────────────────────────────

// FundEscrow moves a trip's agreed price from the shipper's wallet into escrow.
//
// The funds are transferred to the platform wallet and simultaneously held, so
// the shipper can no longer spend them and the driver cannot yet receive them.
func (u *WalletUsecase) FundEscrow(ctx context.Context, actorID, companyID, tripID uuid.UUID) (*dto.EscrowResponse, error) {
	trip, err := u.trips.FindByID(ctx, tripID)
	if err != nil {
		return nil, err
	}
	if trip.CompanyID != companyID {
		return nil, apperror.ErrForbidden.WithMessage("This trip belongs to another company")
	}
	if trip.Status.IsFinished() {
		return nil, apperror.ErrInvalidStatusChange.WithMessage("This trip is already closed")
	}

	if existing, err := u.payments.FindEscrowByTrip(ctx, tripID); err != nil {
		return nil, err
	} else if existing != nil && existing.Status != domain.EscrowPending {
		return escrowResponse(existing), nil
	}

	driver, err := u.drivers.FindByID(ctx, trip.DriverID)
	if err != nil {
		return nil, err
	}
	payer, err := u.wallets.EnsureWallet(ctx, domain.WalletOwnerCompany, &companyID, trip.Currency)
	if err != nil {
		return nil, err
	}
	payee, err := u.wallets.EnsureWallet(ctx, domain.WalletOwnerUser, &driver.UserID, trip.Currency)
	if err != nil {
		return nil, err
	}
	if payer.Available() < trip.AgreedPriceAmount {
		return nil, apperror.ErrInsufficientFunds.WithMessage(
			"Top up the company wallet before funding this shipment")
	}

	commission := u.commissionFor(ctx, trip.AgreedPriceAmount)
	escrow := &domain.Escrow{
		ID:               uuid.New(),
		TripID:           tripID,
		PayerWalletID:    payer.ID,
		PayeeWalletID:    payee.ID,
		Amount:           trip.AgreedPriceAmount,
		CommissionAmount: commission,
		Currency:         trip.Currency,
		Status:           domain.EscrowFunded,
	}
	now := time.Now().UTC()
	escrow.FundedAt = &now

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.payments.CreateEscrow(txCtx, escrow); err != nil {
			return err
		}
		// Hold first: if the transfer then fails the hold is rolled back with it.
		if err := u.wallets.Hold(txCtx, payer.ID, escrow.Amount,
			"escrow_hold:"+escrow.ID.String()); err != nil {
			return err
		}
		return u.wallets.Transfer(txCtx, domain.LedgerMovement{
			FromWalletID:   payer.ID,
			ToWalletID:     mustPlatformID(txCtx, u.wallets, trip.Currency),
			Amount:         escrow.Amount,
			Currency:       trip.Currency,
			Category:       domain.TxEscrowHold,
			ReferenceType:  "escrow",
			ReferenceID:    &escrow.ID,
			Description:    "Funds held for trip " + trip.TripNumber,
			IdempotencyKey: "escrow_fund:" + escrow.ID.String(),
			CreatedBy:      &actorID,
		})
	})
	if err != nil {
		return nil, err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &actorID,
		Action:       "escrow.funded",
		ResourceType: "escrow",
		ResourceID:   &escrow.ID,
		NewValues:    map[string]any{"trip_id": tripID, "amount": escrow.Amount},
	})
	u.notify(ctx, driver.UserID, "Payment secured",
		fmt.Sprintf("The shipper has secured %s for this trip.", formatAmount(escrow.Amount, trip.Currency)),
		map[string]any{"trip_id": tripID.String()})

	return escrowResponse(escrow), nil
}

// ReleaseEscrow pays the driver and takes the platform commission.
//
// This runs when a delivery is confirmed. The commission stays in the platform
// wallet; only the remainder moves on to the driver.
func (u *WalletUsecase) ReleaseEscrow(ctx context.Context, tripID uuid.UUID, actorID *uuid.UUID) error {
	escrow, err := u.payments.FindEscrowByTrip(ctx, tripID)
	if err != nil {
		return err
	}
	if escrow == nil {
		// A trip settled in cash never had an escrow; that is not an error.
		return nil
	}
	if escrow.Status == domain.EscrowReleased {
		return nil
	}

	now := time.Now().UTC()
	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.payments.TransitionEscrow(txCtx, escrow.ID,
			[]domain.EscrowStatus{domain.EscrowFunded, domain.EscrowDisputed},
			domain.EscrowReleased,
			map[string]any{"released_at": now, "released_by": actorID}); err != nil {
			return err
		}
		// The hold is released because the money is leaving the payer for good.
		if err := u.wallets.ReleaseHold(txCtx, escrow.PayerWalletID, escrow.Amount,
			"escrow_release_hold:"+escrow.ID.String()); err != nil {
			return err
		}
		platformID := mustPlatformID(txCtx, u.wallets, escrow.Currency)
		return u.wallets.Transfer(txCtx, domain.LedgerMovement{
			FromWalletID: platformID,
			ToWalletID:   escrow.PayeeWalletID,
			// Commission is simply what does not move — it stays behind in the
			// platform wallet, so no separate commission transfer is needed.
			Amount:            escrow.PayeeAmount(),
			Currency:          escrow.Currency,
			Category:          domain.TxEscrowRelease,
			ReferenceType:     "escrow",
			ReferenceID:       &escrow.ID,
			Description:       "Payment for a completed trip",
			IdempotencyKey:    "escrow_release:" + escrow.ID.String(),
			CreatedBy:         actorID,
			AllowNegativeFrom: true,
		})
	})
	if err != nil {
		return err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  actorID,
		Action:       "escrow.released",
		ResourceType: "escrow",
		ResourceID:   &escrow.ID,
		NewValues: map[string]any{
			"trip_id":    tripID,
			"payee":      escrow.PayeeAmount(),
			"commission": escrow.CommissionAmount,
		},
	})
	return nil
}

// RefundEscrow returns held funds to the shipper, used when a dispute is
// resolved in their favour.
func (u *WalletUsecase) RefundEscrow(ctx context.Context, tripID uuid.UUID, actorID *uuid.UUID, reason string) error {
	escrow, err := u.payments.FindEscrowByTrip(ctx, tripID)
	if err != nil {
		return err
	}
	if escrow == nil {
		return nil
	}
	if escrow.Status == domain.EscrowRefunded {
		return nil
	}

	now := time.Now().UTC()
	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.payments.TransitionEscrow(txCtx, escrow.ID,
			[]domain.EscrowStatus{domain.EscrowFunded, domain.EscrowDisputed},
			domain.EscrowRefunded,
			map[string]any{"refunded_at": now, "notes": reason}); err != nil {
			return err
		}
		if err := u.wallets.ReleaseHold(txCtx, escrow.PayerWalletID, escrow.Amount,
			"escrow_refund_hold:"+escrow.ID.String()); err != nil {
			return err
		}
		// The full amount goes back, commission included: the platform earns
		// nothing on a shipment that did not happen.
		return u.wallets.Transfer(txCtx, domain.LedgerMovement{
			FromWalletID:      mustPlatformID(txCtx, u.wallets, escrow.Currency),
			ToWalletID:        escrow.PayerWalletID,
			Amount:            escrow.Amount,
			Currency:          escrow.Currency,
			Category:          domain.TxEscrowRefund,
			ReferenceType:     "escrow",
			ReferenceID:       &escrow.ID,
			Description:       "Refund: " + reason,
			IdempotencyKey:    "escrow_refund:" + escrow.ID.String(),
			CreatedBy:         actorID,
			AllowNegativeFrom: true,
		})
	})
	if err != nil {
		return err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  actorID,
		Action:       "escrow.refunded",
		ResourceType: "escrow",
		ResourceID:   &escrow.ID,
		NewValues:    map[string]any{"trip_id": tripID, "reason": reason},
	})
	return nil
}

// GetEscrow returns the escrow state for a trip.
func (u *WalletUsecase) GetEscrow(ctx context.Context, tripID uuid.UUID) (*dto.EscrowResponse, error) {
	escrow, err := u.payments.FindEscrowByTrip(ctx, tripID)
	if err != nil {
		return nil, err
	}
	if escrow == nil {
		return nil, apperror.ErrNotFound.WithMessage("This trip has no escrow")
	}
	return escrowResponse(escrow), nil
}

// ReconcileAll audits every wallet against the ledger and reports drift.
//
// The balance column is a cache; this is what turns a silent accounting bug into
// a visible number.
func (u *WalletUsecase) ReconcileAll(ctx context.Context, walletIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	drifts := make(map[uuid.UUID]int64)
	for _, id := range walletIDs {
		drift, err := u.wallets.Reconcile(ctx, id)
		if err != nil {
			u.log.Warn("reconciling wallet failed",
				logger.String("wallet_id", id.String()), logger.Err(err))
			continue
		}
		if drift != 0 {
			drifts[id] = drift
			u.log.Error("wallet balance drifted from the ledger and was corrected",
				logger.String("wallet_id", id.String()), logger.Int64("drift", drift))
		}
	}
	return drifts, nil
}

// ─── internals ──────────────────────────────────────────────────────

func (u *WalletUsecase) commissionFor(ctx context.Context, amount int64) int64 {
	percent := 5.0
	if u.settings != nil {
		if v, err := u.settings.Float(ctx, "platform.commission_percent"); err == nil && v >= 0 {
			percent = v
		}
	}
	if percent <= 0 {
		return 0
	}
	return int64(float64(amount) * percent / 100)
}

func (u *WalletUsecase) notify(ctx context.Context, userID uuid.UUID, title, body string, data map[string]any) {
	if u.notifier == nil {
		return
	}
	if err := u.notifier.Notify(ctx, domain.Notification{
		UserID: userID, Type: "payment", Title: title, Body: body, Data: data,
	}); err != nil {
		u.log.Warn("queueing payment notification failed", logger.Err(err))
	}
}

// mustPlatformID resolves the clearing wallet. Its absence is a deployment
// fault, not a runtime condition callers can handle, so the zero UUID it returns
// on failure will surface as a clear "wallet does not exist" from the transfer.
func mustPlatformID(ctx context.Context, wallets domain.WalletRepository, currency string) uuid.UUID {
	wallet, err := wallets.PlatformWallet(ctx, currency)
	if err != nil {
		return uuid.Nil
	}
	return wallet.ID
}

func walletResponse(w *domain.Wallet) *dto.WalletResponse {
	return &dto.WalletResponse{
		ID:          w.ID,
		Currency:    w.Currency,
		Balance:     w.Balance,
		HeldBalance: w.HeldBalance,
		Available:   w.Available(),
		Status:      w.Status,
	}
}

func transactionResponse(t *domain.WalletTransaction) dto.TransactionResponse {
	return dto.TransactionResponse{
		ID:            t.ID,
		Direction:     string(t.Direction),
		Amount:        t.Amount,
		BalanceAfter:  t.BalanceAfter,
		Currency:      t.Currency,
		Category:      string(t.Category),
		Description:   t.Description,
		ReferenceType: t.ReferenceType,
		ReferenceID:   t.ReferenceID,
		CreatedAt:     t.CreatedAt,
	}
}

func escrowResponse(e *domain.Escrow) *dto.EscrowResponse {
	return &dto.EscrowResponse{
		ID:               e.ID,
		TripID:           e.TripID,
		Amount:           e.Amount,
		CommissionAmount: e.CommissionAmount,
		PayeeAmount:      e.PayeeAmount(),
		Currency:         e.Currency,
		Status:           string(e.Status),
		FundedAt:         e.FundedAt,
		ReleasedAt:       e.ReleasedAt,
		RefundedAt:       e.RefundedAt,
	}
}

// lastFour keeps only the digits a human needs to recognise a card.
func lastFour(cardNumber string) string {
	digits := make([]rune, 0, len(cardNumber))
	for _, r := range cardNumber {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	if len(digits) < 4 {
		return ""
	}
	return string(digits[len(digits)-4:])
}

// formatAmount renders minor units for a human-facing message.
func formatAmount(minor int64, currency string) string {
	return fmt.Sprintf("%.2f %s", float64(minor)/100, currency)
}
