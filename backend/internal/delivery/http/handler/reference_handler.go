package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/usecase"
	"github.com/loadify/backend/pkg/response"
)

// ReferenceHandler exposes the static vocabularies clients need to render forms.
type ReferenceHandler struct {
	reference *usecase.ReferenceUsecase
}

// NewReferenceHandler builds the reference handler.
func NewReferenceHandler(reference *usecase.ReferenceUsecase) *ReferenceHandler {
	return &ReferenceHandler{reference: reference}
}

// Countries lists the markets the platform serves.
// GET /api/v1/reference/countries
func (h *ReferenceHandler) Countries(c *fiber.Ctx) error {
	res, err := h.reference.Countries(c.UserContext(), requestLocale(c))
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Regions lists administrative divisions.
// GET /api/v1/reference/regions?country=UZ
func (h *ReferenceHandler) Regions(c *fiber.Ctx) error {
	res, err := h.reference.Regions(c.UserContext(), c.Query("country"), requestLocale(c))
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Cities lists settlements, filtered by region or a search term.
// GET /api/v1/reference/cities?region_id=...&q=tash
func (h *ReferenceHandler) Cities(c *fiber.Ctx) error {
	var regionID *uuid.UUID
	if raw := c.Query("region_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err == nil {
			regionID = &id
		}
	}
	res, err := h.reference.Cities(c.UserContext(), regionID, c.Query("q"), requestLocale(c))
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// VehicleTypes lists the truck-type vocabulary.
// GET /api/v1/reference/vehicle-types
func (h *ReferenceHandler) VehicleTypes(c *fiber.Ctx) error {
	return response.OK(c, h.reference.VehicleTypes(requestLocale(c)))
}

// requestLocale reads the language the locale middleware resolved.
func requestLocale(c *fiber.Ctx) domain.Locale {
	if v, ok := c.Locals("locale").(string); ok && v != "" {
		return domain.Locale(v)
	}
	return domain.LocaleUZ
}
