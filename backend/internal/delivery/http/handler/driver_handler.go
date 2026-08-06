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

// driverSortColumns is the allowlist for ?sort= on driver search. Anything not
// listed falls back to the default order — user input never becomes SQL.
var driverSortColumns = map[string]string{
	"rating":     "drivers.rating",
	"trips":      "drivers.completed_trips",
	"created_at": "drivers.created_at",
	"distance":   "distance_m",
}

// DriverHandler exposes driver, document and vehicle endpoints.
type DriverHandler struct {
	drivers   *usecase.DriverUsecase
	validator validator.Validator
}

// NewDriverHandler builds the driver handler.
func NewDriverHandler(drivers *usecase.DriverUsecase, v validator.Validator) *DriverHandler {
	return &DriverHandler{drivers: drivers, validator: v}
}

// SaveProfile creates or updates the caller's driver profile.
// PUT /api/v1/drivers/me
func (h *DriverHandler) SaveProfile(c *fiber.Ctx) error {
	var req dto.CreateDriverProfileRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	res, err := h.drivers.SaveProfile(c.UserContext(), userID, req)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// GetMyProfile returns the caller's driver profile.
// GET /api/v1/drivers/me
func (h *DriverHandler) GetMyProfile(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	res, err := h.drivers.GetMyProfile(c.UserContext(), userID)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// GetProfile returns a driver's public profile.
// GET /api/v1/drivers/:id
func (h *DriverHandler) GetProfile(c *fiber.Ctx) error {
	driverID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	// Only company-side users and admins see contact details.
	claims := middleware.Claims(c)
	includePrivate := claims != nil && (claims.HasPermission("drivers.search") || claims.HasPermission("drivers.verify"))
	res, err := h.drivers.GetProfile(c.UserContext(), driverID, includePrivate)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Search finds drivers, optionally near a point.
// GET /api/v1/drivers
func (h *DriverHandler) Search(c *fiber.Ctx) error {
	var q dto.DriverSearchQuery
	if err := bindQuery(c, &q, h.validator); err != nil {
		return err
	}
	page := pagination.Parse(c)
	claims := middleware.Claims(c)
	includePrivate := claims != nil && claims.HasPermission("drivers.search")

	items, total, err := h.drivers.Search(c.UserContext(), q,
		page.Offset(), page.Limit(),
		page.OrderClause(driverSortColumns, ""), includePrivate)
	if err != nil {
		return err
	}
	return response.Paginated(c, items, response.Meta{
		Page:       page.Page,
		PerPage:    page.PerPage,
		Total:      total,
		TotalPages: pagination.TotalPages(total, page.PerPage),
	})
}

// SetStatus toggles the caller's availability.
// PATCH /api/v1/drivers/me/status
func (h *DriverHandler) SetStatus(c *fiber.Ctx) error {
	var req dto.UpdateDriverStatusRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	if err := h.drivers.SetStatus(c.UserContext(), userID, req); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"status": req.Status})
}

// UpdateLocation records a position report.
// POST /api/v1/drivers/me/location
func (h *DriverHandler) UpdateLocation(c *fiber.Ctx) error {
	var req dto.UpdateDriverLocationRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	if err := h.drivers.UpdateLocation(c.UserContext(), userID, req); err != nil {
		return err
	}
	return response.NoContent(c)
}

// ─── documents ──────────────────────────────────────────────────────

// UploadDocument attaches a document to the caller's driver profile.
// POST /api/v1/drivers/me/documents
func (h *DriverHandler) UploadDocument(c *fiber.Ctx) error {
	var req dto.UploadDocumentRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	res, err := h.drivers.UploadDocument(c.UserContext(), userID, req)
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// ListDocuments returns the caller's documents.
// GET /api/v1/drivers/me/documents
func (h *DriverHandler) ListDocuments(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	res, err := h.drivers.ListDocuments(c.UserContext(), userID)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// ReviewDocument records an admin decision on a driver document.
// POST /api/v1/admin/driver-documents/:id/review
func (h *DriverHandler) ReviewDocument(c *fiber.Ctx) error {
	var req dto.ReviewDocumentRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	adminID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	docID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.drivers.ReviewDocument(c.UserContext(), adminID, docID, req); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"status": req.Status})
}

// PendingDocuments serves the admin verification queue.
// GET /api/v1/admin/driver-documents/pending
func (h *DriverHandler) PendingDocuments(c *fiber.Ctx) error {
	page := pagination.Parse(c)
	items, total, err := h.drivers.ListPendingDocuments(c.UserContext(), page.Offset(), page.Limit())
	if err != nil {
		return err
	}
	return response.Paginated(c, items, response.Meta{
		Page:       page.Page,
		PerPage:    page.PerPage,
		Total:      total,
		TotalPages: pagination.TotalPages(total, page.PerPage),
	})
}

// ─── vehicles ───────────────────────────────────────────────────────

// AddVehicle registers a truck.
// POST /api/v1/drivers/me/vehicles
func (h *DriverHandler) AddVehicle(c *fiber.Ctx) error {
	var req dto.SaveVehicleRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	res, err := h.drivers.AddVehicle(c.UserContext(), userID, req)
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// UpdateVehicle edits a truck.
// PUT /api/v1/drivers/me/vehicles/:id
func (h *DriverHandler) UpdateVehicle(c *fiber.Ctx) error {
	var req dto.SaveVehicleRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	vehicleID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.drivers.UpdateVehicle(c.UserContext(), userID, vehicleID, req)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// ListVehicles returns the caller's trucks.
// GET /api/v1/drivers/me/vehicles
func (h *DriverHandler) ListVehicles(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	res, err := h.drivers.ListVehicles(c.UserContext(), userID)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// SetPrimaryVehicle marks the default truck.
// POST /api/v1/drivers/me/vehicles/:id/primary
func (h *DriverHandler) SetPrimaryVehicle(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	vehicleID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.drivers.SetPrimaryVehicle(c.UserContext(), userID, vehicleID); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"message": "Primary vehicle updated"})
}

// DeleteVehicle removes a truck.
// DELETE /api/v1/drivers/me/vehicles/:id
func (h *DriverHandler) DeleteVehicle(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	vehicleID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.drivers.DeleteVehicle(c.UserContext(), userID, vehicleID); err != nil {
		return err
	}
	return response.NoContent(c)
}

// ReviewVehicle records an admin decision on a truck.
// POST /api/v1/admin/vehicles/:id/review
func (h *DriverHandler) ReviewVehicle(c *fiber.Ctx) error {
	var req dto.ReviewDocumentRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	adminID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	vehicleID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.drivers.ReviewVehicle(c.UserContext(), adminID, vehicleID, req); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"status": req.Status})
}

