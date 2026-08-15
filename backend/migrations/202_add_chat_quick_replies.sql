-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS chat_quick_replies (
    id BIGSERIAL PRIMARY KEY,
    admin_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(100) NOT NULL,
    content TEXT NOT NULL,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_chat_quick_replies_admin_id ON chat_quick_replies(admin_id);
CREATE INDEX idx_chat_quick_replies_sort_order ON chat_quick_replies(admin_id, sort_order);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS chat_quick_replies;
-- +goose StatementEnd
