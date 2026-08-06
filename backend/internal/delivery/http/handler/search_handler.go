package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/internal/usecase"
	"github.com/loadify/backend/pkg/response"
	"github.com/loadify/backend/pkg/validator"
)

// SearchHandler turns a written request into filters.
type SearchHandler struct {
	search    *usecase.SearchUsecase
	validator validator.Validator
}

// NewSearchHandler builds the search handler.
func NewSearchHandler(search *usecase.SearchUsecase, v validator.Validator) *SearchHandler {
	return &SearchHandler{search: search, validator: v}
}

// Parse resolves a freight request written in Uzbek, Russian or English into
// the filters /loads or /drivers already accept.
//
// It returns the filters rather than the results. The client shows them as
// removable chips first, so a mis-parse is visible and correctable instead of
// silently changing what was searched — and load search keeps exactly one
// implementation rather than gaining a second behind a natural-language door.
//
// POST /api/v1/search/parse
func (h *SearchHandler) Parse(c *fiber.Ctx) error {
	var req dto.ParseSearchRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}

	res, err := h.search.Parse(c.UserContext(), req, requestLocale(c))
	if err != nil {
		return err
	}
	return response.OK(c, res)
}
