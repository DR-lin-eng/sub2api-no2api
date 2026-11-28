# 2026-11-28 上游同步验证

审查范围固定在 `81ce8356d..f53641e53`。提交图中的 104 个新增提交由 [UPSTREAM_PRS.json](UPSTREAM_PRS.json) 的 37 个首父补丁覆盖。复核命令为：

```sh
python3 diagnostics/upstream-sync-20261127/verify_ledger.py \
  --ledger diagnostics/upstream-sync-20261128/UPSTREAM_PRS.json --require-ancestry
```

台账的关闭状态记录补丁决策。功能覆盖以每一项的 `capability_complete` 及当前代码归属为准。详细边界见[增量审查](../../docs/UPSTREAM_SYNC_20261128.md)。

## 请求模型提取基准

使用 Docker `golang:1.26.11-alpine` 与 2 CPU，对相同 JSON 运行 `BenchmarkFromBodyCandidatesLargeRequest` 各三次。基线只替换模型候选提取源码，其余测试代码和镜像相同。

| 样本 | 主线中位耗时 | 候选中位耗时 | 主线 B/op | 候选 B/op |
| --- | --- | --- | --- | --- |
| 1 MiB | 1.19 ms | 0.89 ms | 2,113,617 | 112 |
| 8 MiB | 8.85 ms | 6.63 ms | 16,793,680 | 112 |

两边均为 5 allocs/op。该基准测量本地请求模型提取。Chrome 组件验证在 1280 和 360 像素宽度打开共享提示，提示右侧均离视口边缘 8 像素。

## Docker 升级与回退

使用主线镜像 `sha-559794d` 和本批生产 Dockerfile 构建的候选镜像。共享新建的 PostgreSQL 18 与 Redis 7。三个阶段的 `/ready` 均返回 `{"ready":true}`，`/health` 均返回 `{"status":"ok"}`，迁移记录均为 322，预置的粘性会话值均为 21。演练覆盖镜像启动、同库迁移兼容性及缓存读取，真实供应商和生产流量未参与演练。

Docker 后端全量 unit 与 MCP Go SDK 互操作通过。前端首轮运行发现用量浮层测试查询了旧挂载位置，也发现渠道向导测试遗漏上批已存在的默认字段。更新测试后，三个受影响套件共九项通过。
