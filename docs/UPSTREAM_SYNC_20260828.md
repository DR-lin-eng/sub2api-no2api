# 上游同步审查记录（2026-08-28）

## 审查边界

- 本项目发布基线：`origin/main` `5cec11852fc55c179a2c8720dd89942d02ca1e8b`。
- 上次已记录的上游冻结点：`e2d9b823f63dc4e8f4014be3fd24a0a73e339867`。
- 本轮上游主线冻结点：`efb46db0a960fdad94502b1c3a982a0051cf5245`。
- `e2d9b823..efb46db0` 共 59 个提交、19 个合并 PR。审查先比较当前模块化 owner、请求协议、调度/计费边界和热路径，再做窄范围语义移植；不把上游 legacy `internal/handler` / `internal/service` 树整体快进。

## 选择性接入

| 上游 PR | 当前 owner / 处理 | 性能与升级结论 |
| --- | --- | --- |
| #6157 Responses Lite API Key/WS | `application/service/openai_responses_lite_tools.go` 与 HTTP、WS ingress、WS bridge 调用点按账号类型分派；API Key 只固定 `parallel_tool_calls=false`，OAuth 保留 namespace 归一化 | 仅在 Lite 请求执行一次有界 JSON 扫描；旧请求不变，修复混合账号间歇性 400 |
| #6164 sticky capacity spillover | `openai_gateway_scheduling.go` 标记健康粘性账号队列满时的临时 spillover，阻止 Layer 2/3 覆盖持久绑定 | 只增加布尔分支；短时拥塞不再造成会话迁移和 prompt-cache 冷启动 |
| #6166 邮箱别名换绑 | `auth_email_binding.go`、`infrastructure/repository/user_repo.go` 增加别名快速查重和事务内 exact/inbox advisory lock；锁内复查后更新邮箱与密码 | 换绑是低频路径，候选查询上限仍为 50；跨实例并发不扩大普通请求热路径 |
| #6175 Antigravity token clamp | `antigravity_gateway_compat.go` 对正的 `max_tokens` / `max_completion_tokens` 统一上限 64000 | 协议边界一次 `min`，避免确定性上游 400；缺省和非正值保持原行为 |
| #6078 Responses tool-call ID | `shared/apicompat/responses_client_tools.go` 在 function/custom/tool-search 升降级及流式生命周期中按类型重写 `fc_`/`ctc_`/`tsc_` 前缀 | 只处理已有工具条目，后缀稳定；不增加 I/O 或重试 |
| #6211 WS v2 regression test | 以共享 `apicompat` 重类型测试覆盖恢复方向；现有 WS ingress 回归继续验证公共入口 | 仅测试面，未复制上游 legacy 测试夹具 |
| #6149 OAuth image prompt | `openai_images.go` / `openai_images_responses.go` 注入明确的 verbatim instructions，并保留原桥接模型 | 固定短字符串，无额外请求；用户提示词不再被 Responses 模型改写 |
| #5920 Antigravity Sonnet 4.5 aliases | `domain/constants.go` 将旧 thinking/versioned alias 默认迁移到 `claude-sonnet-4-6`；账号连接测试默认模型同步更新 | 只影响未自定义映射的旧 alias；显式 canonical `claude-sonnet-4-5` 仍透传，滚动升级可回退 |
| #6229 upstream endpoint observability | `openai_gateway_*` 清理/记录每次尝试的实际 `/v1/chat/completions`，handler 优先读取 service runtime endpoint | 仅修正错误/用量日志字段，不改变路由；failover 复用 Gin context 时不会残留旧端点 |
| #6132 cache TTL billing | 保留本项目已有的 delta 零值不覆盖语义，仅移植缓存 5m/1h 明细超过聚合量时的比例收敛 | 计费上限是 O(1) 数学运算，避免矛盾明细重复计费，不改变正常流式零值兼容 |
| #6152 payment result balance | `features/billing/presentation/pages/PaymentResultPage.vue` 在余额订单明确 `COMPLETED` 后单次刷新 auth profile；失败不影响订单展示 | 只触发一次 best-effort 请求，`PAID`/订阅订单不提前刷新 |
| #6201 Codex `session-id` | scheduler 显式会话信号和 WS 日志解析优先标准连字符 header，兼容旧 `session_id` | 只改 header 选择顺序；粘性哈希和重连稳定性提升，无数据库变更 |

## 已处理、重复或暂缓

