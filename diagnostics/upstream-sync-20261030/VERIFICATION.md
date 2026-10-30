# 上游同步验证（2026-10-30）

基线：`ed0c9ee94eaf3b2e00cfb89677a1848bbba0f018`。上游冻结：
`a3298b46ebe239fb0720b068a1eb3383955bdac2..d851a38477597157e78450d30af2d8504764d085`。

本轮改动只涉及前端依赖覆盖、v11 构建配置和订阅卡测试；没有后端运行时或数据库迁移改动。
本地验证使用 Node 25.8.1/pnpm 11.25.0，Docker 使用项目生产 Dockerfile 的 Node 24、pnpm
11.17.0 和 Go 1.26.10。以下是最终命令及结果：

| 检查 | 结果 |
| --- | --- |
| `pnpm install --frozen-lockfile --ignore-scripts` | 通过；锁文件保持一致，供应链策略 484 项通过 |
| `pnpm exec vitest run src/features/subscriptions/__tests__/SubscriptionPlanCard.spec.ts` | 通过；9 tests |
| `pnpm run typecheck` | 通过 |
| `pnpm run lint:check` | 通过 |
| `pnpm exec vite build --config vite.config.ts` | 通过；保留既有大 chunk 警告 |
| `docker build --progress=plain -t sub2api:upstream-sync-20261030 .` | 通过；生产镜像摘要 `sha256:1f041b9331347b2a7c897943073020f371d59f2d61eee99a36fbfc7ec19a6fc5` |
| `docker run --rm --entrypoint /app/sub2api sub2api:upstream-sync-20261030 -version` | 通过；`Sub2API 0.1.230` |
| Docker runtime asset smoke check | 通过；二进制、`resources/model-pricing/model_prices_and_context_window.json`、入口脚本均存在 |
| `docker build --target backend-builder -t sub2api-backend-test:upstream-sync-20261030 .` | 通过；Go 1.26.10 编译 |
| Docker `go test -tags=unit ./... -count=1` | 通过；application/service 230.945s，HTTP handler 57.076s，其余包通过 |
| `docker build --target frontend-builder -t sub2api-frontend-test:upstream-sync-20261030 .` | 通过；Node 24/pnpm 11.17.0 |
| Docker frontend lint/typecheck + i18n/订阅关键用例 | 通过；3 files / 14 tests |
| Docker `./scripts/check-source-layout.sh` | 仅报告既有超限：`payment_fulfillment_test.go` 1201 行、`pricing_service.go` 1218 行；未重复重构 |

构建阶段的 Vite 大 chunk 警告属于基线已有的分包边界，不是本轮依赖覆盖造成的回归。Docker
生产镜像构建和 unit 测试均在同一份工作区源码上执行；没有将上游 #8389 的未移植代码放入运行路径。
