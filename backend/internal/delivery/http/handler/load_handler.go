package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/loadify/backend/internal/delivery/http/middleware"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/internal/usecase"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/pagination"
	"github.com/loadify/backend/pkg/response"
	"github.com/loadify/backend/pkg/validator"
)

// loadSortColumns is the allowlist for ?sort= on the marketplace feed.
var loadSortColumns = map[string]string{
	"published_at": "loads.published_at",
	"created_at":   "loads.created_at",
	"price":        "loads.price_amount",
	"weight":       "loads.weight_kg",
	"pickup_from":  "loads.pickup_from",
	"distance":     "pickup_distance_m",
	"views":        "loads.views_count",
}

// LoadHandler exposes the shipment endpoints.
type LoadHandler struct {
	loads     *usecase.LoadUsecase
	validator validator.Validator
}

// NewLoadHandler builds the load handler.
func NewLoadHandler(loads *usecase.LoadUsecase, v validator.Validator) *LoadHandler {
	return &LoadHandler{loads: loads, validator: v}
}

// Search runs the public marketplace query.
// GET /api/v1/loads
func (h *LoadHandler) Search(c *fiber.Ctx) error {
	var q dto.LoadSearchQuery
	if err := bindQuery(c, &q, h.validator); err != nil {
		return err
	}
	page := pagination.Parse(c)

	items, total, err := h.loads.Search(c.UserContext(), q, viewerFrom(c),
		page.Offset(), page.Limit(), page.OrderClause(loadSortColumns, ""))
	if err != nil {
		return err
	}
	return paginated(c, items, total, page)
}

// Get returns one load and counts the view.
// GET /api/v1/loads/:id
func (h *LoadHandler) Get(c *fiber.Ctx) error {
	loadID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.loads.Get(c.UserContext(), loadID, viewerFrom(c), true)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Create saves a shipment for the caller's company.
// POST /api/v1/loads
func (h *LoadHandler) Create(c *fiber.Ctx) error {
	var req dto.SaveLoadRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	res, err := h.loads.Create(c.UserContext(), userID, companyID, req)
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// Update edits a shipment.
// PUT /api/v1/loads/:id
func (h *LoadHandler) Update(c *fiber.Ctx) error {
	var req dto.SaveLoadRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	loadID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.loads.Update(c.UserContext(), userID, companyID, loadID, req)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Publish puts a load on the marketplace.
// POST /api/v1/loads/:id/publish
func (h *LoadHandler) Publish(c *fiber.Ctx) error {
	var req dto.PublishLoadRequest
	if err := bindOptional(c, &req, h.validator); err != nil {
		return err
	}
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	loadID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.loads.Publish(c.UserContext(), userID, companyID, loadID, req)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Unpublish pulls a load back to draft.
// POST /api/v1/loads/:id/unpublish
func (h *LoadHandler) Unpublish(c *fiber.Ctx) error {
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	loadID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.loads.Unpublish(c.UserContext(), userID, companyID, loadID); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"message": "Load unpublished"})
}

// Cancel closes a load and its open bookings.
// POST /api/v1/loads/:id/cancel
func (h *LoadHandler) Cancel(c *fiber.Ctx) error {
	var req dto.CancelLoadRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	loadID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.loads.Cancel(c.UserContext(), userID, companyID, loadID, req); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"message": "Load cancelled"})
}

// Delete removes a draft.
// DELETE /api/v1/loads/:id
func (h *LoadHandler) Delete(c *fiber.Ctx) error {
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	loadID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.loads.Delete(c.UserContext(), userID, companyID, loadID); err != nil {
		return err
	}
	return response.NoContent(c)
}

// ListMine returns the caller company's loads, drafts included.
// GET /api/v1/loads/my
func (h *LoadHandler) ListMine(c *fiber.Ctx) error {
	var q dto.LoadSearchQuery
	if err := bindQuery(c, &q, h.validator); err != nil {
		return err
	}
	_, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	page := pagination.Parse(c)

	items, total, err := h.loads.ListCompanyLoads(c.UserContext(), companyID, q, viewerFrom(c),
		page.Offset(), page.Limit(), page.OrderClause(loadSortColumns, ""))
	if err != nil {
		return err
	}
	return paginated(c, items, total, page)
}

// ─── bookmarks ──────────────────────────────────────────────────────

// Save bookmarks a load.
// POST /api/v1/loads/:id/save
func (h *LoadHandler) Save(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	loadID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.loads.SaveLoad(c.UserContext(), userID, loadID); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"saved": true})
}

// Unsave removes a bookmark.
// DELETE /api/v1/loads/:id/save
func (h *LoadHandler) Unsave(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	loadID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.loads.UnsaveLoad(c.UserContext(), userID, loadID); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"saved": false})
}

// ListSaved returns the caller's bookmarks.
// GET /api/v1/loads/saved
func (h *LoadHandler) ListSaved(c *fiber.Ctx) error {
	page := pagination.Parse(c)
	items, total, err := h.loads.ListSavedLoads(c.UserContext(), viewerFrom(c), page.Offset(), page.Limit())
	if err != nil {
		return err
	}
	return paginated(c, items, total, page)
}

// ─── saved searches ─────────────────────────────────────────────────

// CreateSavedSearch stores a reusable filter.
// POST /api/v1/saved-searches
func (h *LoadHandler) CreateSavedSearch(c *fiber.Ctx) error {
	var req dto.SaveSearchRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	res, err := h.loads.CreateSavedSearch(c.UserContext(), userID, req)
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// ListSavedSearches returns the caller's stored filters.
// GET /api/v1/saved-searches
func (h *LoadHandler) ListSavedSearches(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	res, err := h.loads.ListSavedSearches(c.UserContext(), userID)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// DeleteSavedSearch removes one stored filter.
// DELETE /api/v1/saved-searches/:id
func (h *LoadHandler) DeleteSavedSearch(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	searchID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.loads.DeleteSavedSearch(c.UserContext(), userID, searchID); err != nil {
		return err
	}
	return response.NoContent(c)
}

// CargoTypes returns the localised cargo vocabulary.
// GET /api/v1/reference/cargo-types
func (h *LoadHandler) CargoTypes(c *fiber.Ctx) error {
	res, err := h.loads.CargoTypes(c.UserContext(), requestLocale(c))
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// ─── shared helpers ─────────────────────────────────────────────────

// viewerFrom builds the visibility context for the current request. An
// unauthenticated request yields a viewer with no entitlements at all.
func viewerFrom(c *fiber.Ctx) usecase.Viewer {
	claims := middleware.Claims(c)
	if claims == nil {
		return usecase.Viewer{}
	}
	return usecase.Viewer{
		UserID:        claims.UserID,
		CompanyID:     claims.CompanyID,
		DriverID:      claims.DriverID,
		IsAdmin:       isAdmin(claims.Roles),
		Authenticated: true,
	}
}

// actorAndCompany extracts the caller and the company they act for.
func actorAndCompany(c *fiber.Ctx) (uuid.UUID, uuid.UUID, error) {
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if claims.CompanyID == nil {
		return uuid.Nil, uuid.Nil, apperror.ErrForbidden.WithMessage("Create or join a company first")
	}
	return claims.UserID, *claims.CompanyID, nil
}

// paginated writes a page of results with its meta block.
func paginated[T any](c *fiber.Ctx, items []T, total int64, page pagination.Request) error {
	return response.Paginated(c, items, response.Meta{
		Page:       page.Page,
		PerPage:    page.PerPage,
		Total:      total,
		TotalPages: pagination.TotalPages(total, page.PerPage),
	})
}
