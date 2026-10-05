-- Test users inherit the known isolated Compose administrator password hash.
INSERT INTO users (email,password_hash,role,status,username)
SELECT 'qa-user@sub2api.local',password_hash,'user','active','Upgrade QA'
FROM users WHERE email='admin@sub2api.local'
AND NOT EXISTS (SELECT 1 FROM users WHERE email='qa-user@sub2api.local');
INSERT INTO groups (name,platform,status)
SELECT 'sync-openai-browser-fixture','openai','active'
WHERE NOT EXISTS (SELECT 1 FROM groups WHERE name='sync-openai-browser-fixture');
INSERT INTO api_keys (user_id,key,name,quota,group_id)
SELECT u.id,'sk-sync-browser-fixture-20260930','browser-upgrade-fixture',12.34,g.id
FROM users u CROSS JOIN groups g
WHERE u.email='qa-user@sub2api.local' AND g.name='sync-openai-browser-fixture'
AND NOT EXISTS (SELECT 1 FROM api_keys WHERE key='sk-sync-browser-fixture-20260930');
-- Only for the isolated sub2api-sync-20260930 Docker database.
INSERT INTO settings (key, value) VALUES ('model_plaza_enabled', 'true')
ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value;
UPDATE groups SET video_rate_independent=true, video_rate_multiplier=0.5
WHERE name='sync-openai-browser-fixture';
INSERT INTO channels (name) VALUES ('sync-browser-channel-fixture') ON CONFLICT (name) DO NOTHING;
INSERT INTO channel_groups (channel_id, group_id)
SELECT c.id,g.id FROM channels c CROSS JOIN groups g
WHERE c.name='sync-browser-channel-fixture' AND g.name='sync-openai-browser-fixture'
ON CONFLICT (group_id) DO NOTHING;
INSERT INTO channel_model_pricing (channel_id, models, billing_mode, per_request_price, platform)
SELECT id,'["video-fixture"]'::jsonb,'video',2,'openai' FROM channels
WHERE name='sync-browser-channel-fixture'
AND NOT EXISTS (SELECT 1 FROM channel_model_pricing WHERE models='["video-fixture"]'::jsonb);
