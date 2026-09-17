package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/loadify/backend/internal/delivery/http/middleware"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/internal/usecase"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/response"
)

// DashboardHandler exposes the distribution dashboard and the plan's usage.
type DashboardHandler struct {
	dashboard *usecase.DashboardUsecase
}

// NewDashboardHandler builds the handler.
func NewDashboardHandler(dashboard *usecase.DashboardUsecase) *DashboardHandler {
	return &DashboardHandler{dashboard: dashboard}
}

// Get reads today's and this month's numbers. A manager sees the company by
// default and may ask for scope=mine; anyone else sees their own.
// GET /api/v1/dashboard
func (h *DashboardHandler) Get(c *fiber.Ctx) error {
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}
	manager := claims.HasRole(domain.RoleCompanyManager)

	scope := c.Query("scope")
	switch scope {
	case "":
		if manager {
			scope = dto.DashboardScopeCompany
		} else {
			scope = dto.DashboardScopeMine
		}
	case dto.DashboardScopeMine:
	case dto.DashboardScopeCompany:
		if !manager {
			return apperror.ErrForbidden.WithMessage("Only a company manager can see the whole company's numbers.")
		}
	default:
		return apperror.ErrValidation.WithMessage("scope must be mine or company")
	}

	res, err := h.dashboard.Dashboard(c.UserContext(), userID, companyID, scope == dto.DashboardScopeCompany)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Usage reads what the company's plan allows this month and what is left.
// GET /api/v1/subscriptions/usage
func (h *DashboardHandler) Usage(c *fiber.Ctx) error {
	_, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	res, err := h.dashboard.Usage(c.UserContext(), companyID)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}
