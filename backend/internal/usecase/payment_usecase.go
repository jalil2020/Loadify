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

// checkoutTTL is how long an unpaid checkout stays open before the sweep closes
// it, freeing the user to start a fresh attempt.
const checkoutTTL = 30 * time.Minute

// GatewayRegistry resolves a provider to its adapter.
type GatewayRegistry interface {
	Get(provider domain.PaymentProvider) domain.PaymentGateway
	Enabled() []string
}

// PaymentUsecase owns checkout, provider callbacks, subscriptions and refunds.
type PaymentUsecase struct {
	payments      domain.PaymentRepository
	subscriptions domain.SubscriptionRepository
	wallets       domain.WalletRepository
	walletUC      *WalletUsecase
	loads         domain.LoadRepository
	companies     domain.CompanyRepository
	users         domain.UserRepository
	gateways      GatewayRegistry
	tx            domain.TxManager
	audit         domain.AuditLogger
	notifier      domain.Notifier
	log           logger.Logger
}

// NewPaymentUsecase wires the payment usecase.
func NewPaymentUsecase(
	payments domain.PaymentRepository,
	subscriptions domain.SubscriptionRepository,
	wallets domain.WalletRepository,
	walletUC *WalletUsecase,
	loads domain.LoadRepository,
	companies domain.CompanyRepository,
	users domain.UserRepository,
	gateways GatewayRegistry,
	tx domain.TxManager,
	audit domain.AuditLogger,
	notifier domain.Notifier,
	log logger.Logger,
) *PaymentUsecase {
	return &PaymentUsecase{
		payments: payments, subscriptions: subscriptions, wallets: wallets,
		walletUC: walletUC, loads: loads, companies: companies, users: users,
		gateways: gateways, tx: tx, audit: audit, notifier: notifier, log: log,
	}
}

// ─── plans & entitlements ───────────────────────────────────────────

// ListPlans returns the purchasable tiers for a role, marking the current one.
func (u *PaymentUsecase) ListPlans(ctx context.Context, targetRole string, locale domain.Locale, userID *uuid.UUID, companyID *uuid.UUID) ([]dto.PlanResponse, error) {
	plans, err := u.subscriptions.ListPlans(ctx, targetRole)
	if err != nil {
		return nil, err
	}

	var currentPlanID uuid.UUID
	if current, err := u.currentSubscription(ctx, userID, companyID); err == nil && current != nil {
		currentPlanID = current.PlanID
	}

	out := make([]dto.PlanResponse, 0, len(plans))
	for i := range plans {
		item := planResponse(&plans[i], locale)
		item.IsCurrent = plans[i].ID == currentPlanID
		out = append(out, *item)
	}
	return out, nil
}

// GetSubscription returns the caller's current plan, or nil when they are on the
// free tier.
func (u *PaymentUsecase) GetSubscription(ctx context.Context, userID, companyID *uuid.UUID, locale domain.Locale) (*dto.SubscriptionResponse, error) {
	subscription, err := u.currentSubscription(ctx, userID, companyID)
	if err != nil {
		return nil, err
	}
	if subscription == nil {
		return nil, nil
	}
	return subscriptionResponse(subscription, locale), nil
}

// Entitlements tells a client what the caller's plan allows.
//
// The free tier is resolved as a real plan rather than a special case, so the
// client always receives the same shape whether or not anything was paid for.
func (u *PaymentUsecase) Entitlements(ctx context.Context, targetRole string, userID, companyID *uuid.UUID, locale domain.Locale) (*dto.EntitlementsResponse, error) {
	subscription, err := u.currentSubscription(ctx, userID, companyID)
	if err != nil {
		return nil, err
	}

	var plan *domain.SubscriptionPlan
	isPaid := false
	if subscription != nil && subscription.Plan != nil {
		plan = subscription.Plan
		isPaid = plan.PriceAmount > 0
	} else {
		plan, err = u.subscriptions.DefaultPlan(ctx, targetRole)
		if err != nil {
			return nil, err
		}
	}

	res := &dto.EntitlementsResponse{
		PlanCode: plan.Code,
		PlanName: domain.LocalisedName(plan.NameUZ, plan.NameRU, plan.NameEN, locale),
		IsPaid:   isPaid,
		Limits:   map[string]any(plan.Limits),
		Usage:    map[string]any{},
	}
	if res.Limits == nil {
		res.Limits = map[string]any{}
	}

	// Usage is only meaningful for a company: the limit that actually bites is
	// how many loads they may keep live at once.
	if companyID != nil {
		if active, err := u.loads.CountActiveByCompany(ctx, *companyID); err == nil {
			res.Usage["active_loads"] = active
		}
	}
	return res, nil
}

