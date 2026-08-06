// Package usecase holds the business rules. A usecase depends only on domain
// ports, never on Fiber, GORM or Redis types.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
	"github.com/loadify/backend/internal/dto"
	"github.com/loadify/backend/internal/infrastructure/cache"
	"github.com/loadify/backend/internal/infrastructure/sms"
	"github.com/loadify/backend/pkg/apperror"
	"github.com/loadify/backend/pkg/hash"
	"github.com/loadify/backend/pkg/jwt"
	"github.com/loadify/backend/pkg/logger"
)

// DeviceInfo describes the client a session belongs to.
type DeviceInfo struct {
	DeviceID   string
	DeviceName string
	Platform   string
	AppVersion string
	IPAddress  string
	UserAgent  string
}

// AuthUsecase implements registration, authentication and session management.
type AuthUsecase struct {
	users    domain.UserRepository
	roles    domain.RoleRepository
	sessions domain.SessionRepository
	otps     domain.OTPRepository
	resets   domain.PasswordResetRepository
	identity domain.IdentityRepository
	tx       domain.TxManager
	audit    domain.AuditLogger
	tokens   jwt.Manager
	hasher   hash.Hasher
	sms      sms.Sender
	cache    cache.Cache
	cfg      *config.Config
	log      logger.Logger
}

// NewAuthUsecase wires the auth usecase.
func NewAuthUsecase(
	users domain.UserRepository,
	roles domain.RoleRepository,
	sessions domain.SessionRepository,
	otps domain.OTPRepository,
	resets domain.PasswordResetRepository,
	identity domain.IdentityRepository,
	tx domain.TxManager,
	audit domain.AuditLogger,
	tokens jwt.Manager,
	hasher hash.Hasher,
	smsSender sms.Sender,
	c cache.Cache,
	cfg *config.Config,
	log logger.Logger,
) *AuthUsecase {
	return &AuthUsecase{
		users: users, roles: roles, sessions: sessions, otps: otps, resets: resets,
		identity: identity, tx: tx, audit: audit, tokens: tokens, hasher: hasher,
		sms: smsSender, cache: c, cfg: cfg, log: log,
	}
}

// maxOTPRequestsPerHour caps how many codes a single phone can trigger, on top
// of the per-request cooldown. Without it, the cooldown alone still allows 60
// SMS an hour per number.
const maxOTPRequestsPerHour = 10

// Register creates an unverified account and dispatches the confirmation code.
// No tokens are issued until the phone is confirmed.
func (u *AuthUsecase) Register(ctx context.Context, req dto.RegisterRequest, device DeviceInfo) (*dto.RegisterResponse, error) {
	phone := normalisePhone(req.Phone)

	exists, err := u.users.ExistsByPhone(ctx, phone)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, apperror.ErrPhoneTaken
	}
	if req.Email != "" {
		emailTaken, err := u.users.ExistsByEmail(ctx, req.Email)
		if err != nil {
			return nil, err
		}
		if emailTaken {
			return nil, apperror.ErrEmailTaken
		}
	}

	if err := u.guardOTPRate(ctx, phone, domain.OTPPurposeRegister); err != nil {
		return nil, err
	}

	passwordHash, err := u.hasher.Hash(req.Password)
	if err != nil {
		return nil, apperror.ErrInternal.Wrap(err)
	}

	roleCode := domain.RoleDriver
	if req.Role == "company" {
		roleCode = domain.RoleCompanyManager
	}
	role, err := u.roles.FindByCode(ctx, roleCode)
	if err != nil {
		return nil, err
	}

	locale := domain.Locale(req.Locale)
	if locale == "" {
		locale = domain.Locale(u.cfg.App.DefaultLocale)
	}

	user := &domain.User{
		ID:           uuid.New(),
		Phone:        phone,
		PasswordHash: passwordHash,
		FirstName:    strings.TrimSpace(req.FirstName),
		LastName:     strings.TrimSpace(req.LastName),
		Locale:       locale,
		Status:       domain.UserStatusPendingVerification,
	}
	if req.Email != "" {
		email := strings.ToLower(strings.TrimSpace(req.Email))
		user.Email = &email
	}

	var otpExpiresAt time.Time
	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.users.Create(txCtx, user); err != nil {
			return err
		}
		if err := u.roles.AssignRole(txCtx, user.ID, role.ID, nil, nil); err != nil {
			return err
		}
		expiresAt, err := u.issueOTP(txCtx, phone, domain.OTPPurposeRegister, device.IPAddress, locale)
		if err != nil {
			return err
		}
		otpExpiresAt = expiresAt
		return nil
	})
	if err != nil {
		return nil, err
	}

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &user.ID,
		Action:       "auth.register",
		ResourceType: "user",
		ResourceID:   &user.ID,
		NewValues:    map[string]any{"phone": phone, "role": roleCode},
		IPAddress:    device.IPAddress,
		UserAgent:    device.UserAgent,
	})

	return &dto.RegisterResponse{
		UserID:       user.ID,
		Phone:        phone,
		OTPExpiresAt: otpExpiresAt,
		ResendAfter:  int(u.cfg.Security.OTPResendCooldown.Seconds()),
		Message:      "Confirmation code sent",
	}, nil
}

