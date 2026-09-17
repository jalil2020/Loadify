package domain

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TelegramAccountStatus is what the platform believes about a connected
// account. Every value but Active means posting is off, and each says why.
type TelegramAccountStatus string

// Account states.
const (
	TelegramAccountConnecting TelegramAccountStatus = "connecting"
	TelegramAccountActive     TelegramAccountStatus = "active"
	// Paused is Telegram asking us to wait: FLOOD_WAIT, with an end time.
	TelegramAccountPaused TelegramAccountStatus = "paused"
	// Restricted is Telegram saying the account is being treated as a spammer.
	// Nothing is retried automatically from this state.
	TelegramAccountRestricted TelegramAccountStatus = "restricted"
	// Revoked is the session being gone: the person ended it from their phone,
	// or Telegram invalidated it.
	TelegramAccountRevoked TelegramAccountStatus = "revoked"
	// Disabled is the person switching posting off themselves.
	TelegramAccountDisabled TelegramAccountStatus = "disabled"
)

// TelegramAccount is one logistician's own Telegram account, connected with
// their consent and used to post their own adverts.
//
// The session is the whole account, so it is stored sealed and is opened only
// inside the posting worker.
type TelegramAccount struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID         uuid.UUID `gorm:"type:uuid;not null;index"`
	UserID            uuid.UUID `gorm:"type:uuid;not null;index"`
	TgUserID          *int64
	Phone             string `gorm:"size:20;not null"`
	Label             string `gorm:"size:64;not null;default:''"`
	SessionCiphertext []byte `gorm:"type:bytea"`
	SessionKeyVersion *int16

	Status         TelegramAccountStatus `gorm:"size:16;not null;default:'connecting'"`
	StatusReason   *string
	PausedUntil    *time.Time
	ConsentVersion *string `gorm:"size:16"`
	ConsentAt      *time.Time
	LastUsedAt     *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// TableName pins the account table.
func (TelegramAccount) TableName() string { return "telegram_accounts" }

// SessionAAD binds a sealed session to this row. Built here, in one place, so
// the vault and the reader cannot disagree about it.
func (a *TelegramAccount) SessionAAD() string {
	return "telegram_account:" + a.ID.String() + ":" + a.CompanyID.String()
}

// PostReadiness explains whether the account may send right now.
type PostReadiness struct {
	Ready  bool
	Reason string     // empty when ready
	Until  *time.Time // when Reason is a wait
}

// ReadyToPost reports whether the account may send at now, and why not when it
// may not. A pause that has expired is treated as over: the worker clears the
// status on its next use rather than blocking on a stale timestamp.
func (a *TelegramAccount) ReadyToPost(now time.Time) PostReadiness {
	if len(a.SessionCiphertext) == 0 || a.ConsentAt == nil {
		return PostReadiness{Reason: "not_connected"}
	}
	switch a.Status {
	case TelegramAccountActive:
		return PostReadiness{Ready: true}
	case TelegramAccountPaused:
		if a.PausedUntil != nil && now.Before(*a.PausedUntil) {
			return PostReadiness{Reason: "paused", Until: a.PausedUntil}
		}
		return PostReadiness{Ready: true}
	case TelegramAccountRestricted:
		return PostReadiness{Reason: "restricted"}
	case TelegramAccountRevoked:
		return PostReadiness{Reason: "session_revoked"}
	default:
		return PostReadiness{Reason: "not_active"}
	}
}