// AssertCanPublishLoad enforces the company's concurrent-load limit.
//
// Called before a load is published, so the user is told they need a bigger plan
// rather than discovering it after filling in the form.
func (u *PaymentUsecase) AssertCanPublishLoad(ctx context.Context, companyID uuid.UUID) error {
	subscription, err := u.subscriptions.ActiveForCompany(ctx, companyID)
	if err != nil {
		return err
	}

	plan := subscriptionPlan(subscription)
	if plan == nil {
		plan, err = u.subscriptions.DefaultPlan(ctx, "company")
		if err != nil {
			// Without a configured free tier we cannot prove a violation, so the
			// publish is allowed rather than blocked by a configuration gap.
			u.log.Warn("no default company plan configured, skipping the load limit",
				logger.Err(err))
			return nil
		}
	}

	limit := plan.Limit("active_loads")
	if limit < 0 {
		return nil
	}
	active, err := u.loads.CountActiveByCompany(ctx, companyID)
	if err != nil {
		return err
	}
	if active >= limit {
		return apperror.ErrQuotaExceeded.WithMessage(fmt.Sprintf(
			"Your plan allows %d active loads. Upgrade to publish more.", limit))
	}
	return nil
}

// ─── checkout ───────────────────────────────────────────────────────

// Subscribe starts a checkout for a plan.
//
// A wallet payment settles inline; a card provider returns a checkout URL and
// the subscription activates when the callback confirms it.
func (u *PaymentUsecase) Subscribe(ctx context.Context, userID uuid.UUID, companyID *uuid.UUID, req dto.SubscribeRequest) (*dto.CheckoutResponse, error) {
	plan, err := u.subscriptions.FindPlanByID(ctx, req.PlanID)
	if err != nil {
		return nil, err
	}
	if !plan.IsActive {
		return nil, apperror.ErrValidation.WithMessage("This plan is no longer available")
	}

	// A company plan is bought by the company, a driver plan by the person.
	var ownerUserID *uuid.UUID
	var ownerCompanyID *uuid.UUID
	if plan.TargetRole == "company" {
		if companyID == nil {
			return nil, apperror.ErrForbidden.WithMessage("Create a company before buying this plan")
		}
		ownerCompanyID = companyID
	} else {
		ownerUserID = &userID
	}

	if current, err := u.currentSubscription(ctx, ownerUserID, ownerCompanyID); err != nil {
		return nil, err
	} else if current != nil && current.PlanID == plan.ID {
		return nil, apperror.ErrSubscriptionActive.WithMessage("You are already on this plan")
	}

	// The free tier needs no payment at all.
	if plan.PriceAmount == 0 {
		subscription, err := u.activateSubscription(ctx, ownerUserID, ownerCompanyID, plan, nil, req.AutoRenew)
		if err != nil {
			return nil, err
		}
		return &dto.CheckoutResponse{
			OrderID:  subscription.ID,
			Provider: string(domain.ProviderManual),
			Amount:   0,
			Currency: plan.Currency,
			Status:   string(domain.OrderPaid),
			Paid:     true,
		}, nil
	}

	// The key is derived from the buyer, the plan and the day, so a double-tap
	// resolves to the same order rather than opening two checkouts.
	idempotencyKey := fmt.Sprintf("sub:%s:%s:%s",
		ownerKey(ownerUserID, ownerCompanyID), plan.ID, time.Now().UTC().Format("2006-01-02T15"))

	if existing, err := u.payments.FindOrderByKey(ctx, idempotencyKey); err != nil {
		return nil, err
	} else if existing != nil && !existing.Status.IsFinal() {
		return checkoutResponse(existing), nil
	}

	order := &domain.PaymentOrder{
		ID:             uuid.New(),
		UserID:         ownerUserID,
		CompanyID:      ownerCompanyID,
		Provider:       domain.PaymentProvider(req.Provider),
		Purpose:        domain.PurposeSubscription,
		Amount:         plan.PriceAmount,
		Currency:       plan.Currency,
		Status:         domain.OrderPending,
		IdempotencyKey: idempotencyKey,
		Payload: domain.JSONMap{
			"plan_id":    plan.ID.String(),
			"plan_code":  plan.Code,
			"auto_renew": req.AutoRenew,
		},
	}
	referenceType := "subscription_plan"
	order.ReferenceType = &referenceType
	order.ReferenceID = &plan.ID
	expiry := time.Now().UTC().Add(checkoutTTL)
	order.ExpiresAt = &expiry

	if req.Provider == string(domain.ProviderWallet) {
		return u.payFromWallet(ctx, userID, order, plan, ownerUserID, ownerCompanyID, req.AutoRenew)
	}

	gateway := u.gateways.Get(order.Provider)
	if gateway == nil {
		return nil, apperror.ErrUnavailable.WithMessage(
			"This payment method is not available right now")
	}

	if err := u.payments.CreateOrder(ctx, order); err != nil {
		return nil, err
	}

	session, err := gateway.CreateCheckout(ctx, domain.CheckoutRequest{
		OrderID:     order.ID,
		Amount:      order.Amount,
		Currency:    order.Currency,
		Description: domain.LocalisedName(plan.NameUZ, plan.NameRU, plan.NameEN, domain.LocaleEN),
		ReturnURL:   req.ReturnURL,
	})
	if err != nil {
		reason := err.Error()
		if updateErr := u.payments.UpdateOrderColumns(ctx, order.ID, map[string]any{
			"status": domain.OrderFailed, "failed_reason": reason,
		}); updateErr != nil {
			u.log.Warn("marking a failed checkout failed", logger.Err(updateErr))
		}
		return nil, apperror.ErrUnavailable.
			WithMessage("Could not start the payment, try again").Wrap(err)
	}

	columns := map[string]any{"checkout_url": session.CheckoutURL, "status": domain.OrderProcessing}
	if session.ProviderOrderID != "" {
		columns["provider_order_id"] = session.ProviderOrderID
	}
	if err := u.payments.UpdateOrderColumns(ctx, order.ID, columns); err != nil {
		return nil, err
	}

	order.CheckoutURL = &session.CheckoutURL
	order.Status = domain.OrderProcessing
	return checkoutResponse(order), nil
}

