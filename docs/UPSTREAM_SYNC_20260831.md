# 上游同步审查记录（2026-08-31）

## 审查边界

- 本项目基线：`origin/main` `e9c683244af76f4b1a1d0057eb367c4a2b2e0f06`。
- 上次已记录的上游冻结点：`efb46db0a960fdad94502b1c3a982a0051cf5245`。
- 本轮上游主线冻结点：`52374af94031f04df8de6fc91deb77a179e04b06`（上游 `0.1.184`）。
- `efb46db0..52374af9` 共 150 个提交、50 个 first-parent merge 和 8 个 first-parent 直接提交。
- 当前项目已模块化为 `transport -> application -> domain`，并保留独立 feature owner、调度快照、计费和安全边界；本轮继续做语义移植，不恢复上游 legacy `internal/handler` / `internal/service` / `frontend/src/views` 树。

## 选择性接入

| 上游变化 | 当前 owner / 处理 | 性能与升级结论 |
| --- | --- | --- |
| #6414 Anthropic -> Responses 流生命周期 | `shared/apicompat` 在 thinking/tool 新 item 前关闭旧 message，并在每个 text part 完成后推进 `content_index` | 只增加常数分支；修复完成事件丢文本和 SDK part 覆盖，无协议或配置迁移 |
| #6395 账号到期时间本地时区 | `core/utils/format.ts` 严格解析 `datetime-local`；账号和兑换码表单显示浏览器时区 | 低频表单路径；拒绝带时区/溢出日期，旧的合法本地时间保持一致 |
| #6367 移动端用量 tooltip | `features/admin-usage` 隐藏时使用 `display:none` | 不进入布局计算，避免窄屏横向溢出；无请求或状态变化 |
| #5469 超大 WS passthrough 首帧 | `application/service/openai_ws_http_bridge.go` 在已启用 bridge 且首帧超过阈值、无 continuation 时切到 HTTP bridge | 只对大帧做有界扫描；避免 WebSocket 大帧失败，已有 `previous_response_id` 仍保持 passthrough |
| #6385 分组限额部分更新 | admin DTO 保留 omitted/null/zero 三态，application 仅更新显式字段 | 避免滚动升级或窄 PATCH 清空日/周/月限额；无数据库变更 |
| #4872 手工配额重置解除限流 | repository 用单条 SQL 同时清零 quota 和账号级 rate-limit cooldown，并刷新调度快照 | 管理端低频写路径；原子操作，不清除模型级/过载/临时摘除等其他状态 |
| #6309 EasyPay 相对地址 | `modules/payment/provider` 只把根相对 pay URL/QR URL 按 provider origin 补全 | 绝对 URL、deep link 和 opaque QR token 原样保留；无额外网络请求 |
| #6312 WS 客户端关闭归因 | transport handler 识别裸/包装的 1000 close 与 `context.Canceled` | 客户正常离开不再降低账号健康；异常 close、deadline 和上游错误仍计失败 |
| #6293 WSv2 passthrough cyber | passthrough relay 在 `error/response.failed` 写客户端前复用共享 cyber detector 并保留上游 usage | 常数次字段读取；不改变普通错误，命中后由现有 per-turn 审计/计费链消费 |
| #6299 raw Chat 流截断 | raw CC 路径跟踪 `[DONE]`、usage、`finish_reason`；输出前截断可 failover，输出后返回类型化错误 | O(1) 状态、无流缓存；客户端断开仍继续按已有语义收集用量 |
| #6334 支付币种文案 | billing feature 使用选中支付方式的币种，不再固定显示 CNY | 纯展示修复 |
| #6343 Anthropic 工具参数 | `appendRawJSON` 把 `tool_use.input={}` 视为占位，再接收 delta JSON | 仅 tool-use 分支解析一个小对象，避免生成 `{}{...}` 无效参数 |
| #6346 Claude attribution | Use Key 生成配置不再注入 `CLAUDE_CODE_ATTRIBUTION_HEADER=0` | 纯配置生成修复，保留 nonessential traffic 开关 |
| #6270 Images 文字兜底冷却 | 文字回复只触发本次 failover；只有结构化 `image_generation_unavailable` 才写账号模型冷却 | 避免一次请求沿号池冷却全部图片账号，减少调度容量损失 |
| direct merge `f1d845c63` Grok side-call cache | `prompt_cache_key` 优先于 per-call `X-Grok-Conv-Id`，其他显式 IDE/Claude session 信号优先级不变 | 不新增解析；recap/title side-call 复用父会话缓存，避免大上下文全价重放 |

