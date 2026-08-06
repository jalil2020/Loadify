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

// adminUserSortColumns is the allowlist for ?sort= on the user directory.
var adminUserSortColumns = map[string]string{
	"created_at":    "created_at",
	"last_login_at": "last_login_at",
	"phone":         "phone",
}

// AdminHandler exposes moderation, administration and reporting endpoints.
type AdminHandler struct {
	admin     *usecase.AdminUsecase
	validator validator.Validator
}

// NewAdminHandler builds the admin handler.
func NewAdminHandler(admin *usecase.AdminUsecase, v validator.Validator) *AdminHandler {
	return &AdminHandler{admin: admin, validator: v}
}

// ─── dashboard & reports ────────────────────────────────────────────

// Dashboard returns the live marketplace snapshot.
// GET /api/v1/admin/dashboard
func (h *AdminHandler) Dashboard(c *fiber.Ctx) error {
	var q dto.ReportQuery
	if err := bindQuery(c, &q, h.validator); err != nil {
		return err
	}
	res, err := h.admin.Dashboard(c.UserContext(), q.From, q.To)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Series returns a daily chart for one metric.
// GET /api/v1/admin/analytics/series?metric=gmv
func (h *AdminHandler) Series(c *fiber.Ctx) error {
	var q dto.ReportQuery
	if err := bindQuery(c, &q, h.validator); err != nil {
		return err
	}
	res, err := h.admin.Series(c.UserContext(), c.Query("metric"), q.From, q.To)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// TopCorridors ranks routes by volume.
// GET /api/v1/admin/analytics/corridors
func (h *AdminHandler) TopCorridors(c *fiber.Ctx) error {
	var q dto.ReportQuery
	if err := bindQuery(c, &q, h.validator); err != nil {
		return err
	}
	res, err := h.admin.TopCorridors(c.UserContext(), q.From, q.To, c.QueryInt("limit", 20))
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// CompanyReport returns a shipper's performance summary.
//
// A company may read its own report without an admin role; anyone else needs
// the reports permission, which the route enforces on the admin group.
// GET /api/v1/companies/me/report
func (h *AdminHandler) CompanyReport(c *fiber.Ctx) error {
	var q dto.ReportQuery
	if err := bindQuery(c, &q, h.validator); err != nil {
		return err
	}
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}

	companyID := claims.CompanyID
	// An admin may request any company by id.
	if raw := c.Query("company_id"); raw != "" {
		if !isAdmin(claims.Roles) {
			return apperror.ErrForbidden
		}
		id, parseErr := uuid.Parse(raw)
		if parseErr != nil {
			return apperror.ErrBadRequest.WithMessage("Invalid company_id")
		}
		companyID = &id
	}
	if companyID == nil {
		return apperror.ErrForbidden.WithMessage("You do not belong to a company")
	}

	res, err := h.admin.CompanyReport(c.UserContext(), *companyID, q.From, q.To)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// DriverReport returns a driver's earnings summary.
// GET /api/v1/drivers/me/report
func (h *AdminHandler) DriverReport(c *fiber.Ctx) error {
	var q dto.ReportQuery
	if err := bindQuery(c, &q, h.validator); err != nil {
		return err
	}
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}

	driverID := claims.DriverID
	if raw := c.Query("driver_id"); raw != "" {
		if !isAdmin(claims.Roles) {
			return apperror.ErrForbidden
		}
		id, parseErr := uuid.Parse(raw)
		if parseErr != nil {
			return apperror.ErrBadRequest.WithMessage("Invalid driver_id")
		}
		driverID = &id
	}
	if driverID == nil {
		return apperror.ErrForbidden.WithMessage("You do not have a driver profile")
	}

	res, err := h.admin.DriverReport(c.UserContext(), *driverID, q.From, q.To)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// ─── users ──────────────────────────────────────────────────────────

// ListUsers serves the admin user directory.
// GET /api/v1/admin/users
func (h *AdminHandler) ListUsers(c *fiber.Ctx) error {
	var q dto.UserSearchQuery
	if err := bindQuery(c, &q, h.validator); err != nil {
		return err
	}
	filter := domain.UserFilter{Search: q.Search}
	if q.Status != "" {
		filter.Status = []domain.UserStatus{domain.UserStatus(q.Status)}
	}
	if len(q.Roles) > 0 {
		filter.RoleCode = q.Roles[0]
	}

	page := pagination.Parse(c)
	items, total, err := h.admin.ListUsers(c.UserContext(), filter,
		page.Offset(), page.Limit(), page.OrderClause(adminUserSortColumns, "created_at DESC"))
	if err != nil {
		return err
	}
	return paginated(c, items, total, page)
}

// GetUser returns one account.
// GET /api/v1/admin/users/:id
func (h *AdminHandler) GetUser(c *fiber.Ctx) error {
	userID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.admin.GetUser(c.UserContext(), userID)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// BlockUser suspends an account and kills its sessions.
// POST /api/v1/admin/users/:id/block
func (h *AdminHandler) BlockUser(c *fiber.Ctx) error {
	var req dto.BlockUserRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	adminID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	userID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.admin.BlockUser(c.UserContext(), adminID, userID, req.Reason); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"blocked": true})
}

// UnblockUser restores access.
// POST /api/v1/admin/users/:id/unblock
func (h *AdminHandler) UnblockUser(c *fiber.Ctx) error {
	adminID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	userID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.admin.UnblockUser(c.UserContext(), adminID, userID); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"blocked": false})
}

