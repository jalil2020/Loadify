package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// DistributionAlertKind is something about sending that a person has to know
// about or act on. The values double as notification types, so each can be
// muted on its own.
type DistributionAlertKind string

// What the worker reports.
const (
	// Telegram treats the account as a spammer. Nothing is sent from it and
	// what was queued is cancelled. Platform admins hear about this too: it is
	// the signal that the limits are not tight enough for someone.
	AlertAccountRestricted DistributionAlertKind = "telegram.account_restricted"
	// The session ended on Telegram's side; the account has to be connected
	// again.
	AlertAccountRevoked DistributionAlertKind = "telegram.account_revoked"
	// Telegram asked the account to wait. Short waits are routine and are not
	// reported; long ones explain why posts stopped, and very long ones reach
	// platform admins as well.
	AlertAccountPaused DistributionAlertKind = "telegram.account_paused"
	// The account may no longer write in a group, and posting there is off.
	AlertGroupClosed DistributionAlertKind = "telegram.group_closed"
	// The account is no longer in a group.
	AlertGroupLost DistributionAlertKind = "telegram.group_lost"
	// Telegram refused a post for a reason the platform does not classify. The
	// post is not retried; platform admins are told, because the fix is ours.
	AlertTelegramUnknown DistributionAlertKind = "telegram.unknown_error"
)

// DistributionAlert is one such event, with what the message needs to say.
type DistributionAlert struct {
	Kind      DistributionAlertKind
	CompanyID uuid.UUID
	AccountID uuid.UUID
	// OwnerID is the person whose account it is. They hear about everything
	// except an unclassified error, which is the platform's to fix.
	OwnerID uuid.UUID
	// AccountLabel names the account without exposing its number.
	AccountLabel string
	GroupID      *uuid.UUID
	GroupTitle   string
	// Until is when a pause ends.
	Until *time.Time
	// Cancelled is how many queued posts were stopped because of this.
	Cancelled int64
	// ErrorCode is Telegram's own code, for an error nobody classified.
	ErrorCode string
}

// DistributionAlerter tells people. It never fails the caller: the worker has
// already recorded what happened, and a missing message must not undo that.
type DistributionAlerter interface {
	Alert(ctx context.Context, alert DistributionAlert)
}