## 已有实现覆盖，不重复移植

| 上游变化 | 当前结论 |
| --- | --- |
| #6411 TTFT mode | 本项目已有 `openai_visible_output_ttft_enabled`，默认按可见输出计时并覆盖 HTTP、WS、bridge；上游改回默认 semantic 会改变既有口径，不重复 |
| #6165 上游倍率列表 | 当前已有模块化 `/admin/accounts/upstream-billing-rates`、分页/筛选、探测快照和独立 datasource |
| #6198 OpenAI refresh-token reauth | 当前 OAuthAuthorizationFlow 和 AdminReAuthAccountDialog 已覆盖 refresh/mobile refresh token，不恢复旧 modal |
| #6280 版本后缀 | 当前 `parseVersion` 已同时剥离 `-` 与 `+` 后缀 |
| #6306 Responses keepalive | 当前 first-output stage、compact keepalive 计数和 failover 写出边界已覆盖 |
| #6316 Claude session sticky | 当前标准 `session-id`、兼容 header 和 Claude metadata 已统一进入会话解析 |
| #6386 passthrough WS session isolation | 当前 passthrough relay 每个入站连接持有独立 client/upstream conn 与 turn lifecycle，不进入共享 session preemption owner |
| #6328 bulk fingerprint off | 当前 `accountBulkUpdatePayload.ts` 已显式提交 `codex_fingerprint_mode=off` 并有回归测试 |
| #5860 Antigravity mixed tools | 当前 `shared/antigravity` transformer 保留混合 built-in chat tools 并有协议测试 |
| #6227 OAuth promo code | 当前 pending OAuth flow 用 cookie/state 保存 promo code，完成注册时应用 |
| #6310 非流式终止失败 | 当前 `handleOpenAIResponseFailedBeforeOutput` 已统一 native/passthrough 的 pre-output failover 与 committed-response 边界 |
| #6263 reset 后刷新 | 当前 reset handler 分别持久化 reset-credit 和完整 rate-limit snapshot，并刷新账号 DTO |
| #6260 Anthropic/Bedrock transport | 当前共享 streaming transport 分类在输出前 failover、输出后类型化错误，并保留断线后的用量回收 |
| #6278 Grok 4.6 xhigh | 当前 Grok reasoning normalization 和模型广告已支持 4.6/xhigh |
| #5623 Responses `created_at` | 当前 `ResponsesResponse.CreatedAt` 为非 omitempty，流式/非流式 wire 测试已覆盖 |
| #5620 Anthropic reasoning passback | 当前直接 Anthropic <-> Chat bridge 已映射 `reasoning_content` 到 thinking blocks |
| #6246 multimodal tool output | 当前 `shared/apicompat` 已将工具媒体提升到带 attribution 的 user `input_image` 消息 |
| #6200 reasoning replay | 当前 encrypted reasoning 的跨账号恢复、continuation 限制和一次重试已经集中在 gateway request owner |
| #6277 namespace roundtrip | 当前 `openai_responses_namespace.go` 按账号/transport 决定 flatten，并在响应侧恢复 |
| #5395 Images capability cooldown | 当前已有 `openai:image_generation` 模型级 cooldown；本轮只接入 #6270，收紧触发证据 |
| #6358 Spark 429 | 当前 Spark shadow 与普通账号分离，带 reset 的 429 使用 model scope，影子不会触发母账号全局 429 block |
| #6255 pool 两跳 system prompt | 当前 CC pipeline 在单一 modular owner 完成 body normalization，不存在上游旧两跳重复注入路径 |
| direct `ea4291a92` / `32ac921f2` Fable | 当前有 Fable family 7d_oi 模型级限流、usage 补全及独立 prompt/thinking 映射 |
| direct `ed12ea716` Codex API-key config | 当前模板已使用 `env_key` 且 `requires_openai_auth=false`，兼容 Codex 新版配置 |
| direct `c31fe2ed9` SMTP TLS | 当前 SMTP owner 覆盖 implicit TLS、mandatory/opportunistic STARTTLS，测试入口复用保存配置 |

## 暂缓项

