package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/loadify/backend/internal/delivery/http/middleware"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/internal/usecase"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/pagination"
	"github.com/loadify/backend/pkg/response"
	"github.com/loadify/backend/pkg/validator"
)

// PaymentHandler exposes wallet, subscription and checkout endpoints.
type PaymentHandler struct {
	payments  *usecase.PaymentUsecase
	wallets   *usecase.WalletUsecase
	validator validator.Validator
}

// NewPaymentHandler builds the payment handler.
func NewPaymentHandler(payments *usecase.PaymentUsecase, wallets *usecase.WalletUsecase, v validator.Validator) *PaymentHandler {
	return &PaymentHandler{payments: payments, wallets: wallets, validator: v}
}

// ─── plans & subscriptions ──────────────────────────────────────────

// ListPlans returns the purchasable tiers.
// GET /api/v1/plans?target_role=driver
func (h *PaymentHandler) ListPlans(c *fiber.Ctx) error {
	targetRole := c.Query("target_role")
	claims := middleware.Claims(c)

	var userID, companyID *uuid.UUID
	if claims != nil {
		userID = &claims.UserID
		companyID = claims.CompanyID
		// Default to the tier that matches who is asking.
		if targetRole == "" {
			targetRole = "driver"
			if claims.CompanyID != nil {
				targetRole = "company"
			}
		}
	}

	res, err := h.payments.ListPlans(c.UserContext(), targetRole, requestLocale(c), userID, companyID)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// GetSubscription returns the caller's current plan.
// GET /api/v1/subscriptions/me
func (h *PaymentHandler) GetSubscription(c *fiber.Ctx) error {
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}
	res, err := h.payments.GetSubscription(c.UserContext(), &claims.UserID, claims.CompanyID, requestLocale(c))
	if err != nil {
		return err
	}
	// No paid plan is a normal state, not a 404: the client renders the free tier.
	if res == nil {
		return response.OK(c, fiber.Map{"subscription": nil})
	}
	return response.OK(c, res)
}

// Entitlements tells the client what the caller's plan allows.
// GET /api/v1/subscriptions/entitlements
func (h *PaymentHandler) Entitlements(c *fiber.Ctx) error {
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}
	targetRole := "driver"
	if claims.CompanyID != nil {
		targetRole = "company"
	}
	res, err := h.payments.Entitlements(c.UserContext(), targetRole,
		&claims.UserID, claims.CompanyID, requestLocale(c))
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Subscribe starts a checkout for a plan.
// POST /api/v1/subscriptions
func (h *PaymentHandler) Subscribe(c *fiber.Ctx) error {
	var req dto.SubscribeRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}
	res, err := h.payments.Subscribe(c.UserContext(), claims.UserID, claims.CompanyID, req)
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// CancelSubscription stops auto-renewal.
// POST /api/v1/subscriptions/cancel
func (h *PaymentHandler) CancelSubscription(c *fiber.Ctx) error {
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}
	if err := h.payments.CancelSubscription(c.UserContext(), claims.UserID,
		&claims.UserID, claims.CompanyID); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"message": "Auto-renewal turned off"})
}

// ─── wallet ─────────────────────────────────────────────────────────

