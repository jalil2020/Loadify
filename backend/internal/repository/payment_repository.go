package repository

import (
	"context"
	"maps"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/pkg/apperror"
)

type paymentRepository struct{ Base }

// NewPaymentRepository builds the payment repository.
func NewPaymentRepository(db *gorm.DB) domain.PaymentRepository {
	return &paymentRepository{Base: NewBase(db)}
}

// ─── orders ─────────────────────────────────────────────────────────

func (r *paymentRepository) CreateOrder(ctx context.Context, order *domain.PaymentOrder) error {
	if order.Payload == nil {
		order.Payload = domain.JSONMap{}
	}
	err := r.DB(ctx).Create(order).Error
	if err != nil && isUniqueViolation(err) {
		return apperror.ErrConflict.
			WithMessage("This payment has already been started").Wrap(err)
	}
	return translate(err, "payment order")
}

func (r *paymentRepository) FindOrderByID(ctx context.Context, id uuid.UUID) (*domain.PaymentOrder, error) {
	var order domain.PaymentOrder
	if err := r.DB(ctx).First(&order, "id = ?", id).Error; err != nil {
		return nil, translate(err, "payment order")
	}
	return &order, nil
}

// FindOrderByKey resolves an idempotent retry to the order it already created.
func (r *paymentRepository) FindOrderByKey(ctx context.Context, idempotencyKey string) (*domain.PaymentOrder, error) {
	var order domain.PaymentOrder
	err := r.DB(ctx).First(&order, "idempotency_key = ?", idempotencyKey).Error
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, translate(err, "payment order")
	}
	return &order, nil
}

// FindOrderByProviderRef resolves a webhook that only carries the provider's own
// identifier.
func (r *paymentRepository) FindOrderByProviderRef(ctx context.Context, provider domain.PaymentProvider, providerOrderID string) (*domain.PaymentOrder, error) {
	if providerOrderID == "" {
		return nil, nil
	}
	var order domain.PaymentOrder
	err := r.DB(ctx).First(&order,
		"provider = ? AND provider_order_id = ?", provider, providerOrderID).Error
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, translate(err, "payment order")
	}
	return &order, nil
}

func (r *paymentRepository) UpdateOrderColumns(ctx context.Context, id uuid.UUID, columns map[string]any) error {
	if len(columns) == 0 {
		return nil
	}
	err := r.DB(ctx).Model(&domain.PaymentOrder{}).
		Where("id = ?", id).Updates(columns).Error
	return translate(err, "payment order")
}

// TransitionOrder applies a guarded status change.
//
// The `from` set is part of the WHERE clause, which is what makes a replayed
// webhook harmless: the second attempt affects zero rows.
func (r *paymentRepository) TransitionOrder(ctx context.Context, id uuid.UUID, from []domain.OrderStatus, to domain.OrderStatus, columns map[string]any) error {
	updates := make(map[string]any, len(columns)+1)
	maps.Copy(updates, columns)
	updates["status"] = to

	q := r.DB(ctx).Model(&domain.PaymentOrder{}).Where("id = ?", id)
	if len(from) > 0 {
		q = q.Where("status IN ?", from)
	}

	res := q.Updates(updates)
	if res.Error != nil {
		return translate(res.Error, "payment order")
	}
	if res.RowsAffected == 0 {
		return apperror.ErrConflict.WithMessage("This payment has already been settled")
	}
	return nil
}

func (r *paymentRepository) ListOrders(ctx context.Context, userID, companyID *uuid.UUID, offset, limit int) ([]domain.PaymentOrder, int64, error) {
	q := r.DB(ctx).Model(&domain.PaymentOrder{})
	switch {
	case companyID != nil:
		q = q.Where("company_id = ?", *companyID)
	case userID != nil:
		q = q.Where("user_id = ?", *userID)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "payment order")
	}

	var orders []domain.PaymentOrder
	err := applyPage(q.Order("created_at DESC"), offset, limit).Find(&orders).Error
	if err != nil {
		return nil, 0, translate(err, "payment order")
	}
	return orders, total, nil
}

