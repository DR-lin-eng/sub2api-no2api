-- 记录冻结时的视频计费模式。
-- 按次（per_request）计费的视频价格与时长无关，结算时不能按上游实际秒数折算，
-- 否则生成的实际时长与请求时长不一致时会少扣或多扣费用。
ALTER TABLE video_balance_holds
    ADD COLUMN IF NOT EXISTS billing_mode VARCHAR(16) NOT NULL DEFAULT '';
