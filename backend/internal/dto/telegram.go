package dto

import (
	"time"

	"github.com/google/uuid"
)

// StartTelegramLoginRequest begins connecting a person's own Telegram account.
//
// ConsentVersion is required and must match the text the person was shown.
// Connecting an account lets the platform post as that person, and Telegram's
// terms do not allow acting for someone without their knowledge — so the
// agreement is recorded with the version of what they agreed to.
type StartTelegramLoginRequest struct {
	Phone          string `json:"phone" validate:"required,phone"`
	ConsentVersion string `json:"consent_version" validate:"required,max=16"`
	Label          string `json:"label" validate:"omitempty,max=64"`
}

// TelegramCodeRequest carries the code Telegram sent to the person's app.
type TelegramCodeRequest struct {
	Code string `json:"code" validate:"required,min=3,max=10"`
}

// TelegramPasswordRequest carries the two-step verification password. It is
// used once, for the call that needs it, and never stored or logged.
type TelegramPasswordRequest struct {
	Password string `json:"password" validate:"required,max=256"`
}

// TelegramLoginStep says what the person has to do next.
type TelegramLoginStep struct {
	AccountID uuid.UUID `json:"account_id"`
	// Step is code, password or connected.
	Step string `json:"step"`
}

// TelegramAccountResponse is a connected account, without anything that could
// be used to act as it.
type TelegramAccountResponse struct {
	ID           uuid.UUID  `json:"id"`
	UserID       uuid.UUID  `json:"user_id"`
	Mine         bool       `json:"mine"`
	Phone        string     `json:"phone"`
	Label        string     `json:"label,omitempty"`
	Status       string     `json:"status"`
	StatusReason *string    `json:"status_reason,omitempty"`
	PausedUntil  *time.Time `json:"paused_until,omitempty"`
	ConsentAt    *time.Time `json:"consent_at,omitempty"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// TelegramDialogResponse is one group the account could post to.
type TelegramDialogResponse struct {
	// PeerID is sent as a string: -100… ids exceed what a JavaScript number
	// holds exactly, and a rounded id would name a different group.
	PeerID          string     `json:"peer_id"`
	Title           string     `json:"title"`
	Username        *string    `json:"username,omitempty"`
	Kind            string     `json:"kind"`
	MemberCount     *int       `json:"member_count,omitempty"`
	SlowmodeEnabled bool       `json:"slowmode_enabled"`
	CanSend         bool       `json:"can_send"`
	GroupID         *uuid.UUID `json:"group_id,omitempty"`
}

// ConnectGroupRequest adds a group from the account's own dialog list.
type ConnectGroupRequest struct {
	AccountID uuid.UUID `json:"account_id" validate:"required"`
	PeerID    string    `json:"peer_id" validate:"required,max=24"`
	Language  string    `json:"language" validate:"omitempty,oneof=uz ru en"`
}

// GroupRouteRequest is one corridor a group serves. A country without a region
// means the whole country.
type GroupRouteRequest struct {
	OriginCountry  *string    `json:"origin_country" validate:"omitempty,len=2"`
	OriginRegionID *uuid.UUID `json:"origin_region_id"`
	DestCountry    *string    `json:"destination_country" validate:"omitempty,len=2"`
	DestRegionID   *uuid.UUID `json:"destination_region_id"`
}

// SaveGroupRulesRequest is what a person says a group accepts.
//
// The limits are bounded here as well as by the platform: a group rule may make
// posting rarer, never more frequent.
type SaveGroupRulesRequest struct {
	Language        *string             `json:"language" validate:"omitempty,oneof=uz ru en"`
	PostingEnabled  *bool               `json:"posting_enabled"`
	CargoTypeCodes  []string            `json:"cargo_type_codes" validate:"omitempty,max=20,dive,max=32"`
	VehicleTypes    []string            `json:"vehicle_types" validate:"omitempty,max=12,dive,max=24"`
	MinWeightKg     *int                `json:"min_weight_kg" validate:"omitempty,gt=0,lte=100000"`
	MaxWeightKg     *int                `json:"max_weight_kg" validate:"omitempty,gt=0,lte=100000"`
	HourlyLimit     int16               `json:"hourly_limit" validate:"omitempty,min=1,max=10"`
	DailyLimit      int16               `json:"daily_limit" validate:"omitempty,min=1,max=20"`
	CooldownMinutes *int                `json:"cooldown_minutes" validate:"omitempty,min=0,max=10080"`
	QuietFromHour   *int16              `json:"quiet_from_hour" validate:"omitempty,min=0,max=23"`
	QuietToHour     *int16              `json:"quiet_to_hour" validate:"omitempty,min=0,max=23"`
	Notes           *string             `json:"notes" validate:"omitempty,max=500"`
	Routes          []GroupRouteRequest `json:"routes" validate:"omitempty,max=30,dive"`
}

// GroupRulesResponse mirrors the request.
type GroupRulesResponse struct {
	CargoTypeCodes  []string `json:"cargo_type_codes"`
	VehicleTypes    []string `json:"vehicle_types"`
	MinWeightKg     *int     `json:"min_weight_kg,omitempty"`
	MaxWeightKg     *int     `json:"max_weight_kg,omitempty"`
	HourlyLimit     int16    `json:"hourly_limit"`
	DailyLimit      int16    `json:"daily_limit"`
	CooldownMinutes int      `json:"cooldown_minutes"`
	QuietFromHour   *int16   `json:"quiet_from_hour,omitempty"`
	QuietToHour     *int16   `json:"quiet_to_hour,omitempty"`
	Notes           *string  `json:"notes,omitempty"`
}

// TelegramGroupResponse is a connected group with its rules.
type TelegramGroupResponse struct {
	ID              uuid.UUID           `json:"id"`
	AccountID       uuid.UUID           `json:"account_id"`
	PeerID          string              `json:"peer_id"`
	Title           string              `json:"title"`
	Username        *string             `json:"username,omitempty"`
	Kind            string              `json:"kind"`
	Language        string              `json:"language"`
	MemberCount     *int                `json:"member_count,omitempty"`
	SlowmodeSeconds int                 `json:"slowmode_seconds"`
	CanSend         bool                `json:"can_send"`
	PostingEnabled  bool                `json:"posting_enabled"`
	Active          bool                `json:"active"`
	InactiveReason  *string             `json:"inactive_reason,omitempty"`
	LastPostedAt    *time.Time          `json:"last_posted_at,omitempty"`
	Rules           *GroupRulesResponse `json:"rules,omitempty"`
	Routes          []GroupRouteRequest `json:"routes,omitempty"`
}
