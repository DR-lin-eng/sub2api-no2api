# 2026-11-16 上游后续同步证据

范围及升级边界见[审查说明](../../docs/UPSTREAM_SYNC_20261116_FOLLOWUP.md)。

- `UPSTREAM_REVIEW_INPUT.json` 保存原始冻结范围；`UPSTREAM_PRS.json` 保存 7 个 PR 和 1 个直接提交的净 diff hash、owner 和关闭决定。
- `verify_ledger.py` 独立核对首父链、补丁摘要、继承的 259 项决定及真实 owner；祖先关系不作为功能覆盖证明。
- `PRICING_SOURCE.json` 保存另行读取的官方 Carbon 价卡响应摘要和已核对价格。
- `run_record.py` 与 `records/` 保存实际命令、时长、退出码及 gzip 原始日志，SHA-256 针对解压字节；首次失败不覆盖。
- `run_performance.py` 从固定上游源码和当前局部适配生成同一输入，三次采样并重复上游对照；`PERFORMANCE.json` 只反映正则兼容步骤。`run_unicode_guard.py` / `UNICODE_GUARD.json` 另行复核初稿对 Unicode 提示词的额外解析。
- `runtime/` 使用临时 PostgreSQL/Redis 和本地假上游，按同库基线→候选→回退→恢复验证。

最终门禁与发布记录见 [VERIFICATION.md](VERIFICATION.md)。
