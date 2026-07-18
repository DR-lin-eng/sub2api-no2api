ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS openai_force_image_tool BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN groups.openai_force_image_tool IS
    'OpenAI Responses 是否强制注入 image_generation 并改由同组 Images API 账号执行';