// payFromWallet settles a subscription straight from the buyer's balance.
func (u *PaymentUsecase) payFromWallet(ctx context.Context, actorID uuid.UUID, order *domain.PaymentOrder, plan *domain.SubscriptionPlan, ownerUserID, ownerCompanyID *uuid.UUID, autoRenew bool) (*dto.CheckoutResponse, error) {
	ownerType := domain.WalletOwnerUser
	ownerID := ownerUserID
	if ownerCompanyID != nil {
		ownerType = domain.WalletOwnerCompany
		ownerID = ownerCompanyID
	}

	wallet, err := u.wallets.EnsureWallet(ctx, ownerType, ownerID, plan.Currency)
	if err != nil {
		return nil, err
	}
	if wallet.Available() < plan.PriceAmount {
		return nil, apperror.ErrInsufficientFunds.WithMessage(
			"Top up your wallet or pay by card")
	}

	now := time.Now().UTC()
	order.Provider = domain.ProviderWallet
	order.Status = domain.OrderPaid
	order.PaidAt = &now

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.payments.CreateOrder(txCtx, order); err != nil {
			return err
		}
		if err := u.walletUC.Debit(txCtx, wallet.ID, plan.PriceAmount,
			domain.TxSubscription, "payment_order", &order.ID,
			"subscription:"+order.ID.String()); err != nil {
			return err
		}
		_, err := u.activateSubscription(txCtx, ownerUserID, ownerCompanyID, plan, &order.ID, autoRenew)
		return err
	})
	if err != nil {
		return nil, err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &actorID,
		Action:       "subscription.purchased",
		ResourceType: "payment_order",
		ResourceID:   &order.ID,
		NewValues:    map[string]any{"plan": plan.Code, "provider": "wallet"},
	})

	res := checkoutResponse(order)
	res.Paid = true
	return res, nil
}

