package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/loadify/backend/internal/delivery/http/middleware"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/internal/usecase"
	"github.com/loadify/backend/pkg/pagination"
	"github.com/loadify/backend/pkg/response"
	"github.com/loadify/backend/pkg/validator"
)

// bookingSortColumns is the allowlist for ?sort= on booking listings.
var bookingSortColumns = map[string]string{
	"created_at": "created_at",
	"expires_at": "expires_at",
	"price":      "COALESCE(agreed_price_amount, offered_price_amount)",
}

// BookingHandler exposes the booking endpoints.
type BookingHandler struct {
	bookings  *usecase.BookingUsecase
	validator validator.Validator
}

// NewBookingHandler builds the booking handler.
func NewBookingHandler(bookings *usecase.BookingUsecase, v validator.Validator) *BookingHandler {
	return &BookingHandler{bookings: bookings, validator: v}
}

// Reserve is a driver claiming a load.
// POST /api/v1/loads/:id/reserve
func (h *BookingHandler) Reserve(c *fiber.Ctx) error {
	var req dto.ReserveLoadRequest
	if err := bindOptional(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	loadID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.bookings.Reserve(c.UserContext(), userID, loadID, req)
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// Offer is a company proposing a load to a driver.
// POST /api/v1/bookings/offer
func (h *BookingHandler) Offer(c *fiber.Ctx) error {
	var req dto.OfferLoadRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	res, err := h.bookings.Offer(c.UserContext(), userID, companyID, req)
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// Respond accepts or rejects a booking.
// POST /api/v1/bookings/:id/respond
func (h *BookingHandler) Respond(c *fiber.Ctx) error {
	var req dto.RespondBookingRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	bookingID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.bookings.Respond(c.UserContext(), userID, bookingID, req)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Withdraw pulls back an open booking the caller started.
// POST /api/v1/bookings/:id/withdraw
func (h *BookingHandler) Withdraw(c *fiber.Ctx) error {
	var req dto.CancelBookingRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	bookingID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.bookings.Withdraw(c.UserContext(), userID, bookingID, req); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"message": "Booking withdrawn"})
}

// Cancel closes an accepted booking before loading.
// POST /api/v1/bookings/:id/cancel
func (h *BookingHandler) Cancel(c *fiber.Ctx) error {
	var req dto.CancelBookingRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	bookingID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.bookings.Cancel(c.UserContext(), userID, bookingID, req); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"message": "Booking cancelled"})
}

// Get returns one booking.
// GET /api/v1/bookings/:id
func (h *BookingHandler) Get(c *fiber.Ctx) error {
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}
	bookingID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.bookings.Get(c.UserContext(), bookingID, claims.UserID, isAdmin(claims.Roles))
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// ListMine returns the caller's bookings, from whichever side they are on.
// GET /api/v1/bookings
func (h *BookingHandler) ListMine(c *fiber.Ctx) error {
	var q dto.BookingSearchQuery
	if err := bindQuery(c, &q, h.validator); err != nil {
		return err
	}
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}
	page := pagination.Parse(c)
	order := page.OrderClause(bookingSortColumns, "")

	// A user acting for a company sees the company's side; otherwise their own
	// driver bookings.
	if claims.CompanyID != nil {
		items, total, err := h.bookings.ListForCompany(c.UserContext(), *claims.CompanyID, q,
			page.Offset(), page.Limit(), order)
		if err != nil {
			return err
		}
		return paginated(c, items, total, page)
	}

	items, total, err := h.bookings.ListForDriver(c.UserContext(), claims.UserID, q,
		page.Offset(), page.Limit(), order)
	if err != nil {
		return err
	}
	return paginated(c, items, total, page)
}
