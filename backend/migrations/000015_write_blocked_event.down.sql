-- Rows recorded under the new reason would fail the old constraint, so they
-- are folded into the closest one it allows rather than blocking the rollback.
UPDATE spam_events SET kind = 'write_forbidden' WHERE kind = 'write_blocked';

ALTER TABLE spam_events DROP CONSTRAINT spam_events_kind_check;

ALTER TABLE spam_events ADD CONSTRAINT spam_events_kind_check
    CHECK (kind IN ('duplicate', 'cooldown', 'group_limit', 'user_limit',
                    'account_limit', 'plan_limit', 'quiet_hours', 'stale',
                    'flood_wait', 'slowmode', 'peer_flood', 'write_forbidden'));