// Topup starts a checkout that adds funds to the caller's wallet.
func (u *PaymentUsecase) Topup(ctx context.Context, userID uuid.UUID, companyID *uuid.UUID, req dto.TopupRequest) (*dto.CheckoutResponse, error) {
	gateway := u.gateways.Get(domain.PaymentProvider(req.Provider))
	if gateway == nil {
		return nil, apperror.ErrUnavailable.WithMessage(
			"This payment method is not available right now")
	}

	order := &domain.PaymentOrder{
		ID:             uuid.New(),
		Provider:       domain.PaymentProvider(req.Provider),
		Purpose:        domain.PurposeTopup,
		Amount:         req.Amount,
		Currency:       domain.CurrencyUZS,
		Status:         domain.OrderPending,
		IdempotencyKey: "topup:" + uuid.NewString(),
		Payload:        domain.JSONMap{},
	}
	if companyID != nil {
		order.CompanyID = companyID
	} else {
		order.UserID = &userID
	}
	expiry := time.Now().UTC().Add(checkoutTTL)
	order.ExpiresAt = &expiry

	if err := u.payments.CreateOrder(ctx, order); err != nil {
		return nil, err
	}

	session, err := gateway.CreateCheckout(ctx, domain.CheckoutRequest{
		OrderID:     order.ID,
		Amount:      order.Amount,
		Currency:    order.Currency,
		Description: "Wallet top-up",
		ReturnURL:   req.ReturnURL,
	})
	if err != nil {
		return nil, apperror.ErrUnavailable.
			WithMessage("Could not start the payment, try again").Wrap(err)
	}

	if err := u.payments.UpdateOrderColumns(ctx, order.ID, map[string]any{
		"checkout_url": session.CheckoutURL,
		"status":       domain.OrderProcessing,
	}); err != nil {
		return nil, err
	}
	order.CheckoutURL = &session.CheckoutURL
	order.Status = domain.OrderProcessing
	return checkoutResponse(order), nil
}

// ─── provider callbacks ─────────────────────────────────────────────

