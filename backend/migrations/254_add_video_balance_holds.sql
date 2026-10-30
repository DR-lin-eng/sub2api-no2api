-- 视频生成的两段式余额冻结记录。
-- 创建任务时冻结预估金额，轮询到完成按上游实际秒数结算，失败或超时全额释放。
-- usage_billing_dedup 只记录请求是否已处理，无法回读冻结金额，也不适合承载
-- 待释放扫描；因此这里单独持久化每一次冻结的金额、状态与过期时间。
CREATE TABLE IF NOT EXISTS video_balance_holds (
    hold_key VARCHAR(64) PRIMARY KEY,
    request_id VARCHAR(255) NOT NULL DEFAULT '',
    upstream_request_id VARCHAR(255) NOT NULL DEFAULT '',
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    group_id BIGINT,
    account_id BIGINT,
    model VARCHAR(255) NOT NULL DEFAULT '',
    resolution VARCHAR(32) NOT NULL DEFAULT '',
    duration_seconds INTEGER NOT NULL DEFAULT 0,
    hold_amount DECIMAL(20, 8) NOT NULL DEFAULT 0,
    actual_amount DECIMAL(20, 8),
    currency VARCHAR(8) NOT NULL DEFAULT 'USD',
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 超时释放扫描：只关心仍处于 pending 且已过期的记录。
CREATE INDEX IF NOT EXISTS idx_video_balance_holds_pending
    ON video_balance_holds (status, expires_at);

-- 轮询阶段按上游任务 id 反查冻结记录。
CREATE UNIQUE INDEX IF NOT EXISTS uq_video_balance_holds_upstream_request
    ON video_balance_holds (upstream_request_id)
    WHERE upstream_request_id <> '';