// TelegramGroup is a group the account is already a member of and the person
// chose to post into. The platform never joins anything on its own.
type TelegramGroup struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CompanyID       uuid.UUID `gorm:"type:uuid;not null;index"`
	AccountID       uuid.UUID `gorm:"type:uuid;not null;index"`
	TgPeerID        int64     `gorm:"not null"`
	AccessHash      *int64
	Title           string  `gorm:"size:255;not null;default:''"`
	Username        *string `gorm:"size:64"`
	Kind            string  `gorm:"size:16;not null;default:'supergroup'"`
	Language        string  `gorm:"size:5;not null;default:'ru'"`
	MemberCount     *int
	SlowmodeSeconds int  `gorm:"not null;default:0"`
	CanSend         bool `gorm:"not null;default:true"`
	PostingEnabled  bool `gorm:"not null;default:true"`
	Active          bool `gorm:"not null;default:true"`
	InactiveReason  *string
	LastSyncedAt    *time.Time
	LastPostedAt    *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	Rules  *TelegramGroupRules  `gorm:"foreignKey:GroupID"`
	Routes []TelegramGroupRoute `gorm:"foreignKey:GroupID"`
}

// TableName pins the group table.
func (TelegramGroup) TableName() string { return "telegram_groups" }

// Postable reports whether the group is a legitimate destination at all.
func (g *TelegramGroup) Postable() bool {
	return g.Active && g.PostingEnabled && g.CanSend
}

// TelegramGroupRules is what the logistician says the group accepts. These are
// the only group facts the matcher reads: nothing fetched from Telegram is fed
// to a model.
type TelegramGroupRules struct {
	GroupID         uuid.UUID   `gorm:"type:uuid;primaryKey"`
	CompanyID       uuid.UUID   `gorm:"type:uuid;not null"`
	CargoTypeCodes  StringArray `gorm:"type:text[];not null;default:'{}'"`
	VehicleTypes    StringArray `gorm:"type:text[];not null;default:'{}'"`
	MinWeightKg     *int
	MaxWeightKg     *int
	HourlyLimit     int16 `gorm:"not null;default:2"`
	DailyLimit      int16 `gorm:"not null;default:3"`
	CooldownMinutes int   `gorm:"not null;default:60"`
	QuietFromHour   *int16
	QuietToHour     *int16
	Notes           *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// TableName pins the rules table.
func (TelegramGroupRules) TableName() string { return "telegram_group_rules" }

// Cooldown is how long this group must rest between posts: the longest of the
// group's own slow mode, the rule the logistician set, and the platform floor.
// Telegram's slow mode is a hard limit, so it can only ever raise the value.
func (r TelegramGroupRules) Cooldown(slowmodeSeconds int, platformFloor time.Duration) time.Duration {
	cooldown := time.Duration(r.CooldownMinutes) * time.Minute
	if slowmode := time.Duration(slowmodeSeconds) * time.Second; slowmode > cooldown {
		cooldown = slowmode
	}
	if platformFloor > cooldown {
		cooldown = platformFloor
	}
	return cooldown
}

// AcceptsVehicle reports whether the group takes this body type. An empty list
// means the logistician did not narrow it, which is not the same as "none".
func (r TelegramGroupRules) AcceptsVehicle(vehicleType *string) bool {
	return listAccepts(r.VehicleTypes, vehicleType)
}

// AcceptsCargo reports whether the group takes this cargo type.
func (r TelegramGroupRules) AcceptsCargo(cargoCode *string) bool {
	return listAccepts(r.CargoTypeCodes, cargoCode)
}

// AcceptsWeight reports whether a stated tonnage is within the group's range.
// An advert with no tonnage is not excluded: the group can still be right for
// it, and refusing would hide adverts for a fact nobody stated.
func (r TelegramGroupRules) AcceptsWeight(weightKg *int) bool {
	if weightKg == nil {
		return true
	}
	if r.MinWeightKg != nil && *weightKg < *r.MinWeightKg {
		return false
	}
	if r.MaxWeightKg != nil && *weightKg > *r.MaxWeightKg {
		return false
	}
	return true
}

// InQuietHours reports whether the local hour falls inside the window the
// logistician marked as off-limits. A window that wraps midnight is honoured.
func (r TelegramGroupRules) InQuietHours(local time.Time) bool {
	if r.QuietFromHour == nil || r.QuietToHour == nil {
		return false
	}
	from, to, hour := int(*r.QuietFromHour), int(*r.QuietToHour), local.Hour()
	if from == to {
		return false
	}
	if from < to {
		return hour >= from && hour < to
	}
	return hour >= from || hour < to
}

func listAccepts(list StringArray, value *string) bool {
	if len(list) == 0 {
		return true
	}
	if value == nil {
		// The group was narrowed and the advert says nothing: let the score
		// decide rather than dropping a group that may well be right.
		return true
	}
	for _, allowed := range list {
		if strings.EqualFold(strings.TrimSpace(allowed), strings.TrimSpace(*value)) {
			return true
		}
	}
	return false
}

// TelegramGroupRoute is one corridor a group serves. A nil region means the
// whole country.
type TelegramGroupRoute struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	GroupID        uuid.UUID `gorm:"type:uuid;not null;index"`
	CompanyID      uuid.UUID `gorm:"type:uuid;not null"`
	OriginCountry  *string   `gorm:"type:char(2)"`
	OriginRegionID *uuid.UUID
	DestCountry    *string `gorm:"type:char(2)"`
	DestRegionID   *uuid.UUID
	CreatedAt      time.Time
}

