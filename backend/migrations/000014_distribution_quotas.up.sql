-- Telegram distribution allowances on the company plans.
--
-- The keys are what the quota guard reads: tg_accounts and tg_groups are how
-- many may be connected at once; loads, posts and ai_requests are per calendar
-- month. -1 is no ceiling, and a key a plan does not have is no ceiling either,
-- which is why every company plan gets all five explicitly.
UPDATE subscription_plans
SET limits = limits || '{"tg_accounts": 1, "tg_groups": 15, "loads": 30, "posts": 200, "ai_requests": 50}'::JSONB
WHERE code = 'company_start';

UPDATE subscription_plans
SET limits = limits || '{"tg_accounts": 5, "tg_groups": 150, "loads": 1000, "posts": 10000, "ai_requests": 3000}'::JSONB
WHERE code IN ('company_business_m', 'company_business_y');

UPDATE subscription_plans
SET limits = limits || '{"tg_accounts": -1, "tg_groups": -1, "loads": -1, "posts": -1, "ai_requests": -1}'::JSONB
WHERE code = 'company_enterprise_m';
