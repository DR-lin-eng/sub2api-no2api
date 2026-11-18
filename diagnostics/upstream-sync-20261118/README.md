# 2026-11-18 上游增量同步证据

范围、能力边界与滚动升级限制见[审查说明](../../docs/UPSTREAM_SYNC_20261118.md)。

- `UPSTREAM_REVIEW_INPUT.json` 固定 8 个 PR 和版本提交；`UPSTREAM_PRS.json` 关闭每项净差异并记录四份继承台账摘要。
- `verify_ledger.py` 检查首父链、净补丁摘要、当前 owner、历史台账原始字节与能力标记；`--require-ancestry` 另核对 tracking merge。
- `run_record.py` 与 `records/` 保存命令、时间、退出码及 gzip 日志；日志 SHA-256 对应解压字节，修正前失败保留。
- `build_baseline.py` 从冻结主线 archive 构建生产基线，不修改其他 checkout。
- `usage_benchmark_test.go` 与 `run_performance.py` 对照两个实际 parser；`PERFORMANCE.json` 保存样本，生产吞吐不由此推断。
- `run_integration.py` 使用独立 Docker PostgreSQL/Redis；`runtime/` 验证同库四阶段、缓存用量、Chat JSON/SSE 和原价格回归，不访问生产数据。

门禁、镜像摘要与精确 SHA 发布状态见 [VERIFICATION.md](VERIFICATION.md)。
