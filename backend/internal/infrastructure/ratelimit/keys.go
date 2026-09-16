// Package ratelimit is the anti-spam guard: the thing that decides, in the
// moment before a message goes out, whether it may.
//
// Every counter lives in Redis and every check and reservation happens inside
// one script, because the guard is only worth having if two workers cannot both
// pass it. Postgres remains the record of what was sent; these keys are the
// short-lived arithmetic that stops it being sent too often.
package ratelimit

import (
	"strconv"
	"time"

	"github.com/google/uuid"
)

// Key prefixes. Every key the guard uses is built here, so the keyspace stays
// readable in redis-cli and prefixes never drift.
const (
	prefixDuplicate = "dup:tg:"
	prefixCooldown  = "cd:tg:"
	prefixGroup     = "rl:tg:grp:"
	prefixAccount   = "rl:tg:acct:"
	prefixGap       = "gap:tg:acct:"
	prefixPause     = "pause:tg:acct:"
	prefixGlobal    = "rl:tg:global:"
)

// duplicateKey is scoped to the Telegram group itself rather than to the
// company, because the people in that group see one message whoever sent it.
func duplicateKey(peerID int64, fingerprint string) string {
	return prefixDuplicate + strconv.FormatInt(peerID, 10) + ":" + fingerprint
}

// cooldownKey is per company: two forwarders who both connected the same group
// each get their own rest interval, and the group-wide ceiling is what keeps
// the sum reasonable.
func cooldownKey(companyID, groupID uuid.UUID) string {
	return prefixCooldown + companyID.String() + ":" + groupID.String()
}

func groupWindowKey(companyID, groupID uuid.UUID, window string) string {
	return prefixGroup + companyID.String() + ":" + groupID.String() + ":" + window
}

func accountWindowKey(accountID uuid.UUID, window string) string {
	return prefixAccount + accountID.String() + ":" + window
}

// gapKey holds the minimum spacing between two posts from one account. It is a
// key with a short TTL rather than a timestamp comparison so that the check and
// the reservation are the same operation.
func gapKey(accountID uuid.UUID) string {
	return prefixGap + accountID.String()
}

// pauseKey is set when Telegram asks for a wait. While it exists, nothing goes
// out from that account at all.
func pauseKey(accountID uuid.UUID) string {
	return prefixPause + accountID.String()
}

// globalKey throttles every account together, which is what protects the shared
// api_id: a limit breached platform-wide is a limit breached for every company.
func globalKey(at time.Time) string {
	return prefixGlobal + strconv.FormatInt(at.Unix(), 10)
}

// Window identifiers are part of the key rather than something a TTL has to get
// exactly right: an hour boundary is then a different key, not an expiry race.
func hourWindow(at time.Time) string {
	return "h:" + at.UTC().Format("2006010215")
}

func dayWindow(at time.Time) string {
	return "d:" + at.UTC().Format("20060102")
}