// ExpireStaleOrders closes checkouts the user never completed, so a stale order
// does not block a fresh attempt on the same subscription.
func (r *paymentRepository) ExpireStaleOrders(ctx context.Context, before time.Time) (int64, error) {
	res := r.DB(ctx).Model(&domain.PaymentOrder{}).
		Where("status IN ? AND created_at < ?",
			[]domain.OrderStatus{domain.OrderPending, domain.OrderProcessing}, before).
		Updates(map[string]any{
			"status":        domain.OrderCancelled,
			"failed_reason": "Expired without payment",
		})
	if res.Error != nil {
		return 0, translate(res.Error, "payment order")
	}
	return res.RowsAffected, nil
}

// ─── webhooks ───────────────────────────────────────────────────────

// RecordWebhook stores the callback before it is processed, so even a rejected
// or failing callback leaves a trail.
func (r *paymentRepository) RecordWebhook(ctx context.Context, hook *domain.PaymentWebhook) error {
	if hook.Payload == nil {
		hook.Payload = domain.JSONMap{}
	}
	err := r.DB(ctx).Create(hook).Error
	if err != nil && isUniqueViolation(err) {
		// A replay: the unique index on (provider, event_id) already holds it.
		return apperror.ErrConflict.WithMessage("This callback was already received").Wrap(err)
	}
	return translate(err, "payment webhook")
}

func (r *paymentRepository) FindWebhookByEvent(ctx context.Context, provider, eventID string) (*domain.PaymentWebhook, error) {
	if eventID == "" {
		return nil, nil
	}
	var hook domain.PaymentWebhook
	err := r.DB(ctx).First(&hook, "provider = ? AND event_id = ?", provider, eventID).Error
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, translate(err, "payment webhook")
	}
	return &hook, nil
}

func (r *paymentRepository) MarkWebhookProcessed(ctx context.Context, id uuid.UUID, processErr *string) error {
	columns := map[string]any{"processed_at": time.Now().UTC(), "error": processErr}
	err := r.DB(ctx).Model(&domain.PaymentWebhook{}).
		Where("id = ?", id).Updates(columns).Error
	return translate(err, "payment webhook")
}

// ─── escrow ─────────────────────────────────────────────────────────

func (r *paymentRepository) CreateEscrow(ctx context.Context, escrow *domain.Escrow) error {
	err := r.DB(ctx).Create(escrow).Error
	if err != nil && isUniqueViolation(err) {
		return apperror.ErrConflict.
			WithMessage("This trip already has an escrow").Wrap(err)
	}
	return translate(err, "escrow")
}

func (r *paymentRepository) FindEscrowByTrip(ctx context.Context, tripID uuid.UUID) (*domain.Escrow, error) {
	var escrow domain.Escrow
	err := r.DB(ctx).First(&escrow, "trip_id = ?", tripID).Error
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, translate(err, "escrow")
	}
	return &escrow, nil
}

// TransitionEscrow applies a guarded status change, so funds cannot be released
// twice by two concurrent confirmations.
func (r *paymentRepository) TransitionEscrow(ctx context.Context, id uuid.UUID, from []domain.EscrowStatus, to domain.EscrowStatus, columns map[string]any) error {
	updates := make(map[string]any, len(columns)+1)
	maps.Copy(updates, columns)
	updates["status"] = to

	q := r.DB(ctx).Model(&domain.Escrow{}).Where("id = ?", id)
	if len(from) > 0 {
		q = q.Where("status IN ?", from)
	}

	res := q.Updates(updates)
	if res.Error != nil {
		return translate(res.Error, "escrow")
	}
	if res.RowsAffected == 0 {
		return apperror.ErrConflict.WithMessage("These funds have already been settled")
	}
	return nil
}

// ─── invoices ───────────────────────────────────────────────────────

func (r *paymentRepository) CreateInvoice(ctx context.Context, invoice *domain.Invoice) error {
	if len(invoice.Items) == 0 {
		invoice.Items = domain.JSONRaw("[]")
	}
	return translate(r.DB(ctx).Create(invoice).Error, "invoice")
}