// GetWallet returns the caller's balance.
// GET /api/v1/wallet
func (h *PaymentHandler) GetWallet(c *fiber.Ctx) error {
	ownerType, ownerID, err := walletOwner(c)
	if err != nil {
		return err
	}
	res, err := h.wallets.GetWallet(c.UserContext(), ownerType, ownerID)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// ListTransactions returns the caller's ledger history.
// GET /api/v1/wallet/transactions
func (h *PaymentHandler) ListTransactions(c *fiber.Ctx) error {
	ownerType, ownerID, err := walletOwner(c)
	if err != nil {
		return err
	}
	page := pagination.Parse(c)
	items, total, err := h.wallets.ListTransactions(c.UserContext(), ownerType, ownerID,
		page.Offset(), page.Limit())
	if err != nil {
		return err
	}
	return paginated(c, items, total, page)
}

// Topup starts a checkout that adds funds to the wallet.
// POST /api/v1/wallet/topup
func (h *PaymentHandler) Topup(c *fiber.Ctx) error {
	var req dto.TopupRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}
	res, err := h.payments.Topup(c.UserContext(), claims.UserID, claims.CompanyID, req)
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// RequestPayout withdraws funds to a card.
// POST /api/v1/wallet/payout
func (h *PaymentHandler) RequestPayout(c *fiber.Ctx) error {
	var req dto.PayoutRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	res, err := h.wallets.RequestPayout(c.UserContext(), userID, req)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// ─── escrow ─────────────────────────────────────────────────────────

// FundEscrow secures a trip's price.
// POST /api/v1/escrow/fund
func (h *PaymentHandler) FundEscrow(c *fiber.Ctx) error {
	var req dto.FundEscrowRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	res, err := h.wallets.FundEscrow(c.UserContext(), userID, companyID, req.TripID)
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// GetEscrow returns the escrow state of a trip.
// GET /api/v1/escrow/:trip_id
func (h *PaymentHandler) GetEscrow(c *fiber.Ctx) error {
	tripID, err := paramUUID(c, "trip_id")
	if err != nil {
		return err
	}
	res, err := h.wallets.GetEscrow(c.UserContext(), tripID)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// ─── orders, invoices, refunds ──────────────────────────────────────

// ListOrders returns the caller's payment history.
// GET /api/v1/payments/orders
func (h *PaymentHandler) ListOrders(c *fiber.Ctx) error {
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}
	page := pagination.Parse(c)
	items, total, err := h.payments.ListOrders(c.UserContext(), &claims.UserID, claims.CompanyID,
		page.Offset(), page.Limit())
	if err != nil {
		return err
	}
	return paginated(c, items, total, page)
}

// ListInvoices returns the caller's billing documents.
// GET /api/v1/payments/invoices
func (h *PaymentHandler) ListInvoices(c *fiber.Ctx) error {
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}
	page := pagination.Parse(c)
	items, total, err := h.payments.ListInvoices(c.UserContext(), claims.CompanyID, &claims.UserID,
		page.Offset(), page.Limit())
	if err != nil {
		return err
	}
	return paginated(c, items, total, page)
}

// RequestRefund asks for money back on an order.
// POST /api/v1/payments/refunds
func (h *PaymentHandler) RequestRefund(c *fiber.Ctx) error {
	var req dto.RequestRefundRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	res, err := h.payments.RequestRefund(c.UserContext(), userID, req)
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// ReviewRefund is the admin decision on a refund.
// POST /api/v1/admin/refunds/:id/review
func (h *PaymentHandler) ReviewRefund(c *fiber.Ctx) error {
	var req dto.ReviewRefundRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	adminID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	refundID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.payments.ReviewRefund(c.UserContext(), adminID, refundID, req); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"approved": req.Approve})
}

// ListRefunds serves the admin refund queue.
// GET /api/v1/admin/refunds
func (h *PaymentHandler) ListRefunds(c *fiber.Ctx) error {
	page := pagination.Parse(c)
	items, total, err := h.payments.ListRefunds(c.UserContext(), c.Query("status"),
		page.Offset(), page.Limit())
	if err != nil {
		return err
	}
	return paginated(c, items, total, page)
}

// PaymentMethods lists the configured checkout providers.
// GET /api/v1/payments/methods
func (h *PaymentHandler) PaymentMethods(c *fiber.Ctx) error {
	return response.OK(c, h.payments.AvailableProviders())
}

// ─── provider callbacks ─────────────────────────────────────────────

// Webhook receives a provider callback.
//
// This endpoint is deliberately unauthenticated — the provider has no token —
// and relies entirely on the per-provider signature check inside the usecase.
// It always answers 200 with a provider-shaped body, because a non-200 makes
// most providers retry indefinitely.
// POST /api/v1/payments/webhooks/:provider
func (h *PaymentHandler) Webhook(c *fiber.Ctx) error {
	provider := domain.PaymentProvider(c.Params("provider"))
	if provider == "" {
		return apperror.ErrBadRequest.WithMessage("Provider is required")
	}

	headers := make(map[string]string)
	c.Request().Header.VisitAll(func(key, value []byte) {
		headers[string(key)] = string(value)
	})

	body, err := h.payments.HandleWebhook(c.UserContext(), provider, headers, c.Body())
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusOK).JSON(body)
}

// walletOwner decides whose wallet the caller is acting on: a company member
// operates the company wallet, everyone else their own.
func walletOwner(c *fiber.Ctx) (domain.WalletOwnerType, uuid.UUID, error) {
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return "", uuid.Nil, err
	}
	if claims.CompanyID != nil {
		return domain.WalletOwnerCompany, *claims.CompanyID, nil
	}
	return domain.WalletOwnerUser, claims.UserID, nil
}
