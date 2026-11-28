# 2026-11-27 上游同步验证

本批以项目 `origin/main` 的 `1dbeb9c31` 为基线，审查上游 `0f6671457..81ce8356d` 的 22 个首父合并 PR（`#8860`、`#8994`、`#8995`、`#9004`、`#9012`、`#9034`、`#9046`、`#9060`、`#9064`、`#9069`、`#9076`、`#9079`、`#9080`、`#9081`、`#9084`、`#9085`、`#9087`、`#9088`、`#9093`、`#9095`、`#9099`、`#9094`）。机器台账 [UPSTREAM_PRS.json](UPSTREAM_PRS.json) 记录每个补丁的固定 merge SHA、净 diff SHA-256、当前模块 owner、`diff_review_closed`、`capability_complete`、关闭理由与重开条件，并继承上一轮 [2026-11-24 台账](../upstream-sync-20261124/UPSTREAM_PRS.json) 的 SHA-256 摘要。

## 台账与追踪关系校验

从仓库根目录执行：

```sh
python3 diagnostics/upstream-sync-20261127/verify_ledger.py --require-ancestry
```

追踪合并提交以 `ours` 策略链接上游 `81ce8356d9a289411987bdefeddc9710340d6d52`，使后续增量同步直接从 `81ce8356d` 起算，不再重复扫描已关闭的 22 个 PR。合并本 PR 到 `main` 时请保持 Merge commit（勿使用 Squash merge），以保留上游提交祖先关系。

## Docker 验证范围

1. **后端单元、竞态、集成与基准（Docker `golang:1.26.11-alpine`）**：
   - `internal/shared/apicompat`、`internal/shared/antigravity`、`internal/platform/config`、`internal/infrastructure/repository`（含 miniredis CAS）、`internal/application/service` 全量测试、`internal/transport/http/...`（handler、dto、server）以及 Wire 生成一致性核对。
   - 详见 [性能与升级边界](performance.md)。
2. **前端静态门禁与单测（Docker `node:24-alpine` + `pnpm@11.17.0`）**：
   - `pnpm run lint:check`（含 1500 有效行上限与架构债务基线校验）、`pnpm run typecheck` 及新增/修改组件与编解码器 Vitest 用例。
3. **生产镜像构建与同库三阶段滚动升级/回退**：
   - 根目录 `Dockerfile` 构建候选镜像 `sub2api:upstream-sync-20261127`，核对 `--version` 保持项目版本 `0.1.250`。
   - 共享同一套 PostgreSQL 与 Redis 容器依次运行 `origin/main -> candidate -> origin/main`，验证 `/health` 与 `/ready`（`{"ready":true}`）平滑切换与回退。
