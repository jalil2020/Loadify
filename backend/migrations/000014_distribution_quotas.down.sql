UPDATE subscription_plans
SET limits = limits - ARRAY['tg_accounts', 'tg_groups', 'loads', 'posts', 'ai_requests']
WHERE target_role = 'company';