// AssignRole grants a platform role.
// POST /api/v1/admin/users/:id/roles
func (h *AdminHandler) AssignRole(c *fiber.Ctx) error {
	var req dto.AssignRoleRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	adminID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	userID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.admin.AssignRole(c.UserContext(), adminID, userID, req.RoleCode, req.CompanyID); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"role": req.RoleCode})
}

// RevokeRole removes a platform role.
// DELETE /api/v1/admin/users/:id/roles
func (h *AdminHandler) RevokeRole(c *fiber.Ctx) error {
	var req dto.AssignRoleRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	adminID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	userID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.admin.RevokeRole(c.UserContext(), adminID, userID, req.RoleCode, req.CompanyID); err != nil {
		return err
	}
	return response.NoContent(c)
}

// ─── complaints ─────────────────────────────────────────────────────

// FileComplaint records a report from a user.
// POST /api/v1/complaints
func (h *AdminHandler) FileComplaint(c *fiber.Ctx) error {
	var req dto.FileComplaintRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	res, err := h.admin.FileComplaint(c.UserContext(), userID, req)
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// ListComplaints serves the moderation queue.
// GET /api/v1/admin/complaints
func (h *AdminHandler) ListComplaints(c *fiber.Ctx) error {
	var q dto.ComplaintSearchQuery
	if err := bindQuery(c, &q, h.validator); err != nil {
		return err
	}
	page := pagination.Parse(c)
	items, total, err := h.admin.ListComplaints(c.UserContext(), q, page.Offset(), page.Limit())
	if err != nil {
		return err
	}
	return paginated(c, items, total, page)
}

// GetComplaint returns one report.
// GET /api/v1/admin/complaints/:id
func (h *AdminHandler) GetComplaint(c *fiber.Ctx) error {
	complaintID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.admin.GetComplaint(c.UserContext(), complaintID)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// AssignComplaint puts a complaint on a moderator's desk.
// POST /api/v1/admin/complaints/:id/assign
func (h *AdminHandler) AssignComplaint(c *fiber.Ctx) error {
	var req dto.AssignComplaintRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	adminID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	complaintID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.admin.AssignComplaint(c.UserContext(), adminID, complaintID, req.AssigneeID); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"assigned_to": req.AssigneeID})
}

// ResolveComplaint closes a complaint and optionally settles the escrow.
// POST /api/v1/admin/complaints/:id/resolve
func (h *AdminHandler) ResolveComplaint(c *fiber.Ctx) error {
	var req dto.ResolveComplaintRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	adminID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	complaintID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.admin.ResolveComplaint(c.UserContext(), adminID, complaintID, req); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"upheld": req.Upheld})
}

// ─── audit ──────────────────────────────────────────────────────────

// ListAuditLogs searches the audit trail.
// GET /api/v1/admin/audit-logs
func (h *AdminHandler) ListAuditLogs(c *fiber.Ctx) error {
	var q dto.AuditSearchQuery
	if err := bindQuery(c, &q, h.validator); err != nil {
		return err
	}
	page := pagination.Parse(c)
	items, total, err := h.admin.ListAuditLogs(c.UserContext(), q, page.Offset(), page.Limit())
	if err != nil {
		return err
	}
	return paginated(c, items, total, page)
}

// ResourceHistory returns the audit trail of one object.
// GET /api/v1/admin/audit-logs/:resource_type/:id
func (h *AdminHandler) ResourceHistory(c *fiber.Ctx) error {
	resourceID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.admin.ResourceHistory(c.UserContext(),
		c.Params("resource_type"), resourceID, c.QueryInt("limit", 100))
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// ─── settings ───────────────────────────────────────────────────────

// ListSettings returns the operator-tunable values.
// GET /api/v1/admin/settings
func (h *AdminHandler) ListSettings(c *fiber.Ctx) error {
	res, err := h.admin.ListSettings(c.UserContext(), c.Query("group"), false)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// PublicSettings returns the values clients need without an admin role, such as
// the support phone number and the tracking intervals.
// GET /api/v1/settings
func (h *AdminHandler) PublicSettings(c *fiber.Ctx) error {
	res, err := h.admin.ListSettings(c.UserContext(), "", true)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// UpdateSetting changes one tunable value.
// PUT /api/v1/admin/settings/:key
func (h *AdminHandler) UpdateSetting(c *fiber.Ctx) error {
	var req dto.UpdateSettingRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	adminID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	if err := h.admin.UpdateSetting(c.UserContext(), adminID, c.Params("key"), req.Value); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"key": c.Params("key")})
}

// ListFeatureFlags returns the capability switches.
// GET /api/v1/admin/feature-flags
func (h *AdminHandler) ListFeatureFlags(c *fiber.Ctx) error {
	res, err := h.admin.ListFeatureFlags(c.UserContext())
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// SetFeatureFlag toggles a capability.
// PUT /api/v1/admin/feature-flags/:key
func (h *AdminHandler) SetFeatureFlag(c *fiber.Ctx) error {
	var req dto.SetFeatureFlagRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	adminID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	if err := h.admin.SetFeatureFlag(c.UserContext(), adminID,
		c.Params("key"), req.IsEnabled, req.RolloutPercent); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"key": c.Params("key"), "enabled": req.IsEnabled})
}
