package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/apperror"
)

type walletRepository struct{ Base }

// NewWalletRepository builds the wallet and ledger repository.
func NewWalletRepository(db *gorm.DB) domain.WalletRepository {
	return &walletRepository{Base: NewBase(db)}
}

// EnsureWallet returns the owner's wallet, creating it on first use so no other
// code has to care whether a wallet exists yet.
func (r *walletRepository) EnsureWallet(ctx context.Context, ownerType domain.WalletOwnerType, ownerID *uuid.UUID, currency string) (*domain.Wallet, error) {
	if currency == "" {
		currency = domain.CurrencyUZS
	}
	if ownerType == domain.WalletOwnerPlatform {
		return r.PlatformWallet(ctx, currency)
	}
	if ownerID == nil {
		return nil, apperror.ErrInternal.Wrap(errors.New("wallet: owner id is required"))
	}

	existing, err := r.FindByOwner(ctx, ownerType, *ownerID, currency)
	if err == nil {
		return existing, nil
	}
	if !apperror.Is(err, apperror.ErrNotFound.Code) {
		return nil, err
	}

	wallet := &domain.Wallet{
		ID:        uuid.New(),
		OwnerType: ownerType,
		OwnerID:   ownerID,
		Currency:  currency,
		Status:    "active",
	}
	// DoNothing plus a re-read handles the race where two requests create the
	// first wallet for the same owner at once.
	err = r.DB(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(wallet).Error
	if err != nil {
		return nil, translate(err, "wallet")
	}
	return r.FindByOwner(ctx, ownerType, *ownerID, currency)
}

func (r *walletRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Wallet, error) {
	var wallet domain.Wallet
	if err := r.DB(ctx).First(&wallet, "id = ?", id).Error; err != nil {
		return nil, translate(err, "wallet")
	}
	return &wallet, nil
}

func (r *walletRepository) FindByOwner(ctx context.Context, ownerType domain.WalletOwnerType, ownerID uuid.UUID, currency string) (*domain.Wallet, error) {
	var wallet domain.Wallet
	err := r.DB(ctx).First(&wallet,
		"owner_type = ? AND owner_id = ? AND currency = ?", ownerType, ownerID, currency).Error
	if err != nil {
		return nil, translate(err, "wallet")
	}
	return &wallet, nil
}

// PlatformWallet returns the single platform wallet for a currency, creating it
// if the reference migration has not run.
func (r *walletRepository) PlatformWallet(ctx context.Context, currency string) (*domain.Wallet, error) {
	if currency == "" {
		currency = domain.CurrencyUZS
	}
	var wallet domain.Wallet
	err := r.DB(ctx).First(&wallet,
		"owner_type = ? AND currency = ?", domain.WalletOwnerPlatform, currency).Error
	if err == nil {
		return &wallet, nil
	}
	if !isNotFound(err) {
		return nil, translate(err, "wallet")
	}

	created := &domain.Wallet{
		ID:        uuid.New(),
		OwnerType: domain.WalletOwnerPlatform,
		Currency:  currency,
		Status:    "active",
	}
	if err := r.DB(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(created).Error; err != nil {
		return nil, translate(err, "wallet")
	}
	if err := r.DB(ctx).First(&wallet,
		"owner_type = ? AND currency = ?", domain.WalletOwnerPlatform, currency).Error; err != nil {
		return nil, translate(err, "wallet")
	}
	return &wallet, nil
}

func (r *walletRepository) SetStatus(ctx context.Context, walletID uuid.UUID, status string) error {
	err := r.DB(ctx).Model(&domain.Wallet{}).
		Where("id = ?", walletID).Update("status", status).Error
	return translate(err, "wallet")
}

// Transfer moves money between two wallets as one atomic double-entry pair.
//
// Three things make this safe under concurrency:
//   - the whole movement runs in a transaction;
//   - both wallet rows are locked FOR UPDATE in a deterministic order (by id),
//     so two transfers touching the same pair cannot deadlock;
//   - the idempotency key is unique in the database, so a retried request that
//     already committed is detected rather than applied twice.
func (r *walletRepository) Transfer(ctx context.Context, m domain.LedgerMovement) error {
	if m.Amount <= 0 {
		return apperror.ErrValidation.WithMessage("The transfer amount must be positive")
	}
	if m.FromWalletID == m.ToWalletID {
		return apperror.ErrValidation.WithMessage("A transfer needs two different wallets")
	}
	if m.IdempotencyKey == "" {
		return apperror.ErrInternal.Wrap(errors.New("ledger: idempotency key is required"))
	}
	if m.Currency == "" {
		m.Currency = domain.CurrencyUZS
	}

	// A completed movement returns success without touching balances again.
	if existing, err := r.FindTransactionByKey(ctx, debitKey(m.IdempotencyKey)); err != nil {
		return err
	} else if existing != nil {
		return nil
	}

	return r.transaction(ctx, func(tx *gorm.DB) error {
		first, second := m.FromWalletID, m.ToWalletID
		if second.String() < first.String() {
			first, second = second, first
		}

		var locked []domain.Wallet
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id IN ?", []uuid.UUID{first, second}).
			Order("id").
			Find(&locked).Error
		if err != nil {
			return translate(err, "wallet")
		}
		if len(locked) != 2 {
			return apperror.ErrNotFound.WithMessage("One of the wallets does not exist")
		}

		wallets := make(map[uuid.UUID]*domain.Wallet, 2)
		for i := range locked {
			wallets[locked[i].ID] = &locked[i]
		}
		from, to := wallets[m.FromWalletID], wallets[m.ToWalletID]

		if !from.IsUsable() || !to.IsUsable() {
			return apperror.ErrForbidden.WithMessage("One of the wallets is frozen")
		}
		if from.Currency != m.Currency || to.Currency != m.Currency {
			return apperror.ErrValidation.WithMessage("The wallets and the movement use different currencies")
		}
		// The payer must have the money, and it must not be money already
		// committed to an escrow.
		if !m.AllowNegativeFrom && from.Available() < m.Amount {
			return apperror.ErrInsufficientFunds
		}

		groupID := uuid.New()
		fromBalance := from.Balance - m.Amount
		toBalance := to.Balance + m.Amount

		entries := []domain.WalletTransaction{
			{
				ID: uuid.New(), GroupID: groupID, WalletID: from.ID,
				Direction: domain.Debit, Amount: m.Amount, BalanceAfter: fromBalance,
				Currency: m.Currency, Category: m.Category,
				ReferenceType: nilIfEmpty(m.ReferenceType), ReferenceID: m.ReferenceID,
				Description:    nilIfEmpty(m.Description),
				IdempotencyKey: debitKey(m.IdempotencyKey), CreatedBy: m.CreatedBy,
			},
			{
				ID: uuid.New(), GroupID: groupID, WalletID: to.ID,
				Direction: domain.Credit, Amount: m.Amount, BalanceAfter: toBalance,
				Currency: m.Currency, Category: m.Category,
				ReferenceType: nilIfEmpty(m.ReferenceType), ReferenceID: m.ReferenceID,
				Description:    nilIfEmpty(m.Description),
				IdempotencyKey: creditKey(m.IdempotencyKey), CreatedBy: m.CreatedBy,
			},
		}
		if err := tx.Create(&entries).Error; err != nil {
			if isUniqueViolation(err) {
				// Another request committed the same movement between our check
				// and this insert. The ledger is already correct.
				return nil
			}
			return translate(err, "wallet transaction")
		}

		if err := tx.Model(&domain.Wallet{}).Where("id = ?", from.ID).
			Update("balance", fromBalance).Error; err != nil {
			return translate(err, "wallet")
		}
		if err := tx.Model(&domain.Wallet{}).Where("id = ?", to.ID).
			Update("balance", toBalance).Error; err != nil {
			return translate(err, "wallet")
		}
		return nil
	})
}

// Hold reserves part of a balance without moving it.
//
// The money still belongs to the payer — it is simply no longer available to
// spend, which is what an escrow needs before delivery is confirmed.
func (r *walletRepository) Hold(ctx context.Context, walletID uuid.UUID, amount int64, idempotencyKey string) error {
	if amount <= 0 {
		return apperror.ErrValidation.WithMessage("The hold amount must be positive")
	}
	return r.transaction(ctx, func(tx *gorm.DB) error {
		var wallet domain.Wallet
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&wallet, "id = ?", walletID).Error
		if err != nil {
			return translate(err, "wallet")
		}
		if !wallet.IsUsable() {
			return apperror.ErrForbidden.WithMessage("This wallet is frozen")
		}
		if wallet.Available() < amount {
			return apperror.ErrInsufficientFunds
		}
		err = tx.Model(&domain.Wallet{}).Where("id = ?", walletID).
			Update("held_balance", wallet.HeldBalance+amount).Error
		return translate(err, "wallet")
	})
}

// ReleaseHold frees reserved funds. It clamps at zero rather than failing: a
// double release is a bug worth fixing, but leaving a wallet with a negative
// hold would be worse.
func (r *walletRepository) ReleaseHold(ctx context.Context, walletID uuid.UUID, amount int64, idempotencyKey string) error {
	if amount <= 0 {
		return apperror.ErrValidation.WithMessage("The release amount must be positive")
	}
	return r.transaction(ctx, func(tx *gorm.DB) error {
		var wallet domain.Wallet
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&wallet, "id = ?", walletID).Error
		if err != nil {
			return translate(err, "wallet")
		}
		remaining := max(wallet.HeldBalance-amount, 0)
		err = tx.Model(&domain.Wallet{}).Where("id = ?", walletID).
			Update("held_balance", remaining).Error
		return translate(err, "wallet")
	})
}