| 上游变化 | 暂缓原因与重新打开条件 |
| --- | --- |
| #6378 service-tier billing | 会改变真实扣费；需要单独移植 response-tier observer，并分别验证 public API 可降价、private Codex OAuth 不信任 `default`。本轮不把货币语义夹带进兼容同步 |
| direct `b5827cfd5` DeepSeek 峰谷价格、`eb4237a2b` 后缀定价 | 会覆盖默认价卡和峰谷时段，属于独立价格发布；需核对官方生效日期、历史账单和自定义渠道价优先级后单独发布 |
| #6283 compaction kind、#6188 requested reasoning effort | 都新增 usage schema 和全套查询/DTO/filter；上游 migration `231` 已与本项目历史冲突。若接入需从本项目下一迁移号开始并做大表写放大/索引评估 |
| #6303 限制用户公开分组 | 改变 `CanBindGroup` 的权限模型并新增用户字段；当前 `allowed_groups` 只表示专属分组。需独立设计兼容默认值和 auth-cache snapshot 版本 |
| #6388 Ollama usage 挂 CN 平台、#6330 Zhipu team usage | 当前 Ollama owner 限定 OpenAI/Anthropic API-key + ollama.com；跨平台凭据/页面归属需独立协议设计 |
| #6325 Grok vision tool-output、#6291 Grok request sanitizer | 当前 Grok 使用可逆 client-tool adapter；直接复制上游全量 model-input decode/marshal 会在大 base64 请求上扩大分配。待设计有大小上限的媒体提升 adapter 后重开 |
| #5866 channel monitor user groups、#6332 quota singleflight | 当前模块化 monitor 没有上游 legacy quota fetcher owner；不可达代码不移植。若新增 monitor quota mode，必须把 cache recheck 放进 singleflight 执行体 |
| #6254 configurable image cooldown | 当前固定 30 分钟且 #6270 已去除误触发；新增设置需和 admin UI/runtime cache 同批交付，不只接后端隐藏配置 |
| #6384 usage window style、direct `706b5676a` group errors | 主要是旧大页面展示/文案变化；当前 feature widgets 已拆分，未发现相同行为缺陷，后续按具体 UI issue 在 owner 内处理 |
| #5348 subscription reset anchor | 当前订阅窗口有独立 DST/日卡语义；需要单独对账周/月窗口显示与结算事实后再改 |

## 性能与平滑升级边界

- 本轮没有新增 migration、Ent/Wire 生成代码、配置键、后台任务或版本回写；`VERSION` 保持本项目 `0.1.191`。
- 热路径新增状态均为 O(1)。大 WS 首帧检查只在已启用 HTTP bridge 且超过阈值时执行，不在普通请求上解析整棵 JSON。
- raw CC 截断判定不缓存流，只跟踪四个布尔状态；正常 `[DONE]`、usage-only 和 `finish_reason` 兼容上游均保持成功。
- 分组更新对旧客户端更安全：省略限额保持原值，`null` 清除为无限制，数值 `0` 仍表示零额度。
- 手工 quota reset 是唯一会清除账号级 cooldown 的新入口；自动窗口推进、模型级冷却、overload 和 temp-unschedulable 状态不变。
- Grok cache 调整不改变无 `prompt_cache_key` 的请求，且不降低 Claude/OpenCode/CodeBuddy 显式 session 信号的优先级。

## 验证与差异关闭

### 基线复现

修改前的聚焦用例实际复现了以下失败：

- Anthropic text -> thinking 完成事件只剩 reasoning；两个 text part 的 `content_index` 均为 `0`。
- EasyPay 返回的移动支付 URL 保持为裸 `/h5/pay/ORDER_ID`。
- 分组 `daily_limit_usd:null` 被解释为 `0`，省略限额会把持久值清成 nil。
- 裸 WS 1000 close、包装后的 1000 close 和 `context.Canceled` 都被判为账号故障。
- raw Chat 空 `HTTP 200 + SSE` 返回成功的零用量结果，没有 failover。

### Docker 与静态验证

本轮执行并通过：

