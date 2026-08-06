// Package dto holds the request and response contracts of the HTTP API.
// Entities never cross the transport boundary directly.
package dto

import (
	"time"

	"github.com/google/uuid"
)

// ─── requests ───────────────────────────────────────────────────────

// RegisterRequest starts an account for a driver or a logistics company.
type RegisterRequest struct {
	Phone     string `json:"phone" validate:"required,uzphone"`
	Password  string `json:"password" validate:"required,password"`
	FirstName string `json:"first_name" validate:"required,min=2,max=100"`
	LastName  string `json:"last_name" validate:"required,min=2,max=100"`
	Role      string `json:"role" validate:"required,oneof=driver company"`
	Locale    string `json:"locale" validate:"omitempty,locale"`
	Email     string `json:"email" validate:"omitempty,email,max=255"`
}

// VerifyOTPRequest confirms a code sent by SMS.
type VerifyOTPRequest struct {
	Phone   string `json:"phone" validate:"required,uzphone"`
	Code    string `json:"code" validate:"required,min=4,max=8"`
	Purpose string `json:"purpose" validate:"required,oneof=register login reset_password change_phone"`
}

// ResendOTPRequest asks for a fresh code.
type ResendOTPRequest struct {
	Phone   string `json:"phone" validate:"required,uzphone"`
	Purpose string `json:"purpose" validate:"required,oneof=register login reset_password change_phone"`
}

// LoginRequest authenticates with phone and password.
type LoginRequest struct {
	Phone      string `json:"phone" validate:"required,uzphone"`
	Password   string `json:"password" validate:"required,min=1,max=72"`
	DeviceID   string `json:"device_id" validate:"omitempty,max=128"`
	DeviceName string `json:"device_name" validate:"omitempty,max=128"`
	Platform   string `json:"platform" validate:"omitempty,oneof=android ios web"`
	AppVersion string `json:"app_version" validate:"omitempty,max=32"`
}

// RefreshRequest exchanges a refresh token for a new pair.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required,min=16"`
}

// LogoutRequest ends one session.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" validate:"omitempty,min=16"`
}

// ForgotPasswordRequest starts password recovery.
type ForgotPasswordRequest struct {
	Phone string `json:"phone" validate:"required,uzphone"`
}

// ResetPasswordRequest completes password recovery with the token returned by
// a successful reset_password OTP verification.
type ResetPasswordRequest struct {
	ResetToken  string `json:"reset_token" validate:"required,min=16"`
	NewPassword string `json:"new_password" validate:"required,password"`
}

// ChangePasswordRequest changes the password of the signed-in user.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required,min=1,max=72"`
	NewPassword     string `json:"new_password" validate:"required,password,nefield=CurrentPassword"`
}

// ─── responses ──────────────────────────────────────────────────────

// TokenResponse carries a freshly issued token pair.
type TokenResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int64     `json:"expires_in"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// AuthResponse is returned by register, verify, login and refresh.
type AuthResponse struct {
	User   UserResponse  `json:"user"`
	Tokens TokenResponse `json:"tokens"`
}

// RegisterResponse is returned before the phone has been confirmed — no tokens
// are issued until the OTP is verified.
type RegisterResponse struct {
	UserID       uuid.UUID `json:"user_id"`
	Phone        string    `json:"phone"`
	OTPExpiresAt time.Time `json:"otp_expires_at"`
	ResendAfter  int       `json:"resend_after_seconds"`
	Message      string    `json:"message"`
}

// OTPSentResponse acknowledges that a code was dispatched.
type OTPSentResponse struct {
	Phone        string    `json:"phone"`
	Purpose      string    `json:"purpose"`
	OTPExpiresAt time.Time `json:"otp_expires_at"`
	ResendAfter  int       `json:"resend_after_seconds"`
}

// VerifyOTPResponse is polymorphic: register and login verifications return
// tokens, a reset_password verification returns a single-use reset token.
type VerifyOTPResponse struct {
	Verified   bool          `json:"verified"`
	Auth       *AuthResponse `json:"auth,omitempty"`
	ResetToken string        `json:"reset_token,omitempty"`
	ExpiresAt  *time.Time    `json:"reset_token_expires_at,omitempty"`
}

// UserResponse is the account representation returned to clients.
type UserResponse struct {
	ID            uuid.UUID  `json:"id"`
	Phone         string     `json:"phone"`
	Email         *string    `json:"email"`
	FirstName     string     `json:"first_name"`
	LastName      string     `json:"last_name"`
	FullName      string     `json:"full_name"`
	AvatarURL     *string    `json:"avatar_url"`
	Locale        string     `json:"locale"`
	Status        string     `json:"status"`
	PhoneVerified bool       `json:"phone_verified"`
	Roles         []string   `json:"roles"`
	Permissions   []string   `json:"permissions"`
	DriverID      *uuid.UUID `json:"driver_id,omitempty"`
	CompanyID     *uuid.UUID `json:"company_id,omitempty"`
	CompanyRole   string     `json:"company_role,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	LastLoginAt   *time.Time `json:"last_login_at"`
}

// SessionResponse describes one active device session.
type SessionResponse struct {
	ID         uuid.UUID `json:"id"`
	DeviceName *string   `json:"device_name"`
	Platform   string    `json:"platform"`
	AppVersion *string   `json:"app_version"`
	IPAddress  *string   `json:"ip_address"`
	LastUsedAt time.Time `json:"last_used_at"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Current    bool      `json:"is_current"`
}
