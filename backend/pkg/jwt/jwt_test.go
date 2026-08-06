package jwt

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/pkg/apperror"
)

const (
	testAccessSecret  = "access-secret-that-is-long-enough-for-hs256"
	testRefreshSecret = "refresh-secret-that-is-long-enough-for-hs256"
)

func newTestManager(t *testing.T, accessTTL time.Duration) Manager {
	t.Helper()
	m, err := New(Config{
		AccessSecret:  testAccessSecret,
		RefreshSecret: testRefreshSecret,
		AccessTTL:     accessTTL,
		RefreshTTL:    720 * time.Hour,
		Issuer:        "loadify-test",
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return m
}

func TestNewRejectsShortSecrets(t *testing.T) {
	_, err := New(Config{AccessSecret: "short", RefreshSecret: testRefreshSecret})
	if err == nil {
		t.Error("New() accepted a short access secret")
	}
	_, err = New(Config{AccessSecret: testAccessSecret, RefreshSecret: "short"})
	if err == nil {
		t.Error("New() accepted a short refresh secret")
	}
}

func TestGenerateAndParseAccess(t *testing.T) {
	m := newTestManager(t, 15*time.Minute)

	userID := uuid.New()
	companyID := uuid.New()
	sessionID := uuid.New()

	token, jti, expiresAt, err := m.GenerateAccess(Claims{
		UserID:      userID,
		Phone:       "+998901234567",
		Roles:       []string{"driver"},
		Permissions: []string{"loads.read", "bookings.create"},
		CompanyID:   &companyID,
		SessionID:   sessionID,
	})
	if err != nil {
		t.Fatalf("GenerateAccess() error: %v", err)
	}
	if jti == uuid.Nil {
		t.Error("GenerateAccess() returned a nil JTI")
	}
	if !expiresAt.After(time.Now()) {
		t.Errorf("expiresAt %v is not in the future", expiresAt)
	}

	claims, err := m.ParseAccess(token)
	if err != nil {
		t.Fatalf("ParseAccess() error: %v", err)
	}
	if claims.UserID != userID {
		t.Errorf("UserID = %v, want %v", claims.UserID, userID)
	}
	if claims.SessionID != sessionID {
		t.Errorf("SessionID = %v, want %v", claims.SessionID, sessionID)
	}
	if claims.CompanyID == nil || *claims.CompanyID != companyID {
		t.Errorf("CompanyID = %v, want %v", claims.CompanyID, companyID)
	}
	if !claims.HasRole("driver") || claims.HasRole("admin") {
		t.Errorf("roles = %v", claims.Roles)
	}
	if !claims.HasPermission("loads.read") || claims.HasPermission("loads.create") {
		t.Errorf("permissions = %v", claims.Permissions)
	}
	if gotJTI, err := claims.JTI(); err != nil || gotJTI != jti {
		t.Errorf("JTI() = %v, %v, want %v", gotJTI, err, jti)
	}
}

func TestParseAccessRejectsExpired(t *testing.T) {
	m := newTestManager(t, -time.Minute)

	token, _, _, err := m.GenerateAccess(Claims{UserID: uuid.New(), SessionID: uuid.New()})
	if err != nil {
		t.Fatalf("GenerateAccess() error: %v", err)
	}

	_, err = m.ParseAccess(token)
	if err == nil {
		t.Fatal("ParseAccess() accepted an expired token")
	}
	if !apperror.Is(err, apperror.ErrTokenExpired.Code) {
		t.Errorf("error code = %v, want %s", err, apperror.ErrTokenExpired.Code)
	}
}

func TestParseAccessRejectsWrongSignature(t *testing.T) {
	m := newTestManager(t, 15*time.Minute)
	other, err := New(Config{
		AccessSecret:  "a-completely-different-secret-of-sufficient-length",
		RefreshSecret: testRefreshSecret,
		AccessTTL:     15 * time.Minute,
		Issuer:        "loadify-test",
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	token, _, _, err := other.GenerateAccess(Claims{UserID: uuid.New(), SessionID: uuid.New()})
	if err != nil {
		t.Fatalf("GenerateAccess() error: %v", err)
	}

	if _, err := m.ParseAccess(token); err == nil {
		t.Error("ParseAccess() accepted a token signed with another key")
	}
}

func TestParseAccessRejectsWrongIssuer(t *testing.T) {
	other, err := New(Config{
		AccessSecret:  testAccessSecret,
		RefreshSecret: testRefreshSecret,
		AccessTTL:     15 * time.Minute,
		Issuer:        "somebody-else",
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	token, _, _, err := other.GenerateAccess(Claims{UserID: uuid.New(), SessionID: uuid.New()})
	if err != nil {
		t.Fatalf("GenerateAccess() error: %v", err)
	}

	m := newTestManager(t, 15*time.Minute)
	if _, err := m.ParseAccess(token); err == nil {
		t.Error("ParseAccess() accepted a token from another issuer")
	}
}

func TestParseAccessRejectsGarbage(t *testing.T) {
	m := newTestManager(t, 15*time.Minute)
	for _, token := range []string{"", "not-a-jwt", "a.b.c", strings.Repeat("x", 500)} {
		if _, err := m.ParseAccess(token); err == nil {
			t.Errorf("ParseAccess(%q) returned no error", token)
		}
	}
}

func TestGenerateRefreshIsUniqueAndHashed(t *testing.T) {
	m := newTestManager(t, 15*time.Minute)

	token1, hash1, expiresAt, err := m.GenerateRefresh()
	if err != nil {
		t.Fatalf("GenerateRefresh() error: %v", err)
	}
	token2, hash2, _, err := m.GenerateRefresh()
	if err != nil {
		t.Fatalf("GenerateRefresh() error: %v", err)
	}

	if token1 == token2 || hash1 == hash2 {
		t.Error("GenerateRefresh() produced identical tokens on consecutive calls")
	}
	if len(hash1) != 64 {
		t.Errorf("hash length = %d, want 64 (sha256 hex)", len(hash1))
	}
	if strings.Contains(hash1, token1) {
		t.Error("the stored hash contains the raw token")
	}
	if !expiresAt.After(time.Now().Add(700 * time.Hour)) {
		t.Errorf("refresh expiry %v is earlier than the configured TTL", expiresAt)
	}

	// The lookup hash must be reproducible from the presented token.
	if HashRefresh(token1, testRefreshSecret) != hash1 {
		t.Error("HashRefresh() does not reproduce the stored hash")
	}
	// ...and must depend on the pepper.
	if HashRefresh(token1, "another-secret") == hash1 {
		t.Error("HashRefresh() ignores the refresh secret")
	}
}