// VerifyOTP validates a code. For register and login it completes authentication;
// for reset_password it hands back a single-use reset token.
func (u *AuthUsecase) VerifyOTP(ctx context.Context, req dto.VerifyOTPRequest, device DeviceInfo) (*dto.VerifyOTPResponse, error) {
	phone := normalisePhone(req.Phone)
	purpose := domain.OTPPurpose(req.Purpose)

	if err := u.consumeOTP(ctx, phone, purpose, req.Code); err != nil {
		return nil, err
	}

	user, err := u.users.FindByPhone(ctx, phone)
	if err != nil {
		if apperror.Is(err, apperror.ErrNotFound.Code) {
			return nil, apperror.ErrOTPInvalid
		}
		return nil, err
	}
	if user.IsBlocked() {
		return nil, apperror.ErrAccountBlocked
	}

	switch purpose {
	case domain.OTPPurposeResetPassword:
		token, expiresAt, err := u.createResetToken(ctx, user.ID, device.IPAddress)
		if err != nil {
			return nil, err
		}
		return &dto.VerifyOTPResponse{Verified: true, ResetToken: token, ExpiresAt: &expiresAt}, nil

	case domain.OTPPurposeRegister, domain.OTPPurposeLogin, domain.OTPPurposeChangePhone:
		now := time.Now().UTC()
		columns := map[string]any{
			"phone_verified_at":     now,
			"failed_login_attempts": 0,
			"locked_until":          nil,
		}
		if user.Status == domain.UserStatusPendingVerification {
			columns["status"] = domain.UserStatusActive
			user.Status = domain.UserStatusActive
		}
		if err := u.users.UpdateColumns(ctx, user.ID, columns); err != nil {
			return nil, err
		}
		user.PhoneVerifiedAt = &now
		u.invalidateUserCache(ctx, user.ID)

		auth, err := u.issueSession(ctx, user, device)
		if err != nil {
			return nil, err
		}
		return &dto.VerifyOTPResponse{Verified: true, Auth: auth}, nil

	default:
		return nil, apperror.ErrBadRequest.WithMessage("Unknown verification purpose")
	}
}

// ResendOTP issues a fresh code, subject to the cooldown and hourly cap.
func (u *AuthUsecase) ResendOTP(ctx context.Context, req dto.ResendOTPRequest, device DeviceInfo) (*dto.OTPSentResponse, error) {
	phone := normalisePhone(req.Phone)
	purpose := domain.OTPPurpose(req.Purpose)

	if err := u.guardOTPRate(ctx, phone, purpose); err != nil {
		return nil, err
	}

	locale := domain.Locale(u.cfg.App.DefaultLocale)
	if user, err := u.users.FindByPhone(ctx, phone); err == nil {
		locale = user.Locale
		if user.IsBlocked() {
			return nil, apperror.ErrAccountBlocked
		}
	} else if !apperror.Is(err, apperror.ErrNotFound.Code) {
		return nil, err
	} else if purpose != domain.OTPPurposeRegister {
		// Do not disclose whether a number is registered: report success but send nothing.
		return &dto.OTPSentResponse{
			Phone:        phone,
			Purpose:      string(purpose),
			OTPExpiresAt: time.Now().UTC().Add(u.cfg.Security.OTPTTL),
			ResendAfter:  int(u.cfg.Security.OTPResendCooldown.Seconds()),
		}, nil
	}

	expiresAt, err := u.issueOTP(ctx, phone, purpose, device.IPAddress, locale)
	if err != nil {
		return nil, err
	}
	return &dto.OTPSentResponse{
		Phone:        phone,
		Purpose:      string(purpose),
		OTPExpiresAt: expiresAt,
		ResendAfter:  int(u.cfg.Security.OTPResendCooldown.Seconds()),
	}, nil
}

