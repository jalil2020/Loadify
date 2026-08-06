package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UserStatus is the account lifecycle state.
type UserStatus string

const (
	UserStatusPendingVerification UserStatus = "pending_verification"
	UserStatusActive              UserStatus = "active"
	UserStatusBlocked             UserStatus = "blocked"
	UserStatusDeleted             UserStatus = "deleted"
)

// Role codes. These match roles.code in the database and are the only role
// identifiers the code is allowed to reference.
const (
	RoleGuest          = "guest"
	RoleDriver         = "driver"
	RoleDispatcher     = "dispatcher"
	RoleCompanyManager = "company_manager"
	RoleAdmin          = "admin"
	RoleSuperAdmin     = "super_admin"
)

// User is a platform account. One account may hold several roles, optionally
// scoped to a company.
type User struct {
	ID                  uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Phone               string     `gorm:"size:20;not null"`
	Email               *string    `gorm:"size:255"`
	PasswordHash        string     `gorm:"size:255;not null"`
	FirstName           string     `gorm:"size:100;not null;default:''"`
	LastName            string     `gorm:"size:100;not null;default:''"`
	MiddleName          *string    `gorm:"size:100"`
	AvatarFileID        *uuid.UUID `gorm:"type:uuid"`
	BirthDate           *time.Time `gorm:"type:date"`
	Gender              *string    `gorm:"size:10"`
	Locale              Locale     `gorm:"size:5;not null;default:'uz'"`
	Status              UserStatus `gorm:"size:24;not null;default:'pending_verification'"`
	BlockedReason       *string
	BlockedAt           *time.Time
	BlockedBy           *uuid.UUID `gorm:"type:uuid"`
	PhoneVerifiedAt     *time.Time
	EmailVerifiedAt     *time.Time
	LastLoginAt         *time.Time
	LastLoginIP         *string `gorm:"type:inet"`
	FailedLoginAttempts int16   `gorm:"not null;default:0"`
	LockedUntil         *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletedAt           gorm.DeletedAt `gorm:"index"`

	Roles     []Role     `gorm:"many2many:user_roles;joinForeignKey:user_id;joinReferences:role_id"`
	UserRoles []UserRole `gorm:"foreignKey:UserID"`
	Avatar    *File      `gorm:"foreignKey:AvatarFileID"`
}

// FullName is the display name, falling back to the phone number for accounts
// that have not filled in their profile yet.
func (u *User) FullName() string {
	switch {
	case u.FirstName != "" && u.LastName != "":
		return u.FirstName + " " + u.LastName
	case u.FirstName != "":
		return u.FirstName
	case u.LastName != "":
		return u.LastName
	default:
		return u.Phone
	}
}

// IsActive reports whether the account may authenticate and act.
func (u *User) IsActive() bool { return u.Status == UserStatusActive }

// IsBlocked reports whether an administrator has blocked the account.
func (u *User) IsBlocked() bool { return u.Status == UserStatusBlocked }

// IsPhoneVerified reports whether the phone number has been confirmed by OTP.
func (u *User) IsPhoneVerified() bool { return u.PhoneVerifiedAt != nil }

// IsLocked reports whether failed-login lockout is currently in effect.
func (u *User) IsLocked(now time.Time) bool {
	return u.LockedUntil != nil && u.LockedUntil.After(now)
}

// Role is a named bundle of permissions.
type Role struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Code        string    `gorm:"size:32;not null;uniqueIndex"`
	Name        string    `gorm:"size:64;not null"`
	Description *string
	Level       int16 `gorm:"not null;default:0"`
	IsSystem    bool  `gorm:"not null;default:false"`
	CreatedAt   time.Time
	UpdatedAt   time.Time

	Permissions []Permission `gorm:"many2many:role_permissions;joinForeignKey:role_id;joinReferences:permission_id"`
}

// Permission is a single capability, addressed by its stable code.
type Permission struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Code        string    `gorm:"size:64;not null;uniqueIndex"`
	Resource    string    `gorm:"size:32;not null"`
	Action      string    `gorm:"size:32;not null"`
	Description *string
	CreatedAt   time.Time
}

// UserRole grants a role to a user, optionally scoped to one company.
type UserRole struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index"`
	RoleID    uuid.UUID  `gorm:"type:uuid;not null"`
	CompanyID *uuid.UUID `gorm:"type:uuid;index"`
	GrantedBy *uuid.UUID `gorm:"type:uuid"`
	ExpiresAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time

	Role *Role `gorm:"foreignKey:RoleID"`
}

