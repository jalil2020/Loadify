package ratelimit

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/loadify/backend/internal/config"
	"github.com/loadify/backend/internal/domain"
)

// reserveScript checks every limit and then takes every limit, or takes none.
//
// It is one script for one reason: a guard that checks and then reserves in two
// round trips lets two workers both pass. The order of the checks is the order
// of how final the answer is — a paused account and a duplicate are refusals, a
// counter is a wait — so the reason a person sees is the most useful one rather
// than whichever key happened to be looked at first.
var reserveScript = redis.NewScript(`
local function remaining(key, fallback)
  local t = redis.call('TTL', key)
  if t and t > 0 then return t end
  return fallback
end

if redis.call('EXISTS', KEYS[1]) == 1 then
  return {'paused', remaining(KEYS[1], 60)}
end
if redis.call('EXISTS', KEYS[2]) == 1 then
  return {'duplicate', remaining(KEYS[2], tonumber(ARGV[1]))}
end
if redis.call('EXISTS', KEYS[3]) == 1 then
  return {'cooldown', remaining(KEYS[3], tonumber(ARGV[2]))}
end

if tonumber(redis.call('GET', KEYS[4]) or '0') >= tonumber(ARGV[3]) then
  return {'group_limit', remaining(KEYS[4], tonumber(ARGV[9]))}
end
if tonumber(redis.call('GET', KEYS[5]) or '0') >= tonumber(ARGV[4]) then
  return {'group_limit', remaining(KEYS[5], tonumber(ARGV[10]))}
end

local gap = redis.call('PTTL', KEYS[6])
if gap and gap > 0 then
  return {'account_gap', math.ceil(gap / 1000)}
end
if tonumber(redis.call('GET', KEYS[7]) or '0') >= tonumber(ARGV[6]) then
  return {'account_limit', remaining(KEYS[7], tonumber(ARGV[9]))}
end
if tonumber(redis.call('GET', KEYS[8]) or '0') >= tonumber(ARGV[7]) then
  return {'account_limit', remaining(KEYS[8], tonumber(ARGV[10]))}
end

-- The platform-wide counter is taken last and given back on refusal, so a
-- message blocked by it does not also consume it.
local global = redis.call('INCR', KEYS[9])
if global == 1 then redis.call('EXPIRE', KEYS[9], 2) end
if global > tonumber(ARGV[8]) then
  redis.call('DECR', KEYS[9])
  return {'global', 1}
end

redis.call('SET', KEYS[2], 1, 'EX', tonumber(ARGV[1]))
if tonumber(ARGV[2]) > 0 then
  redis.call('SET', KEYS[3], 1, 'EX', tonumber(ARGV[2]))
end
if redis.call('INCR', KEYS[4]) == 1 then redis.call('EXPIRE', KEYS[4], tonumber(ARGV[9])) end
if redis.call('INCR', KEYS[5]) == 1 then redis.call('EXPIRE', KEYS[5], tonumber(ARGV[10])) end
if tonumber(ARGV[5]) > 0 then
  redis.call('SET', KEYS[6], 1, 'PX', tonumber(ARGV[5]))
end
if redis.call('INCR', KEYS[7]) == 1 then redis.call('EXPIRE', KEYS[7], tonumber(ARGV[9])) end
if redis.call('INCR', KEYS[8]) == 1 then redis.call('EXPIRE', KEYS[8], tonumber(ARGV[10])) end

return {'ok', 0}
`)

// releaseScript hands back a reservation when Telegram proved the message never
// went out. It never drives a counter below zero: a release that arrives twice
// must not buy a third post.
var releaseScript = redis.NewScript(`
redis.call('DEL', KEYS[2], KEYS[3], KEYS[6])
for _, key in ipairs({KEYS[4], KEYS[5], KEYS[7], KEYS[8]}) do
  if tonumber(redis.call('GET', key) or '0') > 0 then
    redis.call('DECR', key)
  end
end
return 1
`)

// checkScript is the same reading without the taking, for the preview a person
// sees before they press publish.
var checkScript = redis.NewScript(`
local function remaining(key, fallback)
  local t = redis.call('TTL', key)
  if t and t > 0 then return t end
  return fallback
end

if redis.call('EXISTS', KEYS[1]) == 1 then
  return {'paused', remaining(KEYS[1], 60)}
end
if redis.call('EXISTS', KEYS[2]) == 1 then
  return {'duplicate', remaining(KEYS[2], tonumber(ARGV[1]))}
end
if redis.call('EXISTS', KEYS[3]) == 1 then
  return {'cooldown', remaining(KEYS[3], tonumber(ARGV[2]))}
end
if tonumber(redis.call('GET', KEYS[4]) or '0') >= tonumber(ARGV[3]) then
  return {'group_limit', remaining(KEYS[4], tonumber(ARGV[9]))}
end
if tonumber(redis.call('GET', KEYS[5]) or '0') >= tonumber(ARGV[4]) then
  return {'group_limit', remaining(KEYS[5], tonumber(ARGV[10]))}
end
local gap = redis.call('PTTL', KEYS[6])
if gap and gap > 0 then
  return {'account_gap', math.ceil(gap / 1000)}
end
if tonumber(redis.call('GET', KEYS[7]) or '0') >= tonumber(ARGV[6]) then
  return {'account_limit', remaining(KEYS[7], tonumber(ARGV[9]))}
end
if tonumber(redis.call('GET', KEYS[8]) or '0') >= tonumber(ARGV[7]) then
  return {'account_limit', remaining(KEYS[8], tonumber(ARGV[10]))}
end
return {'ok', 0}
`)