// PendingVehicles serves the admin vehicle queue.
// GET /api/v1/admin/vehicles/pending
func (h *DriverHandler) PendingVehicles(c *fiber.Ctx) error {
	page := pagination.Parse(c)
	items, total, err := h.drivers.ListPendingVehicles(c.UserContext(), page.Offset(), page.Limit())
	if err != nil {
		return err
	}
	return response.Paginated(c, items, response.Meta{
		Page:       page.Page,
		PerPage:    page.PerPage,
		Total:      total,
		TotalPages: pagination.TotalPages(total, page.PerPage),
	})
}

// ─── availability & preferences ─────────────────────────────────────

// PublishAvailability advertises free capacity.
// POST /api/v1/drivers/me/availability
func (h *DriverHandler) PublishAvailability(c *fiber.Ctx) error {
	var req dto.PublishAvailabilityRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	res, err := h.drivers.PublishAvailability(c.UserContext(), userID, req)
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// ListAvailability returns the caller's published windows.
// GET /api/v1/drivers/me/availability
func (h *DriverHandler) ListAvailability(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	res, err := h.drivers.ListAvailability(c.UserContext(), userID, c.QueryBool("active_only", true))
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// WithdrawAvailability deactivates one window.
// DELETE /api/v1/drivers/me/availability/:id
func (h *DriverHandler) WithdrawAvailability(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	availabilityID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.drivers.WithdrawAvailability(c.UserContext(), userID, availabilityID); err != nil {
		return err
	}
	return response.NoContent(c)
}

// GetPreferences returns the caller's matching preferences.
// GET /api/v1/drivers/me/preferences
func (h *DriverHandler) GetPreferences(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	res, err := h.drivers.GetPreferences(c.UserContext(), userID)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// SavePreferences stores the caller's matching preferences.
// PUT /api/v1/drivers/me/preferences
func (h *DriverHandler) SavePreferences(c *fiber.Ctx) error {
	var req dto.SavePreferencesRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	res, err := h.drivers.SavePreferences(c.UserContext(), userID, req)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}
