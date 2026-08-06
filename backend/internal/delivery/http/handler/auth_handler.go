// Package handler contains the Fiber handlers. A handler binds and validates
// the request, calls exactly one usecase and writes the response. No business
// rules live here.
package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/loadify/backend/internal/delivery/http/middleware"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/internal/usecase"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/response"
	"github.com/loadify/backend/pkg/validator"
)

// AuthHandler exposes the authentication endpoints.
type AuthHandler struct {
	auth      *usecase.AuthUsecase
	validator validator.Validator
}

// NewAuthHandler builds the auth handler.
func NewAuthHandler(auth *usecase.AuthUsecase, v validator.Validator) *AuthHandler {
	return &AuthHandler{auth: auth, validator: v}
}

// Register creates an account and sends the confirmation code.
// POST /api/v1/auth/register
func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req dto.RegisterRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	res, err := h.auth.Register(c.UserContext(), req, middleware.Device(c))
	if err != nil {
		return err
	}
	return response.Created(c, res)
}

// VerifyOTP confirms a code and, depending on the purpose, signs the user in or
// returns a password-reset token.
// POST /api/v1/auth/verify-otp
func (h *AuthHandler) VerifyOTP(c *fiber.Ctx) error {
	var req dto.VerifyOTPRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	res, err := h.auth.VerifyOTP(c.UserContext(), req, middleware.Device(c))
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// ResendOTP issues a fresh confirmation code.
// POST /api/v1/auth/resend-otp
func (h *AuthHandler) ResendOTP(c *fiber.Ctx) error {
	var req dto.ResendOTPRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	res, err := h.auth.ResendOTP(c.UserContext(), req, middleware.Device(c))
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Login authenticates with phone and password.
// POST /api/v1/auth/login
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req dto.LoginRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	res, err := h.auth.Login(c.UserContext(), req, middleware.Device(c))
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Refresh rotates the token pair.
// POST /api/v1/auth/refresh
func (h *AuthHandler) Refresh(c *fiber.Ctx) error {
	var req dto.RefreshRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	res, err := h.auth.Refresh(c.UserContext(), req, middleware.Device(c))
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Logout ends the current session.
// POST /api/v1/auth/logout
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	var req dto.LogoutRequest
	// The body is optional here: a client that lost its refresh token can still
	// log out using only the access token.
	_ = c.BodyParser(&req)

	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}
	if err := h.auth.Logout(c.UserContext(), claims, req.RefreshToken); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"message": "Signed out"})
}

// LogoutAll ends every session of the current user.
// POST /api/v1/auth/logout-all
func (h *AuthHandler) LogoutAll(c *fiber.Ctx) error {
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}
	if err := h.auth.LogoutAll(c.UserContext(), claims); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"message": "Signed out on all devices"})
}

// ForgotPassword starts password recovery.
// POST /api/v1/auth/forgot-password
func (h *AuthHandler) ForgotPassword(c *fiber.Ctx) error {
	var req dto.ForgotPasswordRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	res, err := h.auth.ForgotPassword(c.UserContext(), req, middleware.Device(c))
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// ResetPassword completes password recovery.
// POST /api/v1/auth/reset-password
func (h *AuthHandler) ResetPassword(c *fiber.Ctx) error {
	var req dto.ResetPasswordRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	if err := h.auth.ResetPassword(c.UserContext(), req, middleware.Device(c)); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"message": "Password updated, sign in again"})
}

// ChangePassword changes the password of the signed-in user.
// POST /api/v1/auth/change-password
func (h *AuthHandler) ChangePassword(c *fiber.Ctx) error {
	var req dto.ChangePasswordRequest
	if err := bind(c, &req, h.validator); err != nil {
		return err
	}
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}
	if err := h.auth.ChangePassword(c.UserContext(), claims, req); err != nil {
		return err
	}
	return response.OK(c, fiber.Map{"message": "Password changed, sign in again"})
}

// Me returns the signed-in account.
// GET /api/v1/auth/me
func (h *AuthHandler) Me(c *fiber.Ctx) error {
	userID, err := middleware.UserID(c)
	if err != nil {
		return err
	}
	res, err := h.auth.Me(c.UserContext(), userID)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// Sessions lists the user's active devices.
// GET /api/v1/auth/sessions
func (h *AuthHandler) Sessions(c *fiber.Ctx) error {
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}
	res, err := h.auth.ListSessions(c.UserContext(), claims)
	if err != nil {
		return err
	}
	return response.OK(c, res)
}

// RevokeSession ends one device session.
// DELETE /api/v1/auth/sessions/:id
func (h *AuthHandler) RevokeSession(c *fiber.Ctx) error {
	claims, err := middleware.MustClaims(c)
	if err != nil {
		return err
	}
	sessionID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return apperror.ErrBadRequest.WithMessage("Invalid session id")
	}
	if err := h.auth.RevokeSession(c.UserContext(), claims, sessionID); err != nil {
		return err
	}
	return response.NoContent(c)
}
