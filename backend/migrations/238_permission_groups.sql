INSERT INTO settings (key, value, updated_at)
VALUES (
    'permission_groups',
    '[{"id":"support","name":"客服","permissions":["support.read","support.write","users.read_basic"],"built_in":true}]',
    NOW()
)
ON CONFLICT (key) DO NOTHING;
