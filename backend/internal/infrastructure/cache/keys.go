package cache

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/google/uuid"
)

// Key builders. Every Redis key in the codebase is produced here so the
// keyspace stays discoverable and prefixes never drift.
const (
	prefixDenylist   = "auth:denylist:"
	prefixUserPerms  = "auth:perms:"
	prefixUserCache  = "user:"
	prefixOTPCooldwn = "otp:cooldown:"
	prefixOTPRate    = "otp:rate:"
	prefixLoginFail  = "auth:login_fail:"
	prefixRateLimit  = "rl:"
	prefixDriverLoc  = "driver:loc:"
	prefixOnline     = "presence:"
	prefixSettings   = "settings"
	prefixContactRvl = "quota:contact:"
	prefixIdempotent = "idem:"
)

// Realtime pub/sub channels.
const (
	ChannelDriverLocation = "rt:driver_location"
	ChannelBooking        = "rt:booking"
	ChannelChat           = "rt:chat"
	ChannelNotification   = "rt:notification"
	ChannelTrip           = "rt:trip"
	ChannelLoad           = "rt:load"
)

// DenylistKey holds a revoked access-token JTI until its natural expiry.
func DenylistKey(jti uuid.UUID) string { return prefixDenylist + jti.String() }

// UserPermissionsKey caches the effective permission set of a user.
func UserPermissionsKey(userID uuid.UUID) string { return prefixUserPerms + userID.String() }

// UserKey caches the user record used on every authenticated request.
func UserKey(userID uuid.UUID) string { return prefixUserCache + userID.String() }

// OTPCooldownKey throttles resend requests for a phone and purpose.
func OTPCooldownKey(phone, purpose string) string { return prefixOTPCooldwn + purpose + ":" + phone }

// OTPRateKey counts OTP requests per phone per hour.
func OTPRateKey(phone string) string { return prefixOTPRate + phone }

// LoginFailKey counts consecutive failed logins for a phone.
func LoginFailKey(phone string) string { return prefixLoginFail + phone }

// RateLimitKey scopes the HTTP rate limiter by identity and route group.
func RateLimitKey(scope, identity string) string { return prefixRateLimit + scope + ":" + identity }

// DriverLocationKey holds the latest known position of a driver.
func DriverLocationKey(driverID uuid.UUID) string { return prefixDriverLoc + driverID.String() }

// PresenceKey marks a user as connected to a WebSocket node.
func PresenceKey(userID uuid.UUID) string { return prefixOnline + userID.String() }

// SettingsKey caches the system settings map.
func SettingsKey() string { return prefixSettings }

// ContactRevealKey counts a driver's contact reveals for the given day.
func ContactRevealKey(userID uuid.UUID, day string) string {
	return prefixContactRvl + userID.String() + ":" + day
}

// IdempotencyKey namespaces a client-supplied idempotency key by operation.
func IdempotencyKey(operation, key string) string {
	return prefixIdempotent + operation + ":" + key
}

func randomToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
