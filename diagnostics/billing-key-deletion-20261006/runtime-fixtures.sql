-- Entirely synthetic users/accounts in the audit Compose project.
BEGIN;
INSERT INTO users (email,password_hash,role,status,balance,concurrency)
VALUES ('balance@billing-audit.invalid','audit-password-hash','user','active',100,20),
       ('subscription@billing-audit.invalid','audit-password-hash','user','active',100,20)
ON CONFLICT (email) WHERE deleted_at IS NULL DO NOTHING;

INSERT INTO groups (name,platform,subscription_type,rate_multiplier,daily_limit_usd,weekly_limit_usd,monthly_limit_usd)
VALUES ('audit-openai-balance','openai','standard',1,NULL,NULL,NULL),
       ('audit-anthropic-balance','anthropic','standard',1,NULL,NULL,NULL),
       ('audit-openai-subscription','openai','subscription',1,100,100,100),
       ('audit-anthropic-subscription','anthropic','subscription',1,100,100,100);

INSERT INTO accounts (name,platform,type,credentials,extra,concurrency,egress_mode)
VALUES ('audit-openai','openai','apikey','{"api_key":"synthetic-upstream-key","base_url":"http://upstream:8080"}',
        '{"quota_limit":1000,"quota_used":0}',20,'direct'),
       ('audit-anthropic','anthropic','apikey','{"api_key":"synthetic-upstream-key","base_url":"http://upstream:8080"}',
        '{"quota_limit":1000,"quota_used":0}',20,'direct');

INSERT INTO account_groups (account_id,group_id)
SELECT a.id,g.id FROM accounts a JOIN groups g ON a.platform=g.platform
WHERE a.name LIKE 'audit-%' AND g.name LIKE 'audit-%';
INSERT INTO user_allowed_groups (user_id,group_id)
SELECT u.id,g.id FROM users u CROSS JOIN groups g
WHERE u.email LIKE '%@billing-audit.invalid' AND u.role='user' AND g.name LIKE 'audit-%';
INSERT INTO user_subscriptions (user_id,group_id,starts_at,expires_at,status)
SELECT u.id,g.id,NOW()-INTERVAL '1 hour',NOW()+INTERVAL '1 day','active'
FROM users u CROSS JOIN groups g
WHERE u.email='subscription@billing-audit.invalid' AND g.name LIKE 'audit-%' AND g.subscription_type='subscription';
INSERT INTO user_platform_quotas (user_id,platform,daily_limit_usd,weekly_limit_usd,monthly_limit_usd)
SELECT u.id,p.platform,100,100,100 FROM users u CROSS JOIN (VALUES ('openai'),('anthropic')) AS p(platform)
WHERE u.email='balance@billing-audit.invalid';
COMMIT;
