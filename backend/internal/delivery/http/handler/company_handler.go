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

// companySortColumns is the allowlist for ?sort= on the company directory.
var companySortColumns = map[string]string{
	"rating":     "rating",
	"trips":      "completed_trips",
	"created_at": "created_at",
	"name":       "brand_name",
}

// CompanyHandler exposes company, membership and verification endpoints.
type CompanyHandler struct {
	companies *usecase.CompanyUsecase
	validator validator.Validator
}

// NewCompanyHandler builds the company handler.
func NewCompanyHandler(companies *usecase.CompanyUsecase, v validator.Validator) *CompanyHandler {
	return &CompanyHandler{companies: companies, validator: v}
}

// Create registers a company for the caller.
// POST /api/v1/companies
func (h *CompanyHandler) Create(c *fiber.Ctx) error {
	var req dto.CreateCompanyRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	res, err := h.companies.Create(c.UserContext(), userID, req)
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// GetMine returns the caller's company.
// GET /api/v1/companies/me
func (h *CompanyHandler) GetMine(c *fiber.Ctx) error {
	res, err := h.companies.GetMine(c.UserContext(), middleware.CompanyID(c))
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Update edits the caller's company.
// PUT /api/v1/companies/me
func (h *CompanyHandler) Update(c *fiber.Ctx) error {
	var req dto.UpdateCompanyRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, companyID, err := h.actorAndCompany(c)
	if err != nil {
		return err
	}
	res, err := h.companies.Update(c.UserContext(), userID, companyID, req)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Get returns a company's public profile.
// GET /api/v1/companies/:id
func (h *CompanyHandler) Get(c *fiber.Ctx) error {
	companyID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	claims := middleware.Claims(c)
	// A member of the company, or an admin, sees the private fields.
	includePrivate := claims != nil &&
		((claims.CompanyID != nil && *claims.CompanyID == companyID) ||
			claims.HasPermission("companies.verify"))

	res, err := h.companies.Get(c.UserContext(), companyID, includePrivate)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// List serves the public company directory.
// GET /api/v1/companies
func (h *CompanyHandler) List(c *fiber.Ctx) error {
	var q dto.CompanySearchQuery
	if err := bindQuery(c, &q, h.validator); err != nil {
		return err
	}
	claims := middleware.Claims(c)
	includePrivate := claims != nil && claims.HasPermission("companies.verify")
	// Only an admin may filter by a non-verified status.
	if q.Status != "" && !includePrivate {
		q.Status = ""
	}

	page := pagination.Parse(c)
	items, total, err := h.companies.List(c.UserContext(), q,
		page.Offset(), page.Limit(),
		page.OrderClause(companySortColumns, ""), includePrivate)
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

// ─── documents ──────────────────────────────────────────────────────

// UploadDocument attaches a company paper.
// POST /api/v1/companies/me/documents
func (h *CompanyHandler) UploadDocument(c *fiber.Ctx) error {
	var req dto.UploadCompanyDocumentRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, companyID, err := h.actorAndCompany(c)
	if err != nil {
		return err
	}
	res, err := h.companies.UploadDocument(c.UserContext(), userID, companyID, req)
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// ListDocuments returns the company's papers.
// GET /api/v1/companies/me/documents
func (h *CompanyHandler) ListDocuments(c *fiber.Ctx) error {
	userID, companyID, err := h.actorAndCompany(c)
	if err != nil {
		return err
	}
	res, err := h.companies.ListDocuments(c.UserContext(), userID, companyID)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// ReviewDocument records an admin decision on a company paper.
// POST /api/v1/admin/company-documents/:id/review
func (h *CompanyHandler) ReviewDocument(c *fiber.Ctx) error {
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
	if err := h.companies.ReviewDocument(c.UserContext(), adminID, docID, req); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"status": req.Status})
}

// Review verifies or rejects a company directly.
// POST /api/v1/admin/companies/:id/review
func (h *CompanyHandler) Review(c *fiber.Ctx) error {
	var req dto.ReviewCompanyRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	adminID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	companyID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.companies.Review(c.UserContext(), adminID, companyID, req); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"status": req.Status})
}

// PendingDocuments serves the admin company verification queue.
// GET /api/v1/admin/company-documents/pending
func (h *CompanyHandler) PendingDocuments(c *fiber.Ctx) error {
	page := pagination.Parse(c)
	items, total, err := h.companies.ListPendingDocuments(c.UserContext(), page.Offset(), page.Limit())
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

// ─── membership ─────────────────────────────────────────────────────

// InviteMember adds a person to the company.
// POST /api/v1/companies/me/members
func (h *CompanyHandler) InviteMember(c *fiber.Ctx) error {
	var req dto.InviteMemberRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, companyID, err := h.actorAndCompany(c)
	if err != nil {
		return err
	}
	res, err := h.companies.InviteMember(c.UserContext(), userID, companyID, req)
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// ListMembers returns the company roster.
// GET /api/v1/companies/me/members
func (h *CompanyHandler) ListMembers(c *fiber.Ctx) error {
	userID, companyID, err := h.actorAndCompany(c)
	if err != nil {
		return err
	}
	res, err := h.companies.ListMembers(c.UserContext(), userID, companyID, c.QueryBool("active_only", true))
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// UpdateMember changes a member's role or activation.
// PATCH /api/v1/companies/me/members/:id
func (h *CompanyHandler) UpdateMember(c *fiber.Ctx) error {
	var req dto.UpdateMemberRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, companyID, err := h.actorAndCompany(c)
	if err != nil {
		return err
	}
	memberID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.companies.UpdateMember(c.UserContext(), userID, companyID, memberID, req)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// RemoveMember revokes a membership.
// DELETE /api/v1/companies/me/members/:id
func (h *CompanyHandler) RemoveMember(c *fiber.Ctx) error {
	userID, companyID, err := h.actorAndCompany(c)
	if err != nil {
		return err
	}
	memberID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.companies.RemoveMember(c.UserContext(), userID, companyID, memberID); err != nil {
		return err
	}
	return response.NoContent(c)
}

// actorAndCompany extracts the caller and the company they act for.
func (h *CompanyHandler) actorAndCompany(c *fiber.Ctx) (uuid.UUID, uuid.UUID, error) {
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if claims.CompanyID == nil {
		return uuid.Nil, uuid.Nil, apperror.ErrForbidden.WithMessage("Create or join a company first")
	}
	return claims.UserID, *claims.CompanyID, nil
}