// TableName pins the route table.
func (TelegramGroupRoute) TableName() string { return "telegram_group_routes" }

// Matches reports whether the advert travels this corridor.
func (r TelegramGroupRoute) Matches(ad *LoadAd) bool {
	return endMatches(r.OriginCountry, r.OriginRegionID, ad.OriginCountry, ad.OriginRegionID) &&
		endMatches(r.DestCountry, r.DestRegionID, ad.DestCountry, ad.DestRegionID)
}

func endMatches(routeCountry *string, routeRegion *uuid.UUID, adCountry *string, adRegion *uuid.UUID) bool {
	if routeRegion != nil {
		return adRegion != nil && *routeRegion == *adRegion
	}
	if routeCountry != nil {
		return adCountry != nil && strings.EqualFold(*routeCountry, *adCountry)
	}
	return false
}

// ─── the Telegram client, as the rest of the code sees it ───────────

// SendRequest is one message to one group, from one account.
type SendRequest struct {
	Session    []byte
	PeerID     int64
	AccessHash *int64
	// RandomID is generated once per job and reused on every retry, which is
	// what lets Telegram refuse a duplicate after a crash mid-send.
	RandomID int64
	Text     string
}

// SendResult is what came back when the message went out.
type SendResult struct {
	MessageID int64
	Raw       JSONMap
}

// TelegramErrorKind is the platform's reading of a Telegram failure: what to do
// about it, not what it was called.
type TelegramErrorKind string

// How a failure is handled. The mapping from Telegram's own codes lives in the
// client adapter; everything above it reasons in these terms.
const (
	// TelegramFloodWait is Telegram asking for a wait on the whole account.
	TelegramFloodWait TelegramErrorKind = "flood_wait"
	// TelegramSlowmode is a per-group wait.
	TelegramSlowmode TelegramErrorKind = "slowmode"
	// TelegramPeerFlood is the account being treated as a spammer. Nothing is
	// retried; the person and an operator are told.
	TelegramPeerFlood TelegramErrorKind = "peer_flood"
	// TelegramWriteForbidden is the group refusing this account's messages.
	TelegramWriteForbidden TelegramErrorKind = "write_forbidden"
	// TelegramGroupGone is the group no longer being reachable for this
	// account: left, removed, or deleted.
	TelegramGroupGone TelegramErrorKind = "group_gone"
	// TelegramSessionRevoked is the session no longer being valid.
	TelegramSessionRevoked TelegramErrorKind = "session_revoked"
	// TelegramDuplicate is Telegram refusing a random_id it has already seen,
	// which means the message went out on an earlier attempt.
	TelegramDuplicate TelegramErrorKind = "duplicate"
	// TelegramMessageTooLong is a message over Telegram's limit.
	TelegramMessageTooLong TelegramErrorKind = "message_too_long"
	// TelegramTransient is a network or server failure worth retrying.
	TelegramTransient TelegramErrorKind = "transient"
	// TelegramUnknown is anything we have not classified. Treated as terminal,
	// because sending again on a failure we do not understand is how duplicates
	// and bans happen.
	TelegramUnknown TelegramErrorKind = "unknown"
)

