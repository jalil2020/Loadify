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

// tripSortColumns is the allowlist for ?sort= on trip listings.
var tripSortColumns = map[string]string{
	"created_at":   "created_at",
	"started_at":   "started_at",
	"delivered_at": "delivered_at",
	"price":        "agreed_price_amount",
}

// TripHandler exposes the trip endpoints.
type TripHandler struct {
	trips     *usecase.TripUsecase
	validator validator.Validator
}

// NewTripHandler builds the trip handler.
func NewTripHandler(trips *usecase.TripUsecase, v validator.Validator) *TripHandler {
	return &TripHandler{trips: trips, validator: v}
}

// Advance moves a trip to its next status.
// POST /api/v1/trips/:id/advance
func (h *TripHandler) Advance(c *fiber.Ctx) error {
	var req dto.AdvanceTripRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	tripID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.trips.Advance(c.UserContext(), userID, tripID, req)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Confirm is the shipper accepting delivery.
// POST /api/v1/trips/:id/confirm
func (h *TripHandler) Confirm(c *fiber.Ctx) error {
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	tripID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.trips.Confirm(c.UserContext(), userID, companyID, tripID)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Cancel aborts a trip before the cargo is loaded.
// POST /api/v1/trips/:id/cancel
func (h *TripHandler) Cancel(c *fiber.Ctx) error {
	var req dto.CancelTripRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}
	tripID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.trips.Cancel(c.UserContext(), claims.UserID, tripID, req, isAdmin(claims.Roles)); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"message": "Trip cancelled"})
}

// Dispute freezes a trip for admin resolution.
// POST /api/v1/trips/:id/dispute
func (h *TripHandler) Dispute(c *fiber.Ctx) error {
	var req dto.CancelTripRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	tripID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.trips.Dispute(c.UserContext(), userID, tripID, req.Reason); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"message": "Dispute opened"})
}

// Get returns one trip.
// GET /api/v1/trips/:id
func (h *TripHandler) Get(c *fiber.Ctx) error {
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}
	tripID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.trips.Get(c.UserContext(), tripID, claims.UserID, isAdmin(claims.Roles))
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// ListMine returns the caller's trips, from whichever side they are on.
// GET /api/v1/trips
func (h *TripHandler) ListMine(c *fiber.Ctx) error {
	var q dto.TripSearchQuery
	if err := bindQuery(c, &q, h.validator); err != nil {
		return err
	}
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}
	page := pagination.Parse(c)
	order := page.OrderClause(tripSortColumns, "")

	if claims.CompanyID != nil {
		items, total, err := h.trips.ListForCompany(c.UserContext(), *claims.CompanyID, q,
			page.Offset(), page.Limit(), order)
		if err != nil {
			return err
		}
		return paginated(c, items, total, page)
	}

	items, total, err := h.trips.ListForDriver(c.UserContext(), claims.UserID, q,
		page.Offset(), page.Limit(), order)
	if err != nil {
		return err
	}
	return paginated(c, items, total, page)
}

// Track returns the live position and travelled route.
// GET /api/v1/trips/:id/track
func (h *TripHandler) Track(c *fiber.Ctx) error {
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}
	tripID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.trips.Track(c.UserContext(), claims.UserID, tripID, isAdmin(claims.Roles))
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// AddDocument attaches a paper to a trip.
// POST /api/v1/trips/:id/documents
func (h *TripHandler) AddDocument(c *fiber.Ctx) error {
	var req dto.AddTripDocumentRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	tripID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.trips.AddDocument(c.UserContext(), userID, tripID, req)
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// Rate scores the counterparty after a completed trip.
// POST /api/v1/trips/:id/rate
func (h *TripHandler) Rate(c *fiber.Ctx) error {
	var req dto.RateTripRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	tripID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.trips.Rate(c.UserContext(), userID, tripID, req)
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// DriverRatings returns the public reviews of a driver.
// GET /api/v1/drivers/:id/ratings
func (h *TripHandler) DriverRatings(c *fiber.Ctx) error {
	driverID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	page := pagination.Parse(c)
	items, total, err := h.trips.ListDriverRatings(c.UserContext(), driverID, page.Offset(), page.Limit())
	if err != nil {
		return err
	}
	return paginated(c, items, total, page)
}

// CompanyRatings returns the public reviews of a company.
// GET /api/v1/companies/:id/ratings
func (h *TripHandler) CompanyRatings(c *fiber.Ctx) error {
	companyID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	page := pagination.Parse(c)
	items, total, err := h.trips.ListCompanyRatings(c.UserContext(), companyID, page.Offset(), page.Limit())
	if err != nil {
		return err
	}
	return paginated(c, items, total, page)
}
