# 2026-11-16 上游同步证据

范围、兼容性与升级回退边界见 [审查说明](../../docs/UPSTREAM_SYNC_20261116.md)。

- `UPSTREAM_REVIEW_INPUT.json` / `UPSTREAM_SHA.txt`：冻结范围和原始文件清单。
- `UPSTREAM_PRS.json` / `verify_ledger.py`：61 个 PR 的净 diff 摘要、owner、关闭决策、能力边界和独立核对。
- `GO-2026-6859.json`：官方漏洞版本范围；当前 1.26.11 已修复。
- `run_record.py` / `records/`：实际命令、退出码、时长及原始 gzip 日志，SHA-256 针对解压字节。失败记录保留。
- `run_integration.py`：临时 Docker PostgreSQL 18.1 / Redis 8.4，独立 prompt_audit 测试库；退出清理容器和网络。
- `runtime/`：同一数据库的基线、升级、回退、恢复与 HTTP 可靠结算；账号和凭据均为本地临时夹具。
- `performance/`：基线/候选/回退共用固定 benchmark；不依赖真实上游延迟。

最终有效结果及发布记录见 [VERIFICATION.md](VERIFICATION.md)。