// TelegramError is a classified failure from the client.
type TelegramError struct {
	Kind TelegramErrorKind
	// Code is Telegram's own error string, kept for the history row.
	Code string
	// Wait is how long Telegram asked us to wait, when it said.
	Wait time.Duration
	Err  error
}

func (e *TelegramError) Error() string {
	if e.Code != "" {
		return "telegram: " + string(e.Kind) + " (" + e.Code + ")"
	}
	return "telegram: " + string(e.Kind)
}

// Unwrap exposes the underlying transport error.
func (e *TelegramError) Unwrap() error { return e.Err }

// Retryable reports whether trying again could succeed without someone
// changing something first.
func (e *TelegramError) Retryable() bool {
	switch e.Kind {
	case TelegramFloodWait, TelegramSlowmode, TelegramTransient:
		return true
	default:
		return false
	}
}

// TelegramSender sends one message. The implementation holds the MTProto
// connection; everything above it stays testable with a fake.
type TelegramSender interface {
	Send(ctx context.Context, req SendRequest) (SendResult, error)
}

// SessionVault seals and opens the stored session.
type SessionVault interface {
	Seal(plaintext []byte, aad string) (ciphertext []byte, keyVersion int, err error)
	Open(ciphertext []byte, keyVersion int, aad string) ([]byte, error)
}

// ─── ports ──────────────────────────────────────────────────────────

// TelegramAccountRepository persists connected accounts.
type TelegramAccountRepository interface {
	Create(ctx context.Context, account *TelegramAccount) error
	FindByID(ctx context.Context, companyID, id uuid.UUID) (*TelegramAccount, error)
	// FindForUser returns the account a person may actually post with: their
	// own. Nobody posts from someone else's account.
	FindForUser(ctx context.Context, companyID, userID, id uuid.UUID) (*TelegramAccount, error)
	ListByCompany(ctx context.Context, companyID uuid.UUID) ([]TelegramAccount, error)
	ListByUser(ctx context.Context, companyID, userID uuid.UUID) ([]TelegramAccount, error)
	StoreSession(ctx context.Context, id uuid.UUID, ciphertext []byte, keyVersion int16, tgUserID int64) error
	SetStatus(ctx context.Context, id uuid.UUID, status TelegramAccountStatus, reason *string, pausedUntil *time.Time) error
	TouchUsed(ctx context.Context, id uuid.UUID, at time.Time) error
	// Disconnect wipes the session and marks the row, in one statement, so no
	// window exists where a disconnected account still has a usable session.
	Disconnect(ctx context.Context, companyID, id uuid.UUID) error
}

// GroupCandidateQuery is the deterministic filter that runs before any model is
// asked anything.
type GroupCandidateQuery struct {
	AccountID      uuid.UUID
	OriginCountry  *string
	OriginRegionID *uuid.UUID
	DestCountry    *string
	DestRegionID   *uuid.UUID
	VehicleType    *string
	CargoTypeCode  *string
	WeightKg       *int
}

// GroupCandidate is a group that survived the SQL filter, with everything the
// scorer needs to rank it.
type GroupCandidate struct {
	Group  TelegramGroup
	Rules  TelegramGroupRules
	Routes []TelegramGroupRoute
}