// Login authenticates with phone and password and opens a session.
func (u *AuthUsecase) Login(ctx context.Context, req dto.LoginRequest, device DeviceInfo) (*dto.AuthResponse, error) {
	phone := normalisePhone(req.Phone)

	user, err := u.users.FindByPhone(ctx, phone)
	if err != nil {
		if apperror.Is(err, apperror.ErrNotFound.Code) {
			// Spend comparable time so a missing account is not distinguishable
			// from a wrong password by response time alone.
			u.hasher.Verify(req.Password, dummyHash)
			return nil, apperror.ErrInvalidCredentials
		}
		return nil, err
	}

	now := time.Now().UTC()
	if user.IsLocked(now) {
		return nil, apperror.ErrAccountLocked.WithMessage(
			fmt.Sprintf("Too many failed attempts. Try again after %s UTC",
				user.LockedUntil.Format("15:04")))
	}
	if user.IsBlocked() {
		return nil, apperror.ErrAccountBlocked
	}

	if !u.hasher.Verify(req.Password, user.PasswordHash) {
		u.registerFailedLogin(ctx, user)
		return nil, apperror.ErrInvalidCredentials
	}

	if !user.IsPhoneVerified() {
		return nil, apperror.ErrPhoneNotVerified
	}

	device.Platform = firstNonEmpty(req.Platform, device.Platform)
	device.DeviceID = firstNonEmpty(req.DeviceID, device.DeviceID)
	device.DeviceName = firstNonEmpty(req.DeviceName, device.DeviceName)
	device.AppVersion = firstNonEmpty(req.AppVersion, device.AppVersion)

	auth, err := u.issueSession(ctx, user, device)
	if err != nil {
		return nil, err
	}

	columns := map[string]any{
		"last_login_at":         now,
		"failed_login_attempts": 0,
		"locked_until":          nil,
	}
	if device.IPAddress != "" {
		columns["last_login_ip"] = device.IPAddress
	}
	if err := u.users.UpdateColumns(ctx, user.ID, columns); err != nil {
		u.log.Warn("update login metadata failed", logger.Err(err))
	}
	u.invalidateUserCache(ctx, user.ID)

	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &user.ID,
		Action:       "auth.login",
		ResourceType: "user",
		ResourceID:   &user.ID,
		IPAddress:    device.IPAddress,
		UserAgent:    device.UserAgent,
	})
	return auth, nil
}

// Refresh rotates a refresh token. Presenting a token that was already rotated
// means a copy is in circulation, so the whole family is revoked.
func (u *AuthUsecase) Refresh(ctx context.Context, req dto.RefreshRequest, device DeviceInfo) (*dto.AuthResponse, error) {
	tokenHash := jwt.HashRefresh(req.RefreshToken, u.cfg.JWT.RefreshSecret)

	session, err := u.sessions.FindByTokenHash(ctx, tokenHash)
	if err != nil {
		if apperror.Is(err, apperror.ErrNotFound.Code) {
			return nil, apperror.ErrTokenInvalid
		}
		return nil, err
	}

	now := time.Now().UTC()
	if session.RevokedAt != nil {
		if err := u.sessions.RevokeFamily(ctx, session.FamilyID, "token_reuse_detected"); err != nil {
			u.log.Error("revoke session family failed", logger.Err(err))
		}
		u.log.Warn("refresh token reuse detected",
			logger.String("user_id", session.UserID.String()),
			logger.String("family_id", session.FamilyID.String()))
		_ = u.audit.Record(ctx, domain.AuditEntry{
			ActorUserID:  &session.UserID,
			Action:       "auth.token_reuse_detected",
			ResourceType: "session",
			ResourceID:   &session.ID,
			IPAddress:    device.IPAddress,
			UserAgent:    device.UserAgent,
		})
		return nil, apperror.ErrTokenReuse
	}
	if !session.ExpiresAt.After(now) {
		return nil, apperror.ErrTokenExpired
	}

	user, err := u.users.FindByID(ctx, session.UserID)
	if err != nil {
		return nil, err
	}
	if user.IsBlocked() {
		return nil, apperror.ErrAccountBlocked
	}

	device.DeviceID = firstNonEmpty(device.DeviceID, deref(session.DeviceID))
	device.DeviceName = firstNonEmpty(device.DeviceName, deref(session.DeviceName))
	device.Platform = firstNonEmpty(device.Platform, string(session.Platform))

	var auth *dto.AuthResponse
	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		newAuth, newSessionID, err := u.issueSessionInFamily(txCtx, user, device, session.FamilyID)
		if err != nil {
			return err
		}
		if err := u.sessions.Revoke(txCtx, session.ID, "rotated"); err != nil {
			return err
		}
		if err := u.sessions.MarkReplaced(txCtx, session.ID, newSessionID); err != nil {
			return err
		}
		auth = newAuth
		return nil
	})
	if err != nil {
		return nil, err
	}
	return auth, nil
}