// HandleWebhook verifies and applies a provider callback.
//
// It always returns a provider-shaped acknowledgement, never an HTTP error: a
// non-200 makes most providers retry forever, and an unverified callback still
// has to be recorded so an attack leaves a trail.
func (u *PaymentUsecase) HandleWebhook(ctx context.Context, provider domain.PaymentProvider, headers map[string]string, body []byte) (any, error) {
	gateway := u.gateways.Get(provider)
	if gateway == nil {
		return nil, apperror.ErrNotFound.WithMessage("Unknown payment provider")
	}

	result, err := gateway.ParseWebhook(ctx, headers, body)
	if err != nil {
		u.log.Warn("could not parse a payment callback",
			logger.String("provider", string(provider)), logger.Err(err))
		return nil, apperror.ErrBadRequest.WithMessage("Malformed callback")
	}

	hook := &domain.PaymentWebhook{
		ID:             uuid.New(),
		Provider:       string(provider),
		EventType:      &result.EventType,
		OrderID:        result.OrderID,
		SignatureValid: result.SignatureValid,
		Payload:        domain.JSONMap(result.Raw),
	}
	if result.EventID != "" {
		hook.EventID = &result.EventID
	}

	// A replay is acknowledged without being applied a second time.
	if result.EventID != "" {
		if existing, err := u.payments.FindWebhookByEvent(ctx, string(provider), result.EventID); err == nil && existing != nil {
			return gateway.WebhookResponse(result, nil), nil
		}
	}
	if err := u.payments.RecordWebhook(ctx, hook); err != nil &&
		!apperror.Is(err, apperror.ErrConflict.Code) {
		u.log.Warn("recording a payment callback failed", logger.Err(err))
	}

	if !result.SignatureValid {
		u.log.Warn("rejected a payment callback with an invalid signature",
			logger.String("provider", string(provider)),
			logger.String("event_id", result.EventID))
		return gateway.WebhookResponse(result, nil), nil
	}

	processErr := u.applyWebhook(ctx, provider, result)
	var reason *string
	if processErr != nil {
		msg := processErr.Error()
		reason = &msg
		u.log.Warn("processing a payment callback failed",
			logger.String("provider", string(provider)), logger.Err(processErr))
	}
	if err := u.payments.MarkWebhookProcessed(ctx, hook.ID, reason); err != nil {
		u.log.Warn("marking a callback processed failed", logger.Err(err))
	}

	return gateway.WebhookResponse(result, processErr), nil
}

// applyWebhook settles or cancels the order the callback refers to.
func (u *PaymentUsecase) applyWebhook(ctx context.Context, provider domain.PaymentProvider, result *domain.WebhookResult) error {
	order, err := u.resolveOrder(ctx, provider, result)
	if err != nil {
		return err
	}
	if order == nil {
		return fmt.Errorf("order not found")
	}
	if result.Amount > 0 && result.Amount != order.Amount {
		return fmt.Errorf("amount mismatch: expected %d, received %d", order.Amount, result.Amount)
	}

	switch {
	case result.Paid:
		return u.settleOrder(ctx, order, result.ProviderOrderID)
	case result.Cancelled:
		return u.payments.TransitionOrder(ctx, order.ID,
			[]domain.OrderStatus{domain.OrderPending, domain.OrderProcessing, domain.OrderPaid},
			domain.OrderCancelled,
			map[string]any{"failed_reason": result.FailureReason})
	default:
		// A pre-check call such as CheckPerformTransaction: the order exists and
		// the amount matches, which is all the provider is asking.
		return nil
	}
}

// settleOrder marks an order paid and applies what it bought.
func (u *PaymentUsecase) settleOrder(ctx context.Context, order *domain.PaymentOrder, providerOrderID string) error {
	now := time.Now().UTC()

	return u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		columns := map[string]any{"paid_at": now}
		if providerOrderID != "" {
			columns["provider_order_id"] = providerOrderID
		}
		// The guard is what makes a duplicate callback harmless.
		if err := u.payments.TransitionOrder(txCtx, order.ID,
			[]domain.OrderStatus{domain.OrderPending, domain.OrderProcessing},
			domain.OrderPaid, columns); err != nil {
			return err
		}

		switch order.Purpose {
		case domain.PurposeTopup:
			return u.creditTopup(txCtx, order)
		case domain.PurposeSubscription:
			return u.activateFromOrder(txCtx, order)
		default:
			return nil
		}
	})
}

// creditTopup adds the paid amount to the buyer's wallet.
func (u *PaymentUsecase) creditTopup(ctx context.Context, order *domain.PaymentOrder) error {
	ownerType := domain.WalletOwnerUser
	ownerID := order.UserID
	if order.CompanyID != nil {
		ownerType = domain.WalletOwnerCompany
		ownerID = order.CompanyID
	}
	if ownerID == nil {
		return fmt.Errorf("top-up order has no owner")
	}

	wallet, err := u.wallets.EnsureWallet(ctx, ownerType, ownerID, order.Currency)
	if err != nil {
		return err
	}
	return u.walletUC.Credit(ctx, wallet.ID, order.Amount,
		domain.TxTopup, "payment_order", &order.ID, "topup:"+order.ID.String())
}

