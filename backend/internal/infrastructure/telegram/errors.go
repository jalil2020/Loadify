// Package telegram is the MTProto client: the one place that speaks to Telegram
// on behalf of a logistician's own account.
//
// Everything above it reasons in the platform's terms — "wait for the account",
// "the group is closed", "stop everything" — and this package is where
// Telegram's own error strings are turned into those terms. Keeping the
// translation in one file is what lets the worker's rules be tested without a
// network, and what makes a new error string a one-line change.
package telegram

import (
	"context"
	"errors"
	"time"

	"github.com/gotd/td/tgerr"

	"github.com/loadify/backend/internal/domain"
)

// Error types, as Telegram writes them with any numeric argument removed.
// SLOWMODE_WAIT_120 arrives with type SLOWMODE_WAIT and argument 120.
var (
	slowmodeTypes = []string{"SLOWMODE_WAIT"}

	// PEER_FLOOD is the account being limited for spam. USER_DEACTIVATED_BAN is
	// the same verdict delivered harder. Both stop everything.
	restrictedTypes = []string{"PEER_FLOOD", "USER_DEACTIVATED_BAN", "USER_RESTRICTED"}

	// The group is there, this account may not write in it.
	forbiddenTypes = []string{
		"CHAT_WRITE_FORBIDDEN", "CHAT_SEND_PLAIN_FORBIDDEN", "CHAT_RESTRICTED",
		"USER_BANNED_IN_CHANNEL", "CHAT_ADMIN_REQUIRED", "CHAT_GUEST_SEND_FORBIDDEN",
	}

	// The group is not reachable for this account any more: it left, was
	// removed, or the group is gone.
	goneTypes = []string{
		"CHANNEL_PRIVATE", "CHANNEL_INVALID", "PEER_ID_INVALID", "CHAT_ID_INVALID",
		"USER_NOT_PARTICIPANT", "CHAT_FORBIDDEN",
	}

	// The session no longer authorises anything.
	revokedTypes = []string{
		"AUTH_KEY_UNREGISTERED", "SESSION_REVOKED", "SESSION_EXPIRED",
		"USER_DEACTIVATED", "AUTH_KEY_DUPLICATED", "AUTH_KEY_INVALID",
	}
)

// Classify turns whatever the client returned into the platform's reading of
// it. It never returns nil for a non-nil error.
func Classify(err error) *domain.TelegramError {
	if err == nil {
		return nil
	}

	if wait, ok := tgerr.AsFloodWait(err); ok {
		code := "FLOOD_WAIT"
		if rpc, found := tgerr.As(err); found {
			code = rpc.Message
		}
		return &domain.TelegramError{Kind: domain.TelegramFloodWait, Code: code, Wait: wait, Err: err}
	}

	rpc, ok := tgerr.As(err)
	if !ok {
		// Not an answer from Telegram at all: a dropped connection, a timeout,
		// a DNS failure. Worth retrying, with the same random_id.
		if errors.Is(err, context.Canceled) {
			return &domain.TelegramError{Kind: domain.TelegramTransient, Code: "cancelled", Err: err}
		}
		return &domain.TelegramError{Kind: domain.TelegramTransient, Err: err}
	}

	classified := &domain.TelegramError{Code: rpc.Message, Err: err}
	switch {
	case rpc.IsOneOf(slowmodeTypes...):
		classified.Kind = domain.TelegramSlowmode
		classified.Wait = time.Duration(rpc.Argument) * time.Second
	case rpc.IsOneOf(restrictedTypes...):
		classified.Kind = domain.TelegramPeerFlood
	case rpc.IsOneOf(forbiddenTypes...):
		classified.Kind = domain.TelegramWriteForbidden
	case rpc.IsOneOf(goneTypes...):
		classified.Kind = domain.TelegramGroupGone
	case rpc.IsOneOf(revokedTypes...):
		classified.Kind = domain.TelegramSessionRevoked
	case rpc.IsType("RANDOM_ID_DUPLICATE"):
		// Checked before the generic 500 below: Telegram reports this one with
		// an internal-error code, and it means the message already went out.
		classified.Kind = domain.TelegramDuplicate
	case rpc.IsType("MESSAGE_TOO_LONG"):
		classified.Kind = domain.TelegramMessageTooLong
	case rpc.Code >= 500:
		classified.Kind = domain.TelegramTransient
	default:
		classified.Kind = domain.TelegramUnknown
	}
	return classified
}