// Logout revokes the presented refresh token and denylists the current access token.
func (u *AuthUsecase) Logout(ctx context.Context, claims *jwt.Claims, refreshToken string) error {
	if refreshToken != "" {
		tokenHash := jwt.HashRefresh(refreshToken, u.cfg.JWT.RefreshSecret)
		if session, err := u.sessions.FindByTokenHash(ctx, tokenHash); err == nil {
			if err := u.sessions.Revoke(ctx, session.ID, "logout"); err != nil {
				u.log.Warn("revoke session failed", logger.Err(err))
			}
		}
	} else if claims != nil && claims.SessionID != uuid.Nil {
		if err := u.sessions.Revoke(ctx, claims.SessionID, "logout"); err != nil {
			u.log.Warn("revoke session failed", logger.Err(err))
		}
	}

	if claims != nil {
		u.denylistAccessToken(ctx, claims)
		_ = u.audit.Record(ctx, domain.AuditEntry{
			ActorUserID:  &claims.UserID,
			Action:       "auth.logout",
			ResourceType: "user",
			ResourceID:   &claims.UserID,
		})
	}
	return nil
}

// LogoutAll revokes every session of the user.
func (u *AuthUsecase) LogoutAll(ctx context.Context, claims *jwt.Claims) error {
	if err := u.sessions.RevokeAllOfUser(ctx, claims.UserID, "logout_all"); err != nil {
		return err
	}
	u.denylistAccessToken(ctx, claims)
	u.invalidateUserCache(ctx, claims.UserID)
	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &claims.UserID,
		Action:       "auth.logout_all",
		ResourceType: "user",
		ResourceID:   &claims.UserID,
	})
	return nil
}

// ForgotPassword sends a reset code. The response is identical whether or not
// the number is registered, so the endpoint cannot be used to enumerate accounts.
func (u *AuthUsecase) ForgotPassword(ctx context.Context, req dto.ForgotPasswordRequest, device DeviceInfo) (*dto.OTPSentResponse, error) {
	phone := normalisePhone(req.Phone)
	generic := &dto.OTPSentResponse{
		Phone:        phone,
		Purpose:      string(domain.OTPPurposeResetPassword),
		OTPExpiresAt: time.Now().UTC().Add(u.cfg.Security.OTPTTL),
		ResendAfter:  int(u.cfg.Security.OTPResendCooldown.Seconds()),
	}

	user, err := u.users.FindByPhone(ctx, phone)
	if err != nil {
		if apperror.Is(err, apperror.ErrNotFound.Code) {
			return generic, nil
		}
		return nil, err
	}
	if user.IsBlocked() {
		return generic, nil
	}
	if err := u.guardOTPRate(ctx, phone, domain.OTPPurposeResetPassword); err != nil {
		return nil, err
	}

	expiresAt, err := u.issueOTP(ctx, phone, domain.OTPPurposeResetPassword, device.IPAddress, user.Locale)
	if err != nil {
		return nil, err
	}
	generic.OTPExpiresAt = expiresAt
	return generic, nil
}