// activateFromOrder turns a paid subscription order into an active plan.
func (u *PaymentUsecase) activateFromOrder(ctx context.Context, order *domain.PaymentOrder) error {
	rawPlanID, _ := order.Payload["plan_id"].(string)
	planID, err := uuid.Parse(rawPlanID)
	if err != nil {
		if order.ReferenceID == nil {
			return fmt.Errorf("subscription order has no plan")
		}
		planID = *order.ReferenceID
	}
	plan, err := u.subscriptions.FindPlanByID(ctx, planID)
	if err != nil {
		return err
	}
	autoRenew, _ := order.Payload["auto_renew"].(bool)

	_, err = u.activateSubscription(ctx, order.UserID, order.CompanyID, plan, &order.ID, autoRenew)
	return err
}

// activateSubscription closes any current plan and starts the new one.
func (u *PaymentUsecase) activateSubscription(ctx context.Context, userID, companyID *uuid.UUID, plan *domain.SubscriptionPlan, orderID *uuid.UUID, autoRenew bool) (*domain.Subscription, error) {
	now := time.Now().UTC()

	// The partial unique index allows only one active plan per owner, so the
	// previous one must be closed before the new one is inserted.
	if current, err := u.currentSubscription(ctx, userID, companyID); err == nil && current != nil {
		if err := u.subscriptions.UpdateColumns(ctx, current.ID, map[string]any{
			"status":       domain.SubscriptionCancelled,
			"cancelled_at": now,
		}); err != nil {
			return nil, err
		}
	}

	expiresAt := plan.Duration(now)
	status := domain.SubscriptionActive
	if plan.TrialDays > 0 {
		status = domain.SubscriptionTrialing
		expiresAt = now.AddDate(0, 0, int(plan.TrialDays))
	}

	subscription := &domain.Subscription{
		ID:             uuid.New(),
		UserID:         userID,
		CompanyID:      companyID,
		PlanID:         plan.ID,
		Status:         status,
		StartedAt:      &now,
		ExpiresAt:      &expiresAt,
		AutoRenew:      autoRenew,
		PaymentOrderID: orderID,
	}
	if err := u.subscriptions.Create(ctx, subscription); err != nil {
		return nil, err
	}

	if userID != nil {
		u.notify(ctx, *userID, "Subscription active",
			fmt.Sprintf("Your %s plan is active until %s.",
				plan.NameEN, expiresAt.Format("2 January 2006")),
			map[string]any{"plan_code": plan.Code})
	}
	return subscription, nil
}

// CancelSubscription stops auto-renewal. The plan keeps working until it expires,
// because the period is already paid for.
func (u *PaymentUsecase) CancelSubscription(ctx context.Context, actorID uuid.UUID, userID, companyID *uuid.UUID) error {
	subscription, err := u.currentSubscription(ctx, userID, companyID)
	if err != nil {
		return err
	}
	if subscription == nil {
		return apperror.ErrNotFound.WithMessage("You have no active subscription")
	}

	if err := u.subscriptions.UpdateColumns(ctx, subscription.ID, map[string]any{
		"auto_renew":   false,
		"cancelled_at": time.Now().UTC(),
	}); err != nil {
		return err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &actorID,
		Action:       "subscription.cancelled",
		ResourceType: "subscription",
		ResourceID:   &subscription.ID,
	})
	return nil
}

// ─── refunds ────────────────────────────────────────────────────────

