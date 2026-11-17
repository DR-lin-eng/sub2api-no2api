# 2026-11-17 上游增量审查证据

范围、能力边界和升级顺序见[审查说明](../../docs/UPSTREAM_SYNC_20261117.md)。

- `UPSTREAM_REVIEW_INPUT.json` 固定 2 个 PR 和版本提交；`UPSTREAM_PRS.json` 保存净补丁摘要、owner、关闭决定及三份继承台账摘要。
- `verify_ledger.py` 核对首父链、每项补丁摘要、owner 和继承台账原始字节；不以祖先关系证明功能覆盖。
- `PRICING_SOURCE.json` 保存独立获取的官方模型页响应摘要与实际价格摘录。Reranker-4B 的推算美元价明确未采用。
- `run_record.py` 与 `records/` 保存实际命令、退出码、耗时及 gzip 原始日志；日志摘要针对解压字节。失败记录保留。
- `runtime/` 是 Docker 同库基线→候选→回退→恢复夹具，不连接真实模型、支付账号或生产数据库。
- `run_integration.py` 在独立 Docker PostgreSQL/Redis 中运行原有全包 integration，并为独立审计模块使用隔离数据库。

实际门禁和发布状态见 [VERIFICATION.md](VERIFICATION.md)。