// ResetPassword sets a new password using the token from OTP verification and
// signs every device out.
func (u *AuthUsecase) ResetPassword(ctx context.Context, req dto.ResetPasswordRequest, device DeviceInfo) error {
	tokenHash := hash.SHA256Hex(req.ResetToken + u.cfg.JWT.RefreshSecret)

	reset, err := u.resets.FindByTokenHash(ctx, tokenHash)
	if err != nil {
		if apperror.Is(err, apperror.ErrNotFound.Code) {
			return apperror.ErrTokenInvalid
		}
		return err
	}
	if reset.UsedAt != nil {
		return apperror.ErrTokenRevoked
	}
	if time.Now().UTC().After(reset.ExpiresAt) {
		return apperror.ErrTokenExpired
	}

	user, err := u.users.FindByID(ctx, reset.UserID)
	if err != nil {
		return err
	}
	if u.hasher.Verify(req.NewPassword, user.PasswordHash) {
		return apperror.ErrPasswordSame
	}

	newHash, err := u.hasher.Hash(req.NewPassword)
	if err != nil {
		return apperror.ErrInternal.Wrap(err)
	}

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.users.UpdateColumns(txCtx, user.ID, map[string]any{
			"password_hash":         newHash,
			"failed_login_attempts": 0,
			"locked_until":          nil,
		}); err != nil {
			return err
		}
		if err := u.resets.MarkUsed(txCtx, reset.ID); err != nil {
			return err
		}
		return u.sessions.RevokeAllOfUser(txCtx, user.ID, "password_reset")
	})
	if err != nil {
		return err
	}

	u.invalidateUserCache(ctx, user.ID)
	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &user.ID,
		Action:       "auth.password_reset",
		ResourceType: "user",
		ResourceID:   &user.ID,
		IPAddress:    device.IPAddress,
		UserAgent:    device.UserAgent,
	})
	return nil
}

// ChangePassword changes the password of the signed-in user and revokes all
// other sessions, keeping only the current one alive.
func (u *AuthUsecase) ChangePassword(ctx context.Context, claims *jwt.Claims, req dto.ChangePasswordRequest) error {
	user, err := u.users.FindByID(ctx, claims.UserID)
	if err != nil {
		return err
	}
	if !u.hasher.Verify(req.CurrentPassword, user.PasswordHash) {
		return apperror.ErrInvalidCredentials.WithMessage("Current password is incorrect")
	}
	if u.hasher.Verify(req.NewPassword, user.PasswordHash) {
		return apperror.ErrPasswordSame
	}

	newHash, err := u.hasher.Hash(req.NewPassword)
	if err != nil {
		return apperror.ErrInternal.Wrap(err)
	}

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.users.UpdateColumns(txCtx, user.ID, map[string]any{"password_hash": newHash}); err != nil {
			return err
		}
		return u.sessions.RevokeAllOfUser(txCtx, user.ID, "password_changed")
	})
	if err != nil {
		return err
	}

	u.invalidateUserCache(ctx, user.ID)
	_ = u.audit.Record(ctx, domain.AuditEntry{
		ActorUserID:  &user.ID,
		Action:       "auth.password_changed",
		ResourceType: "user",
		ResourceID:   &user.ID,
	})
	return nil
}

// Me returns the signed-in account with its effective roles and permissions.
func (u *AuthUsecase) Me(ctx context.Context, userID uuid.UUID) (*dto.UserResponse, error) {
	user, err := u.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return u.buildUserResponse(ctx, user)
}

// ListSessions returns the user's active devices.
func (u *AuthUsecase) ListSessions(ctx context.Context, claims *jwt.Claims) ([]dto.SessionResponse, error) {
	sessions, err := u.sessions.ListActiveByUser(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.SessionResponse, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, dto.SessionResponse{
			ID:         s.ID,
			DeviceName: s.DeviceName,
			Platform:   string(s.Platform),
			AppVersion: s.AppVersion,
			IPAddress:  s.IPAddress,
			LastUsedAt: s.LastUsedAt,
			CreatedAt:  s.CreatedAt,
			ExpiresAt:  s.ExpiresAt,
			Current:    s.ID == claims.SessionID,
		})
	}
	return out, nil
}

// RevokeSession ends one of the user's own sessions.
func (u *AuthUsecase) RevokeSession(ctx context.Context, claims *jwt.Claims, sessionID uuid.UUID) error {
	session, err := u.sessions.FindByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if session.UserID != claims.UserID {
		return apperror.ErrForbidden
	}
	return u.sessions.Revoke(ctx, sessionID, "revoked_by_user")
}

// ─── internals ──────────────────────────────────────────────────────

// dummyHash is a valid bcrypt hash of a value nobody knows. Comparing against
// it keeps the timing of "unknown phone" close to "wrong password".
const dummyHash = "$2a$12$K8HrH3aVzXQqYtG5kCBhaeVQ2h1H0qLxTn6qgOaZbGVzGZqfvbLQ2"