func (r *paymentRepository) ListInvoices(ctx context.Context, companyID, userID *uuid.UUID, offset, limit int) ([]domain.Invoice, int64, error) {
	q := r.DB(ctx).Model(&domain.Invoice{})
	switch {
	case companyID != nil:
		q = q.Where("company_id = ?", *companyID)
	case userID != nil:
		q = q.Where("user_id = ?", *userID)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "invoice")
	}

	var invoices []domain.Invoice
	err := applyPage(q.Order("issued_at DESC"), offset, limit).Find(&invoices).Error
	if err != nil {
		return nil, 0, translate(err, "invoice")
	}
	return invoices, total, nil
}

func (r *paymentRepository) FindInvoiceByID(ctx context.Context, id uuid.UUID) (*domain.Invoice, error) {
	var invoice domain.Invoice
	if err := r.DB(ctx).First(&invoice, "id = ?", id).Error; err != nil {
		return nil, translate(err, "invoice")
	}
	return &invoice, nil
}

// ─── refunds ────────────────────────────────────────────────────────

func (r *paymentRepository) CreateRefund(ctx context.Context, refund *domain.Refund) error {
	return translate(r.DB(ctx).Create(refund).Error, "refund")
}

func (r *paymentRepository) FindRefundByID(ctx context.Context, id uuid.UUID) (*domain.Refund, error) {
	var refund domain.Refund
	if err := r.DB(ctx).First(&refund, "id = ?", id).Error; err != nil {
		return nil, translate(err, "refund")
	}
	return &refund, nil
}

func (r *paymentRepository) ListRefunds(ctx context.Context, status string, offset, limit int) ([]domain.Refund, int64, error) {
	q := r.DB(ctx).Model(&domain.Refund{})
	if status != "" {
		q = q.Where("status = ?", status)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "refund")
	}

	var refunds []domain.Refund
	err := applyPage(q.Order("created_at DESC"), offset, limit).Find(&refunds).Error
	if err != nil {
		return nil, 0, translate(err, "refund")
	}
	return refunds, total, nil
}

func (r *paymentRepository) UpdateRefundColumns(ctx context.Context, id uuid.UUID, columns map[string]any) error {
	if len(columns) == 0 {
		return nil
	}
	err := r.DB(ctx).Model(&domain.Refund{}).Where("id = ?", id).Updates(columns).Error
	return translate(err, "refund")
}

// ─── subscriptions ──────────────────────────────────────────────────

type subscriptionRepository struct{ Base }

// NewSubscriptionRepository builds the subscription repository.
func NewSubscriptionRepository(db *gorm.DB) domain.SubscriptionRepository {
	return &subscriptionRepository{Base: NewBase(db)}
}

func (r *subscriptionRepository) ListPlans(ctx context.Context, targetRole string) ([]domain.SubscriptionPlan, error) {
	q := r.DB(ctx).Where("is_active")
	if targetRole != "" {
		q = q.Where("target_role = ?", targetRole)
	}
	var plans []domain.SubscriptionPlan
	if err := q.Order("target_role, sort_order").Find(&plans).Error; err != nil {
		return nil, translate(err, "subscription plan")
	}
	return plans, nil
}

func (r *subscriptionRepository) FindPlanByID(ctx context.Context, id uuid.UUID) (*domain.SubscriptionPlan, error) {
	var plan domain.SubscriptionPlan
	if err := r.DB(ctx).First(&plan, "id = ?", id).Error; err != nil {
		return nil, translate(err, "subscription plan")
	}
	return &plan, nil
}

func (r *subscriptionRepository) FindPlanByCode(ctx context.Context, code string) (*domain.SubscriptionPlan, error) {
	var plan domain.SubscriptionPlan
	if err := r.DB(ctx).First(&plan, "code = ?", code).Error; err != nil {
		return nil, translate(err, "subscription plan")
	}
	return &plan, nil
}

// DefaultPlan is the free tier every account falls back to.
func (r *subscriptionRepository) DefaultPlan(ctx context.Context, targetRole string) (*domain.SubscriptionPlan, error) {
	var plan domain.SubscriptionPlan
	err := r.DB(ctx).First(&plan,
		"target_role = ? AND is_default AND is_active", targetRole).Error
	if err != nil {
		return nil, translate(err, "subscription plan")
	}
	return &plan, nil
}

func (r *subscriptionRepository) Create(ctx context.Context, subscription *domain.Subscription) error {
	err := r.DB(ctx).Create(subscription).Error
	if err != nil && isUniqueViolation(err) {
		// The partial unique index allows only one active subscription per owner.
		return apperror.ErrSubscriptionActive.Wrap(err)
	}
	return translate(err, "subscription")
}

