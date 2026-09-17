package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/internal/usecase"
	"github.com/loadify/backend/pkg/pagination"
	"github.com/loadify/backend/pkg/response"
	"github.com/loadify/backend/pkg/validator"
)

// LoadAdHandler exposes the freight advertisements a company distributes to
// Telegram groups.
type LoadAdHandler struct {
	ads       *usecase.LoadAdUsecase
	validator validator.Validator
}

// NewLoadAdHandler builds the advertisement handler.
func NewLoadAdHandler(ads *usecase.LoadAdUsecase, v validator.Validator) *LoadAdHandler {
	return &LoadAdHandler{ads: ads, validator: v}
}

// Parse reads an advertisement as it was written and returns what it understood.
// It stores no advert, so a person can try a sentence and see the reading
// before committing to it.
// POST /api/v1/load-ads/parse
func (h *LoadAdHandler) Parse(c *fiber.Ctx) error {
	var req dto.ParseAdRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	res, err := h.ads.Parse(c.UserContext(), userID, companyID, req.Text)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Create stores an advertisement the caller has confirmed.
// POST /api/v1/load-ads
func (h *LoadAdHandler) Create(c *fiber.Ctx) error {
	var req dto.SaveLoadAdRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	res, err := h.ads.Create(c.UserContext(), userID, companyID, req)
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// Update edits an advertisement.
// PUT /api/v1/load-ads/:id
func (h *LoadAdHandler) Update(c *fiber.Ctx) error {
	var req dto.SaveLoadAdRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	adID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.ads.Update(c.UserContext(), userID, companyID, adID, req)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// List returns the company's advertisements.
// GET /api/v1/load-ads
func (h *LoadAdHandler) List(c *fiber.Ctx) error {
	var q dto.LoadAdQuery
	if err := bindQuery(c, &q, h.validator); err != nil {
		return err
	}
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	page := pagination.Parse(c)

	items, total, err := h.ads.List(c.UserContext(), userID, companyID, q, page.Offset(), page.Limit())
	if err != nil {
		return err
	}
	return paginated(c, items, total, page)
}

// Get returns one advertisement.
// GET /api/v1/load-ads/:id
func (h *LoadAdHandler) Get(c *fiber.Ctx) error {
	_, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	adID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.ads.Get(c.UserContext(), companyID, adID)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Archive retires an advertisement.
// DELETE /api/v1/load-ads/:id
func (h *LoadAdHandler) Archive(c *fiber.Ctx) error {
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	adID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.ads.Archive(c.UserContext(), userID, companyID, adID); err != nil {
		return err
	}
	return response.NoContent(c)
}
