# 上游同步证据（2026-11-12）

范围与升级边界见 [审查说明](../../docs/UPSTREAM_SYNC_20261112.md)。

- [UPSTREAM_PRS.json](UPSTREAM_PRS.json)：33 个冻结 PR 的原始 diff 摘要、owner、关闭依据和重开条件。
- [VERIFICATION.md](VERIFICATION.md)：实际 Docker、性能、回滚和发布验证。
- `records/`：执行命令、退出码、耗时和输出摘要；首次失败保留用于区分环境/基线问题。输出以 gzip 保存原始字节，record 中的 SHA-256 针对解压内容；可用 `gzip -dc` 阅读。
- `runtime/`：一次性同库升级、回滚、恢复的假上游及 HTTP 夹具。
- `performance/`：冻结主线与候选共用的固定性能夹具。

`python3 diagnostics/upstream-sync-20261112/run_integration.py` 会创建并清理独立 Docker 网络，
使用 PostgreSQL 18.1 和 Redis 8.4。审计的真实数据库测试在独立 `prompt_audit` 库中顺序运行，
避免其清理身份表操作影响主集成库。测试凭据只用于本地临时容器。