// Session is one refresh-token grant tied to a device.
type Session struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID           uuid.UUID  `gorm:"type:uuid;not null;index"`
	RefreshTokenHash string     `gorm:"size:64;not null;uniqueIndex"`
	FamilyID         uuid.UUID  `gorm:"type:uuid;not null;index"`
	ReplacedBy       *uuid.UUID `gorm:"type:uuid"`
	DeviceID         *string    `gorm:"size:128"`
	DeviceName       *string    `gorm:"size:128"`
	Platform         Platform   `gorm:"size:16;not null;default:'web'"`
	AppVersion       *string    `gorm:"size:32"`
	IPAddress        *string    `gorm:"type:inet"`
	UserAgent        *string
	LastUsedAt       time.Time
	ExpiresAt        time.Time
	RevokedAt        *time.Time
	RevokedReason    *string `gorm:"size:64"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// IsUsable reports whether the session can still mint a new token pair.
func (s *Session) IsUsable(now time.Time) bool {
	return s.RevokedAt == nil && s.ExpiresAt.After(now)
}

// OTPPurpose distinguishes the flows a one-time code can authorise.
type OTPPurpose string

const (
	OTPPurposeRegister      OTPPurpose = "register"
	OTPPurposeLogin         OTPPurpose = "login"
	OTPPurposeResetPassword OTPPurpose = "reset_password"
	OTPPurposeChangePhone   OTPPurpose = "change_phone"
)

// OTPCode is a hashed one-time code issued to a phone number.
type OTPCode struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Phone       string     `gorm:"size:20;not null;index"`
	CodeHash    string     `gorm:"size:64;not null"`
	Purpose     OTPPurpose `gorm:"size:32;not null"`
	Attempts    int16      `gorm:"not null;default:0"`
	MaxAttempts int16      `gorm:"not null;default:5"`
	IPAddress   *string    `gorm:"type:inet"`
	ExpiresAt   time.Time
	ConsumedAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// IsExpired reports whether the code is past its TTL.
func (o *OTPCode) IsExpired(now time.Time) bool { return now.After(o.ExpiresAt) }

// IsConsumed reports whether the code has already been used.
func (o *OTPCode) IsConsumed() bool { return o.ConsumedAt != nil }

// AttemptsExhausted reports whether too many wrong guesses were made.
func (o *OTPCode) AttemptsExhausted() bool { return o.Attempts >= o.MaxAttempts }

// PasswordReset is a single-use token authorising a password change.
type PasswordReset struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index"`
	TokenHash string    `gorm:"size:64;not null;uniqueIndex"`
	IPAddress *string   `gorm:"type:inet"`
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// ─── ports ──────────────────────────────────────────────────────────

// UserFilter narrows an admin user listing.
type UserFilter struct {
	Search   string
	Status   []UserStatus
	RoleCode string
	Locale   string
	Created  TimeRange
}

// UserRepository persists accounts.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User) error
	FindByID(ctx context.Context, id uuid.UUID) (*User, error)
	FindByPhone(ctx context.Context, phone string) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	ExistsByPhone(ctx context.Context, phone string) (bool, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	List(ctx context.Context, filter UserFilter, offset, limit int, order string) ([]User, int64, error)
	UpdateColumns(ctx context.Context, id uuid.UUID, columns map[string]any) error
	SoftDelete(ctx context.Context, id uuid.UUID) error
}

// RoleRepository reads roles and permissions and manages grants.
type RoleRepository interface {
	FindByCode(ctx context.Context, code string) (*Role, error)
	ListRoles(ctx context.Context) ([]Role, error)
	ListPermissions(ctx context.Context) ([]Permission, error)
	AssignRole(ctx context.Context, userID, roleID uuid.UUID, companyID *uuid.UUID, grantedBy *uuid.UUID) error
	RevokeRole(ctx context.Context, userID, roleID uuid.UUID, companyID *uuid.UUID) error
	RolesOfUser(ctx context.Context, userID uuid.UUID) ([]UserRole, error)
	PermissionCodesOfUser(ctx context.Context, userID uuid.UUID) ([]string, error)
	SetRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error
}

// SessionRepository persists refresh-token grants.
type SessionRepository interface {
	Create(ctx context.Context, session *Session) error
	FindByTokenHash(ctx context.Context, tokenHash string) (*Session, error)
	FindByID(ctx context.Context, id uuid.UUID) (*Session, error)
	ListActiveByUser(ctx context.Context, userID uuid.UUID) ([]Session, error)
	Revoke(ctx context.Context, id uuid.UUID, reason string) error
	RevokeFamily(ctx context.Context, familyID uuid.UUID, reason string) error
	RevokeAllOfUser(ctx context.Context, userID uuid.UUID, reason string) error
	MarkReplaced(ctx context.Context, oldID, newID uuid.UUID) error
	TouchLastUsed(ctx context.Context, id uuid.UUID) error
	DeleteExpired(ctx context.Context, before time.Time) (int64, error)
}

// OTPRepository persists one-time codes.
type OTPRepository interface {
	Create(ctx context.Context, code *OTPCode) error
	FindLatestActive(ctx context.Context, phone string, purpose OTPPurpose) (*OTPCode, error)
	IncrementAttempts(ctx context.Context, id uuid.UUID) error
	Consume(ctx context.Context, id uuid.UUID) error
	InvalidateAll(ctx context.Context, phone string, purpose OTPPurpose) error
	DeleteExpired(ctx context.Context, before time.Time) (int64, error)
}

// PasswordResetRepository persists reset tokens.
type PasswordResetRepository interface {
	Create(ctx context.Context, reset *PasswordReset) error
	FindByTokenHash(ctx context.Context, tokenHash string) (*PasswordReset, error)
	MarkUsed(ctx context.Context, id uuid.UUID) error
	InvalidateForUser(ctx context.Context, userID uuid.UUID) error
}