// RequestRefund records a refund request for admin review.
func (u *PaymentUsecase) RequestRefund(ctx context.Context, userID uuid.UUID, req dto.RequestRefundRequest) (*dto.RefundResponse, error) {
	order, err := u.payments.FindOrderByID(ctx, req.OrderID)
	if err != nil {
		return nil, err
	}
	if order.UserID == nil || *order.UserID != userID {
		if order.CompanyID == nil {
			return nil, apperror.ErrForbidden
		}
		if _, err := u.companies.FindMember(ctx, *order.CompanyID, userID); err != nil {
			return nil, apperror.ErrForbidden
		}
	}
	if order.Status != domain.OrderPaid {
		return nil, apperror.ErrValidation.WithMessage("Only a paid order can be refunded")
	}

	amount := req.Amount
	if amount == 0 || amount > order.Amount {
		amount = order.Amount
	}

	refund := &domain.Refund{
		ID:             uuid.New(),
		PaymentOrderID: order.ID,
		Amount:         amount,
		Currency:       order.Currency,
		Reason:         req.Reason,
		Status:         "pending",
		RequestedBy:    &userID,
	}
	if err := u.payments.CreateRefund(ctx, refund); err != nil {
		return nil, err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &userID,
		Action:       "refund.requested",
		ResourceType: "refund",
		ResourceID:   &refund.ID,
		NewValues:    map[string]any{"order_id": order.ID, "amount": amount},
	})
	return refundResponse(refund), nil
}

// ReviewRefund is the admin decision. An approval credits the wallet rather than
// reversing the card payment: the money reaches the user either way, and a
// wallet credit is immediate and reversible.
func (u *PaymentUsecase) ReviewRefund(ctx context.Context, adminID, refundID uuid.UUID, req dto.ReviewRefundRequest) error {
	refund, err := u.payments.FindRefundByID(ctx, refundID)
	if err != nil {
		return err
	}
	if refund.Status != "pending" {
		return apperror.ErrConflict.WithMessage("This refund has already been reviewed")
	}

	if !req.Approve {
		return u.payments.UpdateRefundColumns(ctx, refundID, map[string]any{
			"status": "rejected", "approved_by": adminID, "processed_at": time.Now().UTC(),
		})
	}

	order, err := u.payments.FindOrderByID(ctx, refund.PaymentOrderID)
	if err != nil {
		return err
	}

	ownerType := domain.WalletOwnerUser
	ownerID := order.UserID
	if order.CompanyID != nil {
		ownerType = domain.WalletOwnerCompany
		ownerID = order.CompanyID
	}
	if ownerID == nil {
		return apperror.ErrValidation.WithMessage("This order has no owner to refund")
	}

	wallet, err := u.wallets.EnsureWallet(ctx, ownerType, ownerID, refund.Currency)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.payments.UpdateRefundColumns(txCtx, refundID, map[string]any{
			"status": "processed", "approved_by": adminID, "processed_at": now,
		}); err != nil {
			return err
		}
		if err := u.payments.UpdateOrderColumns(txCtx, order.ID, map[string]any{
			"status": domain.OrderRefunded,
		}); err != nil {
			return err
		}
		return u.walletUC.Credit(txCtx, wallet.ID, refund.Amount,
			domain.TxRefund, "refund", &refund.ID, "refund:"+refund.ID.String())
	})
	if err != nil {
		return err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &adminID,
		Action:       "refund.approved",
		ResourceType: "refund",
		ResourceID:   &refundID,
		NewValues:    map[string]any{"amount": refund.Amount, "note": req.Note},
	})
	if order.UserID != nil {
		u.notify(ctx, *order.UserID, "Refund processed",
			fmt.Sprintf("%s has been returned to your wallet.",
				formatAmount(refund.Amount, refund.Currency)),
			map[string]any{"refund_id": refundID.String()})
	}
	return nil
}

// ListRefunds serves the admin queue.
func (u *PaymentUsecase) ListRefunds(ctx context.Context, status string, offset, limit int) ([]dto.RefundResponse, int64, error) {
	refunds, total, err := u.payments.ListRefunds(ctx, status, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.RefundResponse, 0, len(refunds))
	for i := range refunds {
		out = append(out, *refundResponse(&refunds[i]))
	}
	return out, total, nil
}