// Guard implements the anti-spam gate over Redis.
//
// The per-account limits are also the per-person limits: an account belongs to
// exactly one logistician, which the database enforces, so counting both would
// be counting the same posts twice.
type Guard struct {
	client *redis.Client
	limits config.TelegramConfig
}

// NewGuard builds the guard.
func NewGuard(client *redis.Client, limits config.TelegramConfig) *Guard {
	return &Guard{client: client, limits: limits}
}

// Plan reads the limits without consuming them, which is what the publish
// preview needs: it explains, it does not decide.
func (g *Guard) Plan(ctx context.Context, targets []domain.GuardTarget, now time.Time) ([]domain.GuardDecision, error) {
	decisions := make([]domain.GuardDecision, 0, len(targets))
	for _, target := range targets {
		decision, err := g.run(ctx, checkScript, target, now)
		if err != nil {
			return nil, err
		}
		decisions = append(decisions, decision)
	}
	return decisions, nil
}

// Reserve takes every limit at once, or none of them.
func (g *Guard) Reserve(ctx context.Context, target domain.GuardTarget, now time.Time) (domain.GuardDecision, error) {
	return g.run(ctx, reserveScript, target, now)
}

// Release gives a reservation back.
func (g *Guard) Release(ctx context.Context, target domain.GuardTarget) error {
	keys := g.keys(target, time.Now())
	if err := releaseScript.Run(ctx, g.client, keys).Err(); err != nil {
		return fmt.Errorf("ratelimit: release: %w", err)
	}
	return nil
}

// PauseAccount stops everything from one account until the given moment, which
// is how a FLOOD_WAIT is honoured: Telegram asked for a wait, so the wait
// applies to the account and not merely to the message that hit it.
func (g *Guard) PauseAccount(ctx context.Context, accountID uuid.UUID, until time.Time) error {
	wait := time.Until(until)
	if wait <= 0 {
		return nil
	}
	if err := g.client.Set(ctx, pauseKey(accountID), until.UTC().Format(time.RFC3339), wait).Err(); err != nil {
		return fmt.Errorf("ratelimit: pause account: %w", err)
	}
	return nil
}

// AccountPausedUntil reports an outstanding pause.
func (g *Guard) AccountPausedUntil(ctx context.Context, accountID uuid.UUID) (time.Time, bool, error) {
	ttl, err := g.client.PTTL(ctx, pauseKey(accountID)).Result()
	if err != nil {
		return time.Time{}, false, fmt.Errorf("ratelimit: read pause: %w", err)
	}
	if ttl <= 0 {
		return time.Time{}, false, nil
	}
	return time.Now().Add(ttl), true, nil
}

// keys is the fixed key order both scripts expect.
func (g *Guard) keys(target domain.GuardTarget, now time.Time) []string {
	hour, day := hourWindow(now), dayWindow(now)
	return []string{
		pauseKey(target.AccountID),
		duplicateKey(target.PeerID, target.Fingerprint),
		cooldownKey(target.CompanyID, target.GroupID),
		groupWindowKey(target.CompanyID, target.GroupID, hour),
		groupWindowKey(target.CompanyID, target.GroupID, day),
		gapKey(target.AccountID),
		accountWindowKey(target.AccountID, hour),
		accountWindowKey(target.AccountID, day),
		globalKey(now),
	}
}

func (g *Guard) args(target domain.GuardTarget) []any {
	groupHourly := target.GroupHourly
	if groupHourly <= 0 {
		groupHourly = math.MaxInt32
	}
	groupDaily := target.GroupDaily
	if groupDaily <= 0 || groupDaily > g.limits.GroupDaily {
		// A company may ask for less than the platform allows, never for more.
		groupDaily = g.limits.GroupDaily
	}

	return []any{
		int(g.limits.DuplicateWindow.Seconds()),
		int(target.Cooldown.Seconds()),
		groupHourly,
		groupDaily,
		g.limits.AccountGap.Milliseconds(),
		g.limits.AccountHourly,
		g.limits.AccountDaily,
		g.limits.GlobalPerSecond,
		3600,
		86400,
	}
}

func (g *Guard) run(ctx context.Context, script *redis.Script, target domain.GuardTarget, now time.Time) (domain.GuardDecision, error) {
	raw, err := script.Run(ctx, g.client, g.keys(target, now), g.args(target)...).Slice()
	if err != nil {
		return domain.GuardDecision{}, fmt.Errorf("ratelimit: evaluate: %w", err)
	}
	if len(raw) != 2 {
		return domain.GuardDecision{}, fmt.Errorf("ratelimit: unexpected script result %v", raw)
	}

	reason, _ := raw[0].(string)
	wait := time.Duration(toInt64(raw[1])) * time.Second

	decision := domain.GuardDecision{GroupID: target.GroupID, RetryAfter: wait}
	switch reason {
	case "ok":
		decision.Allowed = true
	case "duplicate":
		decision.Reason = domain.SkipDuplicate
	case "cooldown":
		decision.Reason = domain.SkipCooldown
	case "group_limit":
		decision.Reason = domain.SkipGroupLimit
	case "account_gap", "account_limit", "paused":
		decision.Reason = domain.SkipAccountLimit
	case "global":
		// Not the company's doing and not worth explaining to them: it is a
		// second of platform-wide pressure, and the job simply waits.
		decision.Reason = domain.SkipAccountLimit
	default:
		return domain.GuardDecision{}, fmt.Errorf("ratelimit: unknown reason %q", reason)
	}
	return decision, nil
}

func toInt64(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case string:
		var parsed int64
		_, _ = fmt.Sscanf(v, "%d", &parsed)
		return parsed
	default:
		return 0
	}
}
