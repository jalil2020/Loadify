package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/loadify/backend/internal/delivery/http/middleware"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/internal/usecase"
	"github.com/loadify/backend/pkg/response"
	"github.com/loadify/backend/pkg/secret"
	"github.com/loadify/backend/pkg/validator"
)

// TelegramHandler exposes connecting a person's own Telegram account and
// choosing the groups it posts to.
type TelegramHandler struct {
	telegram  *usecase.TelegramUsecase
	validator validator.Validator
}

// NewTelegramHandler builds the handler.
func NewTelegramHandler(telegram *usecase.TelegramUsecase, v validator.Validator) *TelegramHandler {
	return &TelegramHandler{telegram: telegram, validator: v}
}

// StartLogin asks Telegram to send a code to the caller's app.
// POST /api/v1/telegram/accounts/login
func (h *TelegramHandler) StartLogin(c *fiber.Ctx) error {
	var req dto.StartTelegramLoginRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	res, err := h.telegram.StartLogin(c.UserContext(), userID, companyID, req)
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// SubmitCode checks the code Telegram sent.
// POST /api/v1/telegram/accounts/:id/code
func (h *TelegramHandler) SubmitCode(c *fiber.Ctx) error {
	var req dto.TelegramCodeRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	accountID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	// The code is wrapped at the edge so that nothing below this line can
	// print it by accident.
	res, err := h.telegram.SubmitCode(c.UserContext(), userID, companyID, accountID, secret.Value(req.Code))
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// SubmitPassword completes a sign-in with two-step verification.
// POST /api/v1/telegram/accounts/:id/password
func (h *TelegramHandler) SubmitPassword(c *fiber.Ctx) error {
	var req dto.TelegramPasswordRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	accountID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.telegram.SubmitPassword(c.UserContext(), userID, companyID, accountID, secret.Value(req.Password))
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Accounts lists connected accounts: the caller's own, or the whole company's
// for a manager — without anything that could be used to post as them.
// GET /api/v1/telegram/accounts
func (h *TelegramHandler) Accounts(c *fiber.Ctx) error {
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}
	wholeCompany := claims.HasRole(domain.RoleCompanyManager) && c.QueryBool("company")

	res, err := h.telegram.Accounts(c.UserContext(), userID, companyID, wholeCompany)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Disconnect ends an account's session and stops what it had queued.
// DELETE /api/v1/telegram/accounts/:id
func (h *TelegramHandler) Disconnect(c *fiber.Ctx) error {
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	accountID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.telegram.Disconnect(c.UserContext(), userID, companyID, accountID); err != nil {
		return err
	}
	return response.NoContent(c)
}

// Dialogs lists the groups the account could post to.
// GET /api/v1/telegram/accounts/:id/dialogs
func (h *TelegramHandler) Dialogs(c *fiber.Ctx) error {
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	accountID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.telegram.Dialogs(c.UserContext(), userID, companyID, accountID)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Groups lists the company's connected groups with their rules.
// GET /api/v1/telegram/groups
func (h *TelegramHandler) Groups(c *fiber.Ctx) error {
	_, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	res, err := h.telegram.Groups(c.UserContext(), companyID)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// ConnectGroup adds a group from the account's own dialog list.
// POST /api/v1/telegram/groups
func (h *TelegramHandler) ConnectGroup(c *fiber.Ctx) error {
	var req dto.ConnectGroupRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	res, err := h.telegram.ConnectGroup(c.UserContext(), userID, companyID, req)
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// SaveRules replaces what a group accepts and how often it may be posted to.
// PUT /api/v1/telegram/groups/:id/rules
func (h *TelegramHandler) SaveRules(c *fiber.Ctx) error {
	var req dto.SaveGroupRulesRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	groupID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	res, err := h.telegram.SaveRules(c.UserContext(), userID, companyID, groupID, req)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// RemoveGroup stops posting to a group.
// DELETE /api/v1/telegram/groups/:id
func (h *TelegramHandler) RemoveGroup(c *fiber.Ctx) error {
	userID, companyID, err := actorAndCompany(c)
	if err != nil {
		return err
	}
	groupID, err := paramUUID(c, "id")
	if err != nil {
		return err
	}
	if err := h.telegram.RemoveGroup(c.UserContext(), userID, companyID, groupID); err != nil {
		return err
	}
	return response.NoContent(c)
}
