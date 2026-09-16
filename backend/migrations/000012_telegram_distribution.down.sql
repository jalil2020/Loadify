DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE code IN ('load_ads.create', 'load_ads.view', 'load_ads.delete', 'load_ads.publish',
                   'telegram.connect', 'telegram.groups.manage', 'posting.view', 'dashboard.view')
);
DELETE FROM permissions
WHERE code IN ('load_ads.create', 'load_ads.view', 'load_ads.delete', 'load_ads.publish',
               'telegram.connect', 'telegram.groups.manage', 'posting.view', 'dashboard.view');

DROP TABLE IF EXISTS ai_tasks;
DROP TABLE IF EXISTS usage_counters;
DROP TABLE IF EXISTS spam_events;
DROP TABLE IF EXISTS posting_history;
DROP TABLE IF EXISTS posting_jobs;
DROP TABLE IF EXISTS publish_runs;
DROP TABLE IF EXISTS load_ads;
DROP TABLE IF EXISTS telegram_group_routes;
DROP TABLE IF EXISTS telegram_group_rules;
DROP TABLE IF EXISTS telegram_groups;
DROP TABLE IF EXISTS telegram_accounts;
