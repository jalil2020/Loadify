// Package apperror defines the single error type that crosses layer boundaries.
// Usecases return apperror values; the HTTP error handler is the only place that
// turns them into status codes and JSON.
package apperror

import (
	"errors"
	"fmt"
	"net/http"
)

// FieldError describes one failed field in a validation error.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Error is a domain-level error carrying a stable machine code, a human message,
// the HTTP status it maps to, optional field details and the wrapped cause.
type Error struct {
	Code       string       `json:"code"`
	Message    string       `json:"message"`
	HTTPStatus int          `json:"-"`
	Details    []FieldError `json:"details,omitempty"`
	cause      error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.cause }

// Wrap attaches the underlying cause. The cause is logged, never returned to the client.
func (e *Error) Wrap(cause error) *Error {
	clone := *e
	clone.cause = cause
	return &clone
}

// WithMessage overrides the human-readable message.
func (e *Error) WithMessage(msg string) *Error {
	clone := *e
	clone.Message = msg
	return &clone
}

// WithDetails attaches per-field details.
func (e *Error) WithDetails(details ...FieldError) *Error {
	clone := *e
	clone.Details = append(append([]FieldError{}, clone.Details...), details...)
	return &clone
}

// New builds a custom error.
func New(code, message string, status int) *Error {
	return &Error{Code: code, Message: message, HTTPStatus: status}
}

// As extracts an *Error from any error, returning ErrInternal when the error is
// not one of ours.
func As(err error) *Error {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr
	}
	return ErrInternal.Wrap(err)
}

// Is reports whether err is an *Error with the given code.
func Is(err error, code string) bool {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Code == code
	}
	return false
}

// Generic
var (
	ErrInternal     = New("INTERNAL_ERROR", "Something went wrong on our side", http.StatusInternalServerError)
	ErrBadRequest   = New("BAD_REQUEST", "Malformed request", http.StatusBadRequest)
	ErrValidation   = New("VALIDATION_ERROR", "Validation failed", http.StatusUnprocessableEntity)
	ErrNotFound     = New("NOT_FOUND", "Resource not found", http.StatusNotFound)
	ErrConflict     = New("CONFLICT", "Resource already exists", http.StatusConflict)
	ErrUnauthorized = New("UNAUTHORIZED", "Authentication required", http.StatusUnauthorized)
	ErrForbidden    = New("FORBIDDEN", "You do not have permission to perform this action", http.StatusForbidden)
	ErrRateLimited  = New("RATE_LIMITED", "Too many requests, slow down", http.StatusTooManyRequests)
	ErrUnavailable  = New("SERVICE_UNAVAILABLE", "Service temporarily unavailable", http.StatusServiceUnavailable)
	ErrPayload      = New("PAYLOAD_TOO_LARGE", "Uploaded file is too large", http.StatusRequestEntityTooLarge)
)

// Authentication
var (
	ErrInvalidCredentials = New("INVALID_CREDENTIALS", "Phone or password is incorrect", http.StatusUnauthorized)
	ErrPhoneTaken         = New("PHONE_ALREADY_REGISTERED", "This phone number is already registered", http.StatusConflict)
	ErrEmailTaken         = New("EMAIL_ALREADY_REGISTERED", "This email is already registered", http.StatusConflict)
	ErrPhoneNotVerified   = New("PHONE_NOT_VERIFIED", "Confirm your phone number first", http.StatusForbidden)
	ErrAccountBlocked     = New("ACCOUNT_BLOCKED", "This account has been blocked", http.StatusForbidden)
	ErrAccountLocked      = New("ACCOUNT_LOCKED", "Too many failed attempts, try again later", http.StatusLocked)
	ErrTokenInvalid       = New("TOKEN_INVALID", "Token is invalid", http.StatusUnauthorized)
	ErrTokenExpired       = New("TOKEN_EXPIRED", "Token has expired", http.StatusUnauthorized)
	ErrTokenRevoked       = New("TOKEN_REVOKED", "Session has been revoked", http.StatusUnauthorized)
	ErrTokenReuse         = New("TOKEN_REUSE_DETECTED", "Session invalidated for security reasons, sign in again", http.StatusUnauthorized)
	ErrOTPInvalid         = New("OTP_INVALID", "Confirmation code is incorrect", http.StatusUnprocessableEntity)
	ErrOTPExpired         = New("OTP_EXPIRED", "Confirmation code has expired", http.StatusUnprocessableEntity)
	ErrOTPAttempts        = New("OTP_TOO_MANY_ATTEMPTS", "Too many incorrect attempts, request a new code", http.StatusTooManyRequests)
	ErrOTPCooldown        = New("OTP_COOLDOWN", "Wait before requesting another code", http.StatusTooManyRequests)
	ErrPasswordSame       = New("PASSWORD_SAME", "The new password must differ from the current one", http.StatusUnprocessableEntity)
)

// Domain
var (
	ErrDriverNotVerified   = New("DRIVER_NOT_VERIFIED", "Your account is not verified yet", http.StatusForbidden)
	ErrCompanyNotVerified  = New("COMPANY_NOT_VERIFIED", "Your company is not verified yet", http.StatusForbidden)
	ErrNoApprovedVehicle   = New("NO_APPROVED_VEHICLE", "Add and get a vehicle approved first", http.StatusForbidden)
	ErrVehicleUnsuitable   = New("VEHICLE_UNSUITABLE", "The selected vehicle does not meet the load requirements", http.StatusUnprocessableEntity)
	ErrLoadNotPublished    = New("LOAD_NOT_PUBLISHED", "This load is not open for reservation", http.StatusConflict)
	ErrLoadAlreadyBooked   = New("LOAD_ALREADY_BOOKED", "This load has already been assigned", http.StatusConflict)
	ErrBookingExists       = New("BOOKING_EXISTS", "You already have an open booking for this load", http.StatusConflict)
	ErrBookingNotPending   = New("BOOKING_NOT_PENDING", "This booking can no longer be changed", http.StatusConflict)
	ErrTripConflict        = New("TRIP_CONFLICT", "You already have a trip overlapping this period", http.StatusConflict)
	ErrInvalidStatusChange = New("INVALID_STATUS_TRANSITION", "This status change is not allowed", http.StatusConflict)
	ErrQuotaExceeded       = New("QUOTA_EXCEEDED", "Your plan limit has been reached", http.StatusPaymentRequired)
	ErrInsufficientFunds   = New("INSUFFICIENT_FUNDS", "Not enough balance", http.StatusPaymentRequired)
	ErrSubscriptionActive  = New("SUBSCRIPTION_ACTIVE", "An active subscription already exists", http.StatusConflict)
)