func (u *AuthUsecase) issueSession(ctx context.Context, user *domain.User, device DeviceInfo) (*dto.AuthResponse, error) {
	auth, _, err := u.issueSessionInFamily(ctx, user, device, uuid.New())
	return auth, err
}

func (u *AuthUsecase) issueSessionInFamily(ctx context.Context, user *domain.User, device DeviceInfo, familyID uuid.UUID) (*dto.AuthResponse, uuid.UUID, error) {
	permissions, err := u.permissionsOf(ctx, user.ID)
	if err != nil {
		return nil, uuid.Nil, err
	}
	roleCodes, err := u.roleCodesOf(ctx, user.ID)
	if err != nil {
		return nil, uuid.Nil, err
	}
	userCtx, err := u.identity.Resolve(ctx, user.ID)
	if err != nil {
		return nil, uuid.Nil, err
	}

	sessionID := uuid.New()
	accessToken, _, accessExpiresAt, err := u.tokens.GenerateAccess(jwt.Claims{
		UserID:      user.ID,
		Phone:       user.Phone,
		Roles:       roleCodes,
		Permissions: permissions,
		CompanyID:   userCtx.CompanyID,
		DriverID:    userCtx.DriverID,
		SessionID:   sessionID,
	})
	if err != nil {
		return nil, uuid.Nil, apperror.ErrInternal.Wrap(err)
	}

	refreshToken, refreshHash, refreshExpiresAt, err := u.tokens.GenerateRefresh()
	if err != nil {
		return nil, uuid.Nil, apperror.ErrInternal.Wrap(err)
	}

	session := &domain.Session{
		ID:               sessionID,
		UserID:           user.ID,
		RefreshTokenHash: refreshHash,
		FamilyID:         familyID,
		Platform:         domain.ParsePlatform(device.Platform),
		LastUsedAt:       time.Now().UTC(),
		ExpiresAt:        refreshExpiresAt,
	}
	session.DeviceID = nilIfBlank(device.DeviceID)
	session.DeviceName = nilIfBlank(device.DeviceName)
	session.AppVersion = nilIfBlank(device.AppVersion)
	session.IPAddress = nilIfBlank(device.IPAddress)
	session.UserAgent = nilIfBlank(device.UserAgent)

	if err := u.sessions.Create(ctx, session); err != nil {
		return nil, uuid.Nil, err
	}

	userResp, err := u.buildUserResponseWith(ctx, user, roleCodes, permissions, userCtx)
	if err != nil {
		return nil, uuid.Nil, err
	}

	return &dto.AuthResponse{
		User: *userResp,
		Tokens: dto.TokenResponse{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			TokenType:    "Bearer",
			ExpiresIn:    int64(u.tokens.AccessTTL().Seconds()),
			ExpiresAt:    accessExpiresAt,
		},
	}, sessionID, nil
}

