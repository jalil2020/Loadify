package dto

import (
	"time"

	"github.com/google/uuid"
)

// ─── users ──────────────────────────────────────────────────────────

// AdminUserResponse is an account as an administrator sees it.
type AdminUserResponse struct {
	ID              uuid.UUID  `json:"id"`
	Phone           string     `json:"phone"`
	Email           *string    `json:"email"`
	FullName        string     `json:"full_name"`
	AvatarURL       *string    `json:"avatar_url"`
	Locale          string     `json:"locale"`
	Status          string     `json:"status"`
	BlockedReason   *string    `json:"blocked_reason"`
	Roles           []string   `json:"roles"`
	PhoneVerifiedAt *time.Time `json:"phone_verified_at"`
	LastLoginAt     *time.Time `json:"last_login_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

// BlockUserRequest suspends an account.
type BlockUserRequest struct {
	Reason string `json:"reason" validate:"required,min=3,max=500"`
}

// AssignRoleRequest grants or revokes a platform role.
type AssignRoleRequest struct {
	RoleCode  string     `json:"role_code" validate:"required,oneof=driver dispatcher company_manager admin super_admin"`
	CompanyID *uuid.UUID `json:"company_id" validate:"omitempty"`
}

// UserSearchQuery filters the admin user directory.
type UserSearchQuery struct {
	Search string   `query:"q" validate:"omitempty,max=100"`
	Status string   `query:"status" validate:"omitempty,oneof=pending_verification active blocked deleted"`
	Roles  []string `query:"role" validate:"omitempty,dive,max=32"`
}

// ─── complaints ─────────────────────────────────────────────────────

// FileComplaintRequest reports a problem with another party.
type FileComplaintRequest struct {
	TargetType      string      `json:"target_type" validate:"required,oneof=user driver company load trip"`
	TargetID        uuid.UUID   `json:"target_id" validate:"required"`
	TripID          *uuid.UUID  `json:"trip_id" validate:"omitempty"`
	Category        string      `json:"category" validate:"required,oneof=fraud no_show damage payment behaviour documents other"`
	Description     string      `json:"description" validate:"required,min=10,max=4000"`
	EvidenceFileIDs []uuid.UUID `json:"evidence_file_ids" validate:"omitempty,max=10"`
}

// AssignComplaintRequest puts a complaint on a moderator's desk.
type AssignComplaintRequest struct {
	AssigneeID uuid.UUID `json:"assignee_id" validate:"required"`
}

// ResolveComplaintRequest closes a complaint.
type ResolveComplaintRequest struct {
	Upheld     bool   `json:"upheld"`
	Resolution string `json:"resolution" validate:"required,min=3,max=2000"`
	// Settlement optionally moves the disputed trip's escrow in the same action:
	// "refund" returns it to the shipper, "release" pays the driver, "" leaves
	// the money where it is.
	Settlement string `json:"settlement" validate:"omitempty,oneof=refund release"`
}

// ComplaintSearchQuery filters the moderation queue.
type ComplaintSearchQuery struct {
	Statuses   []string `query:"status" validate:"omitempty,dive,oneof=open in_review resolved rejected"`
	Category   string   `query:"category" validate:"omitempty,max=24"`
	Priority   string   `query:"priority" validate:"omitempty,oneof=low medium high critical"`
	TargetType string   `query:"target_type" validate:"omitempty,oneof=user driver company load trip"`
	AssignedTo string   `query:"assigned_to" validate:"omitempty,uuid"`
	OpenOnly   bool     `query:"open_only"`
}

// ComplaintResponse is one report.
type ComplaintResponse struct {
	ID           uuid.UUID      `json:"id"`
	ReporterID   uuid.UUID      `json:"reporter_id"`
	ReporterName string         `json:"reporter_name,omitempty"`
	TargetType   string         `json:"target_type"`
	TargetID     uuid.UUID      `json:"target_id"`
	TripID       *uuid.UUID     `json:"trip_id"`
	Category     string         `json:"category"`
	Description  string         `json:"description"`
	Evidence     []FileResponse `json:"evidence"`
	Status       string         `json:"status"`
	Priority     string         `json:"priority"`
	AssignedTo   *uuid.UUID     `json:"assigned_to"`
	Resolution   *string        `json:"resolution"`
	ResolvedAt   *time.Time     `json:"resolved_at"`
	CreatedAt    time.Time      `json:"created_at"`
}

// ─── audit ──────────────────────────────────────────────────────────

// AuditSearchQuery filters the audit trail.
type AuditSearchQuery struct {
	ActorUserID  string     `query:"actor_user_id" validate:"omitempty,uuid"`
	Action       string     `query:"action" validate:"omitempty,max=64"`
	ResourceType string     `query:"resource_type" validate:"omitempty,max=48"`
	ResourceID   string     `query:"resource_id" validate:"omitempty,uuid"`
	From         *time.Time `query:"from"`
	To           *time.Time `query:"to"`
}

// AuditLogResponse is one recorded action.
type AuditLogResponse struct {
	ID           int64      `json:"id"`
	ActorID      *uuid.UUID `json:"actor_id"`
	ActorName    string     `json:"actor_name,omitempty"`
	ActorRole    string     `json:"actor_role,omitempty"`
	Action       string     `json:"action"`
	ResourceType string     `json:"resource_type"`
	ResourceID   *uuid.UUID `json:"resource_id"`
	OldValues    string     `json:"old_values,omitempty"`
	NewValues    string     `json:"new_values,omitempty"`
	Description  *string    `json:"description"`
	IPAddress    string     `json:"ip_address,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// ─── settings ───────────────────────────────────────────────────────

// UpdateSettingRequest changes one tunable value.
type UpdateSettingRequest struct {
	Value any `json:"value" validate:"required"`
}

// SettingResponse is one operator-tunable value.
type SettingResponse struct {
	Key         string    `json:"key"`
	Value       string    `json:"value"`
	ValueType   string    `json:"value_type"`
	Group       string    `json:"group"`
	Description *string   `json:"description"`
	IsPublic    bool      `json:"is_public"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SetFeatureFlagRequest toggles a capability.
type SetFeatureFlagRequest struct {
	IsEnabled      bool `json:"is_enabled"`
	RolloutPercent int  `json:"rollout_percent" validate:"omitempty,gte=0,lte=100"`
}

// FeatureFlagResponse is one capability switch.
type FeatureFlagResponse struct {
	Key            string  `json:"key"`
	Description    *string `json:"description"`
	IsEnabled      bool    `json:"is_enabled"`
	RolloutPercent int16   `json:"rollout_percent"`
}

// ─── reports ────────────────────────────────────────────────────────

// ReportQuery bounds a report to a period.
type ReportQuery struct {
	From *time.Time `query:"from"`
	To   *time.Time `query:"to"`
}
