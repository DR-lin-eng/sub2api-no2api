# 历史暂缓差异处理证据

范围、取舍、性能和升级边界见[当前台账](../../docs/UPSTREAM_DEFERRED_STATUS.md)。

- `HISTORICAL_INPUT.json`：去重后的 198 个上游主线 PR 及原记录。
- `RESOLUTIONS.json` / `UPSTREAM_PRS.json`：当前归组决策、逐 PR 摘要与能力状态。
- `build_ledger.py`：在仓库根目录运行，复核 owner、重复项和冻结净 diff hash。
- `run_record.py NAME COMMAND...`：保存命令、退出状态、耗时与原始 gzip 输出；SHA-256 针对解压字节。
- `run_integration.py`：一次性 Docker PostgreSQL/Redis 集成；创建的容器和网络在退出时清理。
- `runtime/`：同一数据库的主线 → 候选 → 回退 → 恢复矩阵，测试凭据只用于临时容器。
- `records/`：保留初次失败、基线复现和最终执行结果，不能将单次局部通过代替全量检查。

完整验证结果在最终 `VERIFICATION.md` 汇总。原始记录中，前端 3 GiB 容器未显式增大 Node 堆时出现 V8 OOM；后续按固定堆配置重跑。测试夹具错误和真实实现错误均保留首次日志。