func (u *AuthUsecase) buildUserResponse(ctx context.Context, user *domain.User) (*dto.UserResponse, error) {
	roleCodes, err := u.roleCodesOf(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	permissions, err := u.permissionsOf(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	userCtx, err := u.identity.Resolve(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	return u.buildUserResponseWith(ctx, user, roleCodes, permissions, userCtx)
}

func (u *AuthUsecase) buildUserResponseWith(_ context.Context, user *domain.User, roles, permissions []string, userCtx domain.UserContext) (*dto.UserResponse, error) {
	return &dto.UserResponse{
		ID:            user.ID,
		Phone:         user.Phone,
		Email:         user.Email,
		FirstName:     user.FirstName,
		LastName:      user.LastName,
		FullName:      user.FullName(),
		Locale:        string(user.Locale),
		Status:        string(user.Status),
		PhoneVerified: user.IsPhoneVerified(),
		Roles:         roles,
		Permissions:   permissions,
		DriverID:      userCtx.DriverID,
		CompanyID:     userCtx.CompanyID,
		CompanyRole:   userCtx.CompanyRole,
		CreatedAt:     user.CreatedAt,
		LastLoginAt:   user.LastLoginAt,
	}, nil
}

func (u *AuthUsecase) permissionsOf(ctx context.Context, userID uuid.UUID) ([]string, error) {
	key := cache.UserPermissionsKey(userID)
	var cached []string
	if err := u.cache.Get(ctx, key, &cached); err == nil {
		return cached, nil
	} else if !errors.Is(err, cache.ErrNotFound) {
		u.log.Warn("permission cache read failed", logger.Err(err))
	}

	codes, err := u.roles.PermissionCodesOfUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := u.cache.Set(ctx, key, codes, 10*time.Minute); err != nil {
		u.log.Warn("permission cache write failed", logger.Err(err))
	}
	return codes, nil
}

func (u *AuthUsecase) roleCodesOf(ctx context.Context, userID uuid.UUID) ([]string, error) {
	grants, err := u.roles.RolesOfUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	codes := make([]string, 0, len(grants))
	for _, g := range grants {
		if g.Role != nil {
			codes = append(codes, g.Role.Code)
		}
	}
	return codes, nil
}

// issueOTP invalidates any live code, stores the hash of a new one and sends the SMS.
func (u *AuthUsecase) issueOTP(ctx context.Context, phone string, purpose domain.OTPPurpose, ip string, locale domain.Locale) (time.Time, error) {
	if err := u.otps.InvalidateAll(ctx, phone, purpose); err != nil {
		return time.Time{}, err
	}

	code, err := hash.NumericCode(u.cfg.Security.OTPLength)
	if err != nil {
		return time.Time{}, apperror.ErrInternal.Wrap(err)
	}

	expiresAt := time.Now().UTC().Add(u.cfg.Security.OTPTTL)
	record := &domain.OTPCode{
		ID:          uuid.New(),
		Phone:       phone,
		CodeHash:    hash.SHA256Hex(code),
		Purpose:     purpose,
		MaxAttempts: int16(u.cfg.Security.OTPMaxAttempts),
		ExpiresAt:   expiresAt,
		IPAddress:   nilIfBlank(ip),
	}
	if err := u.otps.Create(ctx, record); err != nil {
		return time.Time{}, err
	}

	// Sending must not roll back the transaction that created the code: a
	// delivery failure is retryable by the user, a lost code row is not.
	if err := u.sms.Send(ctx, phone, otpMessage(code, locale, int(u.cfg.Security.OTPTTL.Minutes()))); err != nil {
		u.log.Error("otp sms delivery failed",
			logger.String("phone", maskPhone(phone)), logger.Err(err))
	}

	if err := u.cache.Set(ctx,
		cache.OTPCooldownKey(phone, string(purpose)), 1, u.cfg.Security.OTPResendCooldown); err != nil {
		u.log.Warn("otp cooldown write failed", logger.Err(err))
	}
	if _, err := u.cache.Incr(ctx, cache.OTPRateKey(phone), time.Hour); err != nil {
		u.log.Warn("otp rate counter failed", logger.Err(err))
	}
	return expiresAt, nil
}

func (u *AuthUsecase) consumeOTP(ctx context.Context, phone string, purpose domain.OTPPurpose, code string) error {
	record, err := u.otps.FindLatestActive(ctx, phone, purpose)
	if err != nil {
		if apperror.Is(err, apperror.ErrNotFound.Code) {
			return apperror.ErrOTPExpired
		}
		return err
	}
	now := time.Now().UTC()
	if record.IsExpired(now) {
		return apperror.ErrOTPExpired
	}
	if record.AttemptsExhausted() {
		return apperror.ErrOTPAttempts
	}
	if !hash.ConstantTimeEqual(record.CodeHash, hash.SHA256Hex(code)) {
		if err := u.otps.IncrementAttempts(ctx, record.ID); err != nil {
			u.log.Warn("otp attempt increment failed", logger.Err(err))
		}
		if record.Attempts+1 >= record.MaxAttempts {
			return apperror.ErrOTPAttempts
		}
		return apperror.ErrOTPInvalid
	}
	return u.otps.Consume(ctx, record.ID)
}

// guardOTPRate enforces the resend cooldown and the hourly ceiling per phone.
func (u *AuthUsecase) guardOTPRate(ctx context.Context, phone string, purpose domain.OTPPurpose) error {
	cooldownKey := cache.OTPCooldownKey(phone, string(purpose))
	active, err := u.cache.Exists(ctx, cooldownKey)
	if err != nil {
		u.log.Warn("otp cooldown check failed", logger.Err(err))
	} else if active {
		wait, _ := u.cache.TTL(ctx, cooldownKey)
		return apperror.ErrOTPCooldown.WithMessage(
			fmt.Sprintf("Request another code in %d seconds", int(wait.Seconds())+1))
	}

	count, err := u.cache.Incr(ctx, cache.OTPRateKey(phone), time.Hour)
	if err != nil {
		u.log.Warn("otp rate check failed", logger.Err(err))
		return nil
	}
	// Incr counts this request; issueOTP increments again on success, so the
	// effective ceiling is maxOTPRequestsPerHour codes actually sent.
	if count > maxOTPRequestsPerHour*2 {
		return apperror.ErrRateLimited.WithMessage("Too many code requests for this number, try again in an hour")
	}
	return nil
}

func (u *AuthUsecase) createResetToken(ctx context.Context, userID uuid.UUID, ip string) (string, time.Time, error) {
	token, err := hash.RandomToken(32)
	if err != nil {
		return "", time.Time{}, apperror.ErrInternal.Wrap(err)
	}
	expiresAt := time.Now().UTC().Add(u.cfg.Security.PasswordResetTTL)

	err = u.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := u.resets.InvalidateForUser(txCtx, userID); err != nil {
			return err
		}
		return u.resets.Create(txCtx, &domain.PasswordReset{
			ID:        uuid.New(),
			UserID:    userID,
			TokenHash: hash.SHA256Hex(token + u.cfg.JWT.RefreshSecret),
			IPAddress: nilIfBlank(ip),
			ExpiresAt: expiresAt,
		})
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

func (u *AuthUsecase) registerFailedLogin(ctx context.Context, user *domain.User) {
	attempts := user.FailedLoginAttempts + 1
	columns := map[string]any{"failed_login_attempts": attempts}
	if int(attempts) >= u.cfg.Security.LoginMaxFailed {
		columns["locked_until"] = time.Now().UTC().Add(u.cfg.Security.LoginLockoutDuration)
		columns["failed_login_attempts"] = 0
		u.log.Warn("account locked after failed logins",
			logger.String("user_id", user.ID.String()))
	}
	if err := u.users.UpdateColumns(ctx, user.ID, columns); err != nil {
		u.log.Warn("record failed login attempt failed", logger.Err(err))
	}
	u.invalidateUserCache(ctx, user.ID)
}

// denylistAccessToken makes the still-valid access token unusable for the rest
// of its lifetime.
func (u *AuthUsecase) denylistAccessToken(ctx context.Context, claims *jwt.Claims) {
	jti, err := claims.JTI()
	if err != nil {
		return
	}
	ttl := time.Until(claims.ExpiresAt.Time)
	if ttl <= 0 {
		return
	}
	if err := u.cache.Set(ctx, cache.DenylistKey(jti), true, ttl); err != nil {
		u.log.Warn("denylist access token failed", logger.Err(err))
	}
}

func (u *AuthUsecase) invalidateUserCache(ctx context.Context, userID uuid.UUID) {
	if err := u.cache.Delete(ctx,
		cache.UserKey(userID), cache.UserPermissionsKey(userID)); err != nil {
		u.log.Warn("user cache invalidation failed", logger.Err(err))
	}
}

// ─── helpers ────────────────────────────────────────────────────────

// normalisePhone strips formatting so +998 (90) 123-45-67 and +998901234567
// are the same account.
func normalisePhone(phone string) string {
	var b strings.Builder
	for i, r := range phone {
		switch {
		case r == '+' && i == 0:
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	out := b.String()
	if !strings.HasPrefix(out, "+") && len(out) > 0 {
		out = "+" + out
	}
	return out
}

// maskPhone hides the subscriber digits before a number reaches the logs.
func maskPhone(phone string) string {
	if len(phone) < 6 {
		return "***"
	}
	return phone[:len(phone)-6] + "****" + phone[len(phone)-2:]
}

func otpMessage(code string, locale domain.Locale, minutes int) string {
	switch locale {
	case domain.LocaleRU:
		return fmt.Sprintf("Loadify: код подтверждения %s. Действителен %d минут. Никому не сообщайте его.", code, minutes)
	case domain.LocaleEN:
		return fmt.Sprintf("Loadify: your confirmation code is %s. Valid for %d minutes. Do not share it.", code, minutes)
	default:
		return fmt.Sprintf("Loadify: tasdiqlash kodi %s. %d daqiqa amal qiladi. Hech kimga aytmang.", code, minutes)
	}
}

func nilIfBlank(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