// ─── listings & jobs ────────────────────────────────────────────────

// ListOrders returns the caller's payment history.
func (u *PaymentUsecase) ListOrders(ctx context.Context, userID, companyID *uuid.UUID, offset, limit int) ([]dto.OrderResponse, int64, error) {
	orders, total, err := u.payments.ListOrders(ctx, userID, companyID, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.OrderResponse, 0, len(orders))
	for i := range orders {
		out = append(out, orderResponse(&orders[i]))
	}
	return out, total, nil
}

// ListInvoices returns the caller's billing documents.
func (u *PaymentUsecase) ListInvoices(ctx context.Context, companyID, userID *uuid.UUID, offset, limit int) ([]dto.InvoiceResponse, int64, error) {
	invoices, total, err := u.payments.ListInvoices(ctx, companyID, userID, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]dto.InvoiceResponse, 0, len(invoices))
	for i := range invoices {
		out = append(out, invoiceResponse(&invoices[i]))
	}
	return out, total, nil
}

// AvailableProviders lists the configured checkout methods.
func (u *PaymentUsecase) AvailableProviders() dto.PaymentMethodsResponse {
	providers := u.gateways.Enabled()
	// Wallet is always offered: it needs no provider credentials.
	return dto.PaymentMethodsResponse{Providers: append(providers, string(domain.ProviderWallet))}
}

// ExpireStale is the scheduled sweep for abandoned checkouts and lapsed plans.
func (u *PaymentUsecase) ExpireStale(ctx context.Context) (orders int64, subscriptions int64, err error) {
	orders, err = u.payments.ExpireStaleOrders(ctx, time.Now().UTC().Add(-checkoutTTL))
	if err != nil {
		return 0, 0, err
	}
	subscriptions, err = u.subscriptions.ExpireOverdue(ctx, time.Now().UTC())
	if err != nil {
		return orders, 0, err
	}
	return orders, subscriptions, nil
}

// ─── internals ──────────────────────────────────────────────────────

// resolveOrder finds the order a callback refers to, by our id when the provider
// echoed it and by their reference otherwise.
func (u *PaymentUsecase) resolveOrder(ctx context.Context, provider domain.PaymentProvider, result *domain.WebhookResult) (*domain.PaymentOrder, error) {
	if result.OrderID != nil {
		order, err := u.payments.FindOrderByID(ctx, *result.OrderID)
		if err == nil {
			return order, nil
		}
		if !apperror.Is(err, apperror.ErrNotFound.Code) {
			return nil, err
		}
	}
	if result.ProviderOrderID != "" {
		return u.payments.FindOrderByProviderRef(ctx, provider, result.ProviderOrderID)
	}
	return nil, nil
}

func (u *PaymentUsecase) currentSubscription(ctx context.Context, userID, companyID *uuid.UUID) (*domain.Subscription, error) {
	if companyID != nil {
		return u.subscriptions.ActiveForCompany(ctx, *companyID)
	}
	if userID != nil {
		return u.subscriptions.ActiveForUser(ctx, *userID)
	}
	return nil, nil
}

func (u *PaymentUsecase) notify(ctx context.Context, userID uuid.UUID, title, body string, data map[string]any) {
	if u.notifier == nil {
		return
	}
	if err := u.notifier.Notify(ctx, domain.Notification{
		UserID: userID, Type: "payment", Title: title, Body: body, Data: data,
	}); err != nil {
		u.log.Warn("queueing payment notification failed", logger.Err(err))
	}
}

func subscriptionPlan(s *domain.Subscription) *domain.SubscriptionPlan {
	if s == nil {
		return nil
	}
	return s.Plan
}

func ownerKey(userID, companyID *uuid.UUID) string {
	if companyID != nil {
		return "company:" + companyID.String()
	}
	if userID != nil {
		return "user:" + userID.String()
	}
	return "unknown"
}