// TelegramGroupRepository persists groups and their rules.
type TelegramGroupRepository interface {
	Upsert(ctx context.Context, group *TelegramGroup) error
	FindByID(ctx context.Context, companyID, id uuid.UUID) (*TelegramGroup, error)
	ListByAccount(ctx context.Context, companyID, accountID uuid.UUID) ([]TelegramGroup, error)
	ListByCompany(ctx context.Context, companyID uuid.UUID) ([]TelegramGroup, error)
	// ListWithRules is the management view: every group with its rules and
	// corridors, read in three queries rather than two per group.
	ListWithRules(ctx context.Context, companyID uuid.UUID) ([]GroupCandidate, error)
	Candidates(ctx context.Context, companyID uuid.UUID, query GroupCandidateQuery) ([]GroupCandidate, error)
	SaveRules(ctx context.Context, rules *TelegramGroupRules, routes []TelegramGroupRoute) error
	Rules(ctx context.Context, companyID, groupID uuid.UUID) (*TelegramGroupRules, []TelegramGroupRoute, error)
	// UpdateSettings changes what the person controls on the group itself, as
	// opposed to its rules: the language posts are written in, and whether it
	// receives posts at all.
	UpdateSettings(ctx context.Context, companyID, id uuid.UUID, language *string, postingEnabled *bool) error
	Deactivate(ctx context.Context, companyID, id uuid.UUID, reason string) error
	DisablePosting(ctx context.Context, companyID, id uuid.UUID, reason string) error
	MarkPosted(ctx context.Context, id uuid.UUID, at time.Time) error
	Remove(ctx context.Context, companyID, id uuid.UUID) error
	CountActive(ctx context.Context, companyID uuid.UUID) (int64, error)
}

// ─── connecting an account ──────────────────────────────────────────

// TelegramLogin is a sign-in in progress.
//
// The session travels with it because Telegram ties the code it sent to the
// connection that asked for it: the step that submits the code has to resume
// that same authorisation, not open a new one. The caller seals it between
// requests exactly as it seals a finished session.
type TelegramLogin struct {
	Phone    string
	CodeHash string
	Session  []byte
}

// TelegramLoginResult is where a sign-in step left things.
type TelegramLoginResult struct {
	// Session is the authorised session once signing in is complete, or the
	// in-progress one when a password is still needed.
	Session  []byte
	TgUserID int64
	// PasswordRequired means the account has two-step verification on and
	// the person has to enter that password next.
	PasswordRequired bool
}

// TelegramDialog is one group the account is a member of, as the person picks
// from the list when connecting groups.
type TelegramDialog struct {
	PeerID          int64
	AccessHash      *int64
	Title           string
	Username        *string
	Kind            string
	MemberCount     *int
	SlowmodeEnabled bool
	// CanSend is whether this account may write there at all. A group where it
	// may not is shown but cannot be connected.
	CanSend bool
}

// TelegramAuthenticator connects an account and reads what it can see.
//
// Codes and passwords arrive as secret.Value-backed strings and are used for the
// one call that needs them; nothing here stores them.
type TelegramAuthenticator interface {
	SendCode(ctx context.Context, phone string) (TelegramLogin, error)
	SignIn(ctx context.Context, login TelegramLogin, code string) (TelegramLoginResult, error)
	CheckPassword(ctx context.Context, login TelegramLogin, password string) (TelegramLoginResult, error)
	Dialogs(ctx context.Context, session []byte) ([]TelegramDialog, error)
	SlowmodeSeconds(ctx context.Context, session []byte, peerID int64, accessHash *int64) (int, error)
	// LogOut ends the session on Telegram's side, so disconnecting here also
	// removes the entry from the person's list of active devices.
	LogOut(ctx context.Context, session []byte) error
}

// Sign-in failures a person can act on, each with its own message rather than
// a generic "login failed".
var (
	ErrTelegramPhoneInvalid    = errors.New("telegram: the phone number is not valid")
	ErrTelegramPhoneBanned     = errors.New("telegram: this phone number is banned by Telegram")
	ErrTelegramPhoneUnknown    = errors.New("telegram: this phone number has no Telegram account")
	ErrTelegramCodeInvalid     = errors.New("telegram: the code is wrong")
	ErrTelegramCodeExpired     = errors.New("telegram: the code has expired")
	ErrTelegramPasswordInvalid = errors.New("telegram: the password is wrong")
)
