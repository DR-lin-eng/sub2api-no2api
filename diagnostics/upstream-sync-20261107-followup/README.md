# 上游主线后续同步证据（2026-11-07）

范围、逐项决定和升级边界见 [审查说明](../../docs/UPSTREAM_SYNC_20261107_FOLLOWUP.md)。

- [UPSTREAM_PRS.json](UPSTREAM_PRS.json)：冻结的 20 个 PR、merge/head SHA、原始 merge diff SHA-256、当前 owner、关闭决定及重开条件。
- [VERIFICATION.md](VERIFICATION.md)：Docker、性能、同库升级回退和 CI 的实际结果。
- `performance/`：相同主线/候选夹具与冻结上游算法对照；结果不代表生产吞吐。
- `ui/`：真实组件的隔离 Vite 夹具，仅返回本地固定额度，无外部写入。
- `records/`：验证输出，不保存认证材料。

从仓库根目录运行 `python3 diagnostics/upstream-sync-20261107-followup/run_integration.py`，会创建并清理本轮独立的 Docker PostgreSQL/Redis 网络，使用 `sub2api-followup-gomod` 和 `sub2api-followup-gocache` 缓存。
