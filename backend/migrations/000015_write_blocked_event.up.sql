-- Telegram sometimes refuses an account's messages to groups in general,
-- reported per group as USER_BANNED_IN_CHANNEL. It is neither a group's
-- refusal nor a verdict on the account: it passes. The account pauses and
-- tries again, and the reason is worth counting like every other reason a
-- post did not go out.

ALTER TABLE spam_events DROP CONSTRAINT spam_events_kind_check;

ALTER TABLE spam_events ADD CONSTRAINT spam_events_kind_check
    CHECK (kind IN ('duplicate', 'cooldown', 'group_limit', 'user_limit',
                    'account_limit', 'plan_limit', 'quiet_hours', 'stale',
                    'flood_wait', 'slowmode', 'peer_flood', 'write_forbidden',
                    'write_blocked'));