- #5658 OpenCode Go `GoUsageLimitError`：本项目已有完整的 `Resets in` 多单位解析和回归测试，不重复移植。
- #6208 OAuth 429：当前主线已有按 5h/7d reset 的 model-scoped cooldown、Codex prewarm 429 豁免和无 OAuth 同账号重试的 failover 约束；直接套用上游旧 `SameAccountRetry` 会放宽既有调度边界，保留本项目实现。
- #6204 Kimi concurrency 403、#6155 Kimi K3 composite：本项目没有独立 `PlatformKimi` 调度桶，Kimi 兼容请求按 OpenAI 账号协议处理；强行增加平台会扩大快照/筛选和迁移面，暂缓。
- #6193 / #6101 Channel Monitor V2 SQL：本项目模块化树没有上游 `channel_monitor_v2_aggregation` owner，不引入不可达 SQL。
- #5926 routed Codex model catalogs：上游改动约 7,900 行，跨账号能力快照、Models.dev 外部请求、组级缓存副本、旧前端页面和多平台目录。当前已有独立 Codex manifest 缓存与 feature owner；整体移植会扩大请求热路径、缓存失效和滚动升级契约，待单独设计兼容 API 后再评估。

## 升级与性能边界

- 本轮无数据库迁移、生成 Ent/Wire 变更或后台队列变更。
- Lite、工具 ID、邮箱 alias 和计费修复均有明确输入/候选上限；普通非目标请求不新增 JSON 解析或外部 I/O。
- 旧配置文件、旧 `session_id` header、旧邮箱主地址和显式 Sonnet 4.5 选择保持兼容；新逻辑只在目标协议/账号类型命中时生效。
- 上游版本提交 `0.1.182/0.1.183` 与本项目 `0.1.190` 不直接回写，避免滚动升级时版本倒退。

## 验证

本轮执行并通过：

```sh
cd backend && GOCACHE=/private/tmp/go-cache-sync-20260828 go test -tags=unit ./internal/application/service ./internal/infrastructure/repository ./internal/transport/http/handler ./internal/transport/http/server/routes ./internal/domain ./internal/shared/apicompat
cd backend && GOCACHE=/private/tmp/go-cache-sync-20260828 go vet ./internal/application/service ./internal/infrastructure/repository ./internal/transport/http/handler ./internal/shared/apicompat
cd backend && ./scripts/check-source-layout.sh
cd frontend && pnpm run typecheck
cd frontend && pnpm run lint:check
cd frontend && pnpm exec vitest run src/features/billing/__tests__/PaymentResultPage.spec.ts src/features/billing/__tests__/paymentLocaleScopes.spec.ts
docker buildx build --load -t sub2api-upstream-sync-20260828:local .
docker run --rm -v /private/tmp/sub2api-sync-20260828/backend:/workspace -w /workspace golang:1.26.6-alpine sh -c 'apk add --no-cache git && GOCACHE=/tmp/go-build GOPROXY=https://goproxy.cn,direct go test -tags=unit ./internal/application/service ./internal/infrastructure/repository ./internal/shared/apicompat ./internal/transport/http/handler'
cd backend && TESTCONTAINERS_RYUK_DISABLED=true GOCACHE=/private/tmp/go-cache-sync-20260828 go test -tags=integration ./internal/infrastructure/repository -run 'TestUserRepoSuite/TestUpdateEmailWithAliasGuardRejectsAliasCollision' -count=1 -v
```

Docker 运行栈使用 PostgreSQL 18、Redis 8 和本轮镜像，`/ready` 返回 `{"ready":true}`，`/health` 返回 `{"status":"ok"}`。Docker unit 包含 service、repository、apicompat 和 HTTP handler，均通过；PostgreSQL/Redis Testcontainers 的邮箱 alias 事务测试也通过。

## 差异关闭

选择性移植提交 `46b04aa6e37fc55174c04ebbcc4a52d187bf1234` 完成后，以 `ours` tracking merge `be7f583e362624dc33fe0402530d7500a1a6c8c7` 记录 `upstream/main` `efb46db0`，再以 `56d09a95f0906dcc888a774173443c2d64ad3e08` 补齐 PostgreSQL alias 集成回归；后续 `HEAD..upstream/main=0`，暂缓项、已处理项和上游版本倒退均在本记录中保留决策，避免未来重复检查。

## 发布证据

- `git ls-remote origin refs/heads/main`：`56d09a95f0906dcc888a774173443c2d64ad3e08`（exit 0）。
- `CI` run `33110189211`：success；`Security Scan` run `33110189238`：success；`Docker Image` run `33110189216`：success。
- GHCR tags `main`, `latest`, `sha-56d09a9` 共用 manifest digest `sha256:a82dcdfa4b86a1fa8bd72dd8f32a90bf1071ea28b2d2b5919e0e93d1944d6a2e`。