func (r *subscriptionRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Subscription, error) {
	var subscription domain.Subscription
	err := r.DB(ctx).Preload("Plan").First(&subscription, "id = ?", id).Error
	if err != nil {
		return nil, translate(err, "subscription")
	}
	return &subscription, nil
}

// ActiveForUser returns the caller's paid plan, or nil when they are on the free
// tier — which is the normal case, not an error.
func (r *subscriptionRepository) ActiveForUser(ctx context.Context, userID uuid.UUID) (*domain.Subscription, error) {
	return r.findActive(ctx, "user_id = ?", userID)
}

func (r *subscriptionRepository) ActiveForCompany(ctx context.Context, companyID uuid.UUID) (*domain.Subscription, error) {
	return r.findActive(ctx, "company_id = ?", companyID)
}

func (r *subscriptionRepository) findActive(ctx context.Context, where string, id uuid.UUID) (*domain.Subscription, error) {
	var subscription domain.Subscription
	err := r.DB(ctx).Preload("Plan").
		Where(where, id).
		Where("status IN ?", []domain.SubscriptionStatus{
			domain.SubscriptionActive, domain.SubscriptionTrialing,
		}).
		Where("expires_at IS NULL OR expires_at > now()").
		Order("expires_at DESC NULLS FIRST").
		First(&subscription).Error
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, translate(err, "subscription")
	}
	return &subscription, nil
}

func (r *subscriptionRepository) UpdateColumns(ctx context.Context, id uuid.UUID, columns map[string]any) error {
	if len(columns) == 0 {
		return nil
	}
	err := r.DB(ctx).Model(&domain.Subscription{}).
		Where("id = ?", id).Updates(columns).Error
	return translate(err, "subscription")
}

func (r *subscriptionRepository) ListHistoryForUser(ctx context.Context, userID uuid.UUID, offset, limit int) ([]domain.Subscription, int64, error) {
	return r.listHistory(ctx, "user_id = ?", userID, offset, limit)
}

func (r *subscriptionRepository) ListHistoryForCompany(ctx context.Context, companyID uuid.UUID, offset, limit int) ([]domain.Subscription, int64, error) {
	return r.listHistory(ctx, "company_id = ?", companyID, offset, limit)
}

func (r *subscriptionRepository) listHistory(ctx context.Context, where string, id uuid.UUID, offset, limit int) ([]domain.Subscription, int64, error) {
	q := r.DB(ctx).Model(&domain.Subscription{}).Where(where, id)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, translate(err, "subscription")
	}

	var subscriptions []domain.Subscription
	err := applyPage(q.Preload("Plan").Order("created_at DESC"), offset, limit).
		Find(&subscriptions).Error
	if err != nil {
		return nil, 0, translate(err, "subscription")
	}
	return subscriptions, total, nil
}

// ExpireOverdue closes subscriptions past their end date, which is what drops an
// account back to the free tier.
func (r *subscriptionRepository) ExpireOverdue(ctx context.Context, now time.Time) (int64, error) {
	res := r.DB(ctx).Model(&domain.Subscription{}).
		Where("status IN ? AND expires_at IS NOT NULL AND expires_at < ?",
			[]domain.SubscriptionStatus{domain.SubscriptionActive, domain.SubscriptionTrialing}, now).
		Update("status", domain.SubscriptionExpired)
	if res.Error != nil {
		return 0, translate(res.Error, "subscription")
	}
	return res.RowsAffected, nil
}

// ListRenewable finds auto-renewing subscriptions due for a charge.
func (r *subscriptionRepository) ListRenewable(ctx context.Context, before time.Time, limit int) ([]domain.Subscription, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	var subscriptions []domain.Subscription
	err := r.DB(ctx).Preload("Plan").
		Where("auto_renew AND status = ? AND expires_at IS NOT NULL AND expires_at < ?",
			domain.SubscriptionActive, before).
		Order("expires_at").
		Limit(limit).
		Find(&subscriptions).Error
	if err != nil {
		return nil, translate(err, "subscription")
	}
	return subscriptions, nil
}