func (r *walletRepository) ListTransactions(ctx context.Context, walletID uuid.UUID, offset, limit int) ([]domain.WalletTransaction, int64, error) {
	q := r.DB(ctx).Model(&domain.WalletTransaction{}).Where("wallet_id = ?", walletID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "wallet transaction")
	}

	var entries []domain.WalletTransaction
	err := applyPage(q.Order("created_at DESC"), offset, limit).Find(&entries).Error
	if err != nil {
		return nil, 0, translate(err, "wallet transaction")
	}
	return entries, total, nil
}

func (r *walletRepository) FindTransactionByKey(ctx context.Context, idempotencyKey string) (*domain.WalletTransaction, error) {
	var entry domain.WalletTransaction
	err := r.DB(ctx).First(&entry, "idempotency_key = ?", idempotencyKey).Error
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, translate(err, "wallet transaction")
	}
	return &entry, nil
}

// Reconcile recomputes a wallet's balance from the ledger and corrects it.
//
// The balance column is a cache; the ledger is the truth. This runs as a
// scheduled audit and reports any drift so a bug shows up as a number rather
// than as quietly wrong money.
func (r *walletRepository) Reconcile(ctx context.Context, walletID uuid.UUID) (int64, error) {
	var result struct {
		Cached   int64
		Computed int64
	}
	err := r.DB(ctx).Raw(`
		SELECT
			w.balance AS cached,
			COALESCE((
				SELECT SUM(CASE WHEN t.direction = 'credit' THEN t.amount ELSE -t.amount END)
				FROM wallet_transactions t WHERE t.wallet_id = w.id
			), 0) AS computed
		FROM wallets w WHERE w.id = ?`, walletID).Scan(&result).Error
	if err != nil {
		return 0, translate(err, "wallet")
	}

	drift := result.Cached - result.Computed
	if drift == 0 {
		return 0, nil
	}
	err = r.DB(ctx).Model(&domain.Wallet{}).
		Where("id = ?", walletID).Update("balance", result.Computed).Error
	if err != nil {
		return drift, translate(err, "wallet")
	}
	return drift, nil
}

// transaction runs fn in the ambient transaction when one is open, or opens its
// own. Ledger writes must never run outside a transaction.
func (r *walletRepository) transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	db := r.DB(ctx)
	if db.Statement != nil && db.Statement.ConnPool != nil {
		if _, inTx := db.Statement.ConnPool.(gorm.TxCommitter); inTx {
			return fn(db)
		}
	}
	return db.Transaction(fn)
}

// The two legs of a movement share the caller's key with a suffix, because the
// unique index is per row and both legs must be individually idempotent.
func debitKey(key string) string  { return key + ":debit" }
func creditKey(key string) string { return key + ":credit" }

// Compile-time proof that the adapter still satisfies the port.
var _ domain.WalletRepository = (*walletRepository)(nil)
