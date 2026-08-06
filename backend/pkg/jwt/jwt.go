// Package jwt issues and verifies the platform's access tokens.
// Refresh tokens are opaque random strings persisted (hashed) in the sessions
// table — they are deliberately not JWTs, so revocation is a database fact.
package jwt

import (
	"errors"
	"fmt"
	"slices"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/hash"
)

// Claims is the access-token payload.
type Claims struct {
	UserID      uuid.UUID  `json:"sub_id"`
	Phone       string     `json:"phone"`
	Roles       []string   `json:"roles"`
	Permissions []string   `json:"perms"`
	CompanyID   *uuid.UUID `json:"company_id,omitempty"`
	DriverID    *uuid.UUID `json:"driver_id,omitempty"`
	SessionID   uuid.UUID  `json:"sid"`
	jwtlib.RegisteredClaims
}

// TokenPair is what the auth endpoints hand back to clients.
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int64     `json:"expires_in"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Manager issues and parses tokens.
type Manager interface {
	GenerateAccess(claims Claims) (token string, jti uuid.UUID, expiresAt time.Time, err error)
	GenerateRefresh() (token string, tokenHash string, expiresAt time.Time, err error)
	ParseAccess(token string) (*Claims, error)
	AccessTTL() time.Duration
	RefreshTTL() time.Duration
}

// Config configures the manager.
type Config struct {
	AccessSecret  string
	RefreshSecret string
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
	Issuer        string
}

type manager struct{ cfg Config }

// New builds a Manager, rejecting secrets too short to be safe for HS256.
func New(cfg Config) (Manager, error) {
	if len(cfg.AccessSecret) < 32 {
		return nil, fmt.Errorf("jwt: access secret must be at least 32 bytes")
	}
	if len(cfg.RefreshSecret) < 32 {
		return nil, fmt.Errorf("jwt: refresh secret must be at least 32 bytes")
	}
	return &manager{cfg: cfg}, nil
}

func (m *manager) AccessTTL() time.Duration  { return m.cfg.AccessTTL }
func (m *manager) RefreshTTL() time.Duration { return m.cfg.RefreshTTL }

func (m *manager) GenerateAccess(claims Claims) (string, uuid.UUID, time.Time, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(m.cfg.AccessTTL)
	jti := uuid.New()

	claims.RegisteredClaims = jwtlib.RegisteredClaims{
		ID:        jti.String(),
		Subject:   claims.UserID.String(),
		Issuer:    m.cfg.Issuer,
		IssuedAt:  jwtlib.NewNumericDate(now),
		NotBefore: jwtlib.NewNumericDate(now),
		ExpiresAt: jwtlib.NewNumericDate(expiresAt),
	}

	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(m.cfg.AccessSecret))
	if err != nil {
		return "", uuid.Nil, time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return signed, jti, expiresAt, nil
}

func (m *manager) GenerateRefresh() (string, string, time.Time, error) {
	token, err := hash.RandomToken(48)
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("generate refresh token: %w", err)
	}
	// The refresh secret peppers the stored hash, so a database leak alone
	// is not enough to forge a lookup.
	tokenHash := hash.SHA256Hex(token + m.cfg.RefreshSecret)
	return token, tokenHash, time.Now().UTC().Add(m.cfg.RefreshTTL), nil
}

// HashRefresh reproduces the stored hash for a presented refresh token.
func HashRefresh(token, refreshSecret string) string {
	return hash.SHA256Hex(token + refreshSecret)
}

func (m *manager) ParseAccess(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwtlib.ParseWithClaims(tokenStr, claims, func(t *jwtlib.Token) (any, error) {
		if _, ok := t.Method.(*jwtlib.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return []byte(m.cfg.AccessSecret), nil
	}, jwtlib.WithIssuer(m.cfg.Issuer), jwtlib.WithExpirationRequired())

	if err != nil {
		switch {
		case errors.Is(err, jwtlib.ErrTokenExpired):
			return nil, apperror.ErrTokenExpired.Wrap(err)
		default:
			return nil, apperror.ErrTokenInvalid.Wrap(err)
		}
	}
	return claims, nil
}

// JTI returns the token identifier as a UUID.
func (c *Claims) JTI() (uuid.UUID, error) {
	return uuid.Parse(c.ID)
}

// HasPermission reports whether the token carries the given permission code.
func (c *Claims) HasPermission(code string) bool {
	return slices.Contains(c.Permissions, code)
}

// HasRole reports whether the token carries the given role code.
func (c *Claims) HasRole(code string) bool {
	return slices.Contains(c.Roles, code)
}