```sh
docker run --rm -v /private/tmp/sub2api-sync-20260831/backend:/workspace -w /workspace golang:1.26.6-alpine sh -c 'apk add --no-cache git >/dev/null && GOCACHE=/tmp/go-build GOPROXY=https://goproxy.cn,direct go test -tags=unit ./internal/application/service ./internal/infrastructure/repository ./internal/transport/http/handler ./internal/transport/http/handler/admin ./internal/modules/payment/provider ./internal/shared/apicompat'
docker run --rm -v /Users/lin/go/pkg/mod:/go/pkg/mod:ro -v /private/tmp/sub2api-sync-docker-gotmp:/gotmp -v /private/tmp/sub2api-sync-docker-gocache:/gocache -v /private/tmp/sub2api-sync-20260831/backend:/workspace -w /workspace sub2api-pr25-followup-go-test:1.26.6 sh -c 'GOTMPDIR=/gotmp GOCACHE=/gocache go test -vet=off -p=1 -tags=unit ./...'
docker run --rm -v sub2api-sync-frontend-node_modules:/workspace/frontend/node_modules -v /private/tmp/sub2api-sync-20260831:/workspace -w /workspace/frontend node:24-alpine sh -c 'corepack enable && corepack prepare pnpm@11.17.0 --activate >/dev/null && pnpm exec vitest run'
docker run --rm -e GOMAXPROCS=2 -v /private/tmp/sub2api-sync-20260831/backend:/app -w /app golangci/golangci-lint:v2.9.0-alpine golangci-lint run --concurrency 1 --timeout 10m ./internal/application/service/...
docker run --rm -e GOMAXPROCS=2 -v /private/tmp/sub2api-sync-20260831/backend:/app -w /app golangci/golangci-lint:v2.9.0-alpine golangci-lint run --concurrency 1 --timeout 10m ./internal/infrastructure/repository/... ./internal/transport/http/handler/... ./internal/modules/payment/provider/... ./internal/shared/apicompat/...
cd backend && GOCACHE=/private/tmp/sub2api-gocache-sync0831 go vet ./internal/application/service ./internal/infrastructure/repository ./internal/transport/http/handler ./internal/transport/http/handler/admin ./internal/modules/payment/provider ./internal/shared/apicompat
cd frontend && pnpm run typecheck && pnpm run lint:check && pnpm run build
make check-docs
git diff --check
```

- 同范围 Docker 基线/修改耗时基本持平：application service `176.893s -> 176.318s`，repository `5.642s -> 6.106s`，handler `31.867s -> 32.237s`。
- 全量 Docker Go unit 所有包通过；代表包 application service `174.920s`、handler `31.150s`。
- 全量 Docker Vitest：348 个文件、2,134 项测试通过。
- 两批 golangci-lint 均为 `0 issues.`；go vet、frontend typecheck/lint/build、docs 和 diff check 均为 exit 0。

### 性能结果

- raw Chat 终止状态检查优化后 5 次为 `210.4-216.3 ns/op, 0 B/op, 0 allocs/op`；初版 `choices.Array()` 的 80 B/1 alloc 已移除。
- WS turn mapping 5 次为 `18.15-18.59 ns/op, 0 B/op, 0 allocs/op`。
- 既有 Grok tool-cache snapshot benchmark 为 `341.7-344.9 ns/op, 136 B/op, 3 allocs/op`；对照的 8 MiB full-body copy 为 `293.6-336.9 us/op, 8,396,939-8,396,945 B/op`。本轮 cache-priority 修复复用 snapshot 路径，没有引入 body copy。
- 基线镜像为 42,400,058 bytes；最终候选镜像为 42,405,982 bytes，只增加 5,924 bytes。

### 真实运行验证

- 最终镜像：`sub2api-upstream-sync:20260831`，local manifest `sha256:0ff34bc07b56386d229c2c0bea0c97ea333b237a5296f475206ebeccb0614631`。
- PostgreSQL 18 + Redis 8 隔离栈完成首次初始化，再用同一数据库滚动重启最终镜像；Docker health 为 `healthy`。
- `/health` 返回 HTTP 200 `{"status":"ok"}`；`/ready` 返回 HTTP 200，`ready=true`、`current_version=0.1.191`、`draining=false`。
- `schema_migrations` 共 287 行；230-236 均存在，最新为 `236_add_custom_model_request_templates.sql`，本轮没有新增迁移或改写 checksum。
- quota reset SQL 的事务回滚验证得到：总/日/周 quota 均为 0，账号级 rate-limit 两列为 null；`overload_until`、`temp_unschedulable_until/reason` 和无关 JSONB 字段均保留。

### 差异关闭

语义移植提交为 `907712e6d73b184ab3d8402d353048419751b61a`。随后以 tree 不变的 tracking merge `fbbfa3bacd860fe7f0f805b9c421ea782c2e944e` 记录 `upstream/main` `52374af94031f04df8de6fc91deb77a179e04b06`：两个相邻 tree 均为 `0ca315f3595e41b4fd480d25dff5d43e9e87425d`，`git diff HEAD^1 HEAD` 为空，`HEAD..upstream/main` 为 0。任何未来审查只在上游行为或本表 owner/决策发生实质变化时重开。

远端 SHA、Actions 和 GHCR 证据在实际发布后核验。
