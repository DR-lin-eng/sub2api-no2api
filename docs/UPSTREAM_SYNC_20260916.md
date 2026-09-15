# 上游渠道适配同步审查记录（2026-09-16）

## 冻结点与范围

| 项目 | 值 |
| --- | --- |
| 本项目起点 | `df540cea99c810c978fc648adf454ec81d404293`（`0.1.204`） |
| 上次已审查上游 | `32682a4f84c6a29104439050a0be14737552cec2` |
| 本轮上游冻结点 | `881f3202694c6bc932446931a30c27d9675178b9`（上游 `0.2.5`） |
| 新增上游范围 | 11 个提交、5 个 first-parent merge、6 个 first-parent 提交 |
| 功能提交 | `b4675ae5d39036951cb77260ec3f30453bd73bff` |
| 跟踪合并 | `0fd90b4bd1fa590cf1a68be7e6c481aebcdbb839` |

本轮继续按本项目的模块 owner 做语义移植，不恢复上游旧的 `internal/service`、`internal/handler` 和 `frontend/src/views` 目录。上游 `VERSION=0.2.5` 不导入；本项目版本保持 `0.1.204`。

## 新增上游范围

| 上游 PR / 提交 | 处理 | 当前 owner 与结果 |
| --- | --- | --- |
| #7153 / `55a95d4c6` | 已移植 | API Key 创建器增加 provider 初筛；本项目既有有序多分组绑定仍是最终路由事实源。 |
| #6977 / `a939553c8` | 已移植 | DeepSeek 无显式映射时按官方模型目录校验，并规范 `[1m]` 等客户端后缀。 |
| #7074 / `db76cc4e4` | 已移植 | 调度器统一读取 canonical Codex 5h/7d 用量与重置时间，绝对时间优先，relative reset 锚定快照时间。 |
| #7162 / `8447bdd36` | 已移植 | Antigravity Gemini SSE 不重复写事件分隔空行。 |
| #7129 / `087987070` | 已移植 | Responses 事件和本地失败事件始终输出 `sequence_number`，包括值为 0。 |

## 历史渠道缺口

此前的上游 tracking ancestry 已覆盖部分渠道提交，但本项目只接入了平台枚举和基础 Chat Completions 路由。此次重新按行为核对 `901a0439f`、`242907854`、`c44711ac9`、`c4e46c3be`、`e377c4358`、`ab9bd9e87`、`5032d0669` 等历史实现，补齐以下端到端能力：

- Kimi、智谱 GLM、DeepSeek、MiniMax 支持 `chat_completions`、`anthropic`、`responses` 和按入站 API 自适应模式；智谱没有原生 Responses 时继续走兼容转换。
- 自适应账号独立保存 `api_base_urls.chat_completions`、`api_base_urls.anthropic` 和可用时的 `api_base_urls.responses`。固定 Anthropic 模式的 `base_url` 明确表示 Anthropic 地址；固定 Chat/Responses 的 `base_url` 保持原协议地址。
- Kimi For Coding、智谱和 MiniMax Coding Plan 可手工查询滚动窗口；智谱团队套餐可配置组织和项目 ID。Kimi、DeepSeek 按量账号可手工查询余额。
- 探测请求复用账号级代理/IPv6 出口和请求头覆写。Kimi 自定义中转地址不显示也不调用固定 Moonshot 官方余额端点，避免把中转 Key 发往另一主机。
- OpenCode 成为一等平台，区分 Zen/Go 地址、模型目录、会话头、额度窗口、分组、Composite、监控和用户平台额度。自适应路由支持有序的精确模型 ID 或末尾 `*` 规则，首条命中生效，未命中走 Chat Completions。
- DeepSeek V4.1 Pro/Flash 和 vision 模型补齐目录、价格与能力；Pro 切换到 Flash 的时间边界按上游规则处理。自定义渠道或分组价格仍优先，不被官方 fallback 价格覆盖。
- 迁移 `244_add_opencode_go_platform.sql` 仅放宽四个 CHECK 约束，不改写旧账号、旧分组或既有迁移 checksum。

## 前端闭环

- 账号创建、编辑、筛选、平台徽标、渠道监控、Composite 路由、用户额度和 Ops 筛选均识别 OpenCode/CN 平台。
- 创建和编辑界面可配置三种自适应端点、OpenCode 模型协议规则、智谱团队字段；模式切换只替换仍等于旧官方默认值的地址，管理员自定义地址会保留。
- CN/OpenCode 用量单元格只显示持久快照，管理员点击刷新后才发起凭据请求；页面加载不会自动探测全部账号。
- API Key provider 选择只限制新增绑定的候选分组，多个已选分组仍按管理员排列顺序尝试。
- 英中渠道文案拆到 `accounts.channels.ts`，创建和编辑弹窗保持在仓库 1500 行维护门禁内。

## 升级与运行边界

- 旧 CN 账号没有 `api_protocol` 时仍按 `chat_completions`，不自动迁移为 adaptive。
- OpenCode 缺少 `api_protocol` 时按 adaptive，并使用 Zen/Go 内置规则；显式空规则表示全部回退 Chat Completions。
- CN quota/balance 使用 `singleflight` 合并同账号并发查询；账号校验在加入 flight 和发出网络请求前完成。
- 热请求只增加常数次字段读取和模型规则的有界线性匹配，规则上限为 64；没有无界缓存、后台全量扫描或自动凭据探测。
- 本项目 `VERSION`、README 无赞助内容、现有 migration 历史及有序多分组路由保持不变。

## Docker 验证

### 后端

```sh
docker run --rm -e GOMAXPROCS=4 -v "$PWD/backend":/workspace -v /Users/lin/go/pkg/mod:/go/pkg/mod:ro -v sub2api-channel-sync-go-cache:/root/.cache/go-build -w /workspace golang:1.26.6-alpine sh -c 'go test -tags=unit -p=2 ./...'
```

结果：所有 Go unit 包通过，exit 0。`internal/application/service` 首轮非缓存耗时 `187.837s`；修正 API 合约夹具后全量复跑通过。

```sh
docker run --rm -e GOMAXPROCS=4 -v "$PWD/backend":/workspace -v /Users/lin/go/pkg/mod:/go/pkg/mod:ro -v sub2api-channel-sync-go-cache:/root/.cache/go-build -w /workspace golang:1.26.6-bookworm sh -c 'go test -race -tags=unit -parallel=1 ./internal/application/service -run "Test(CNProvider|OpenCode|OpenAIQuotaHeadroomFactor|OpenAISchedulerCanonical|HandleCC.*Anthropic|HandleResponses.*)" -count=1'
docker run --rm -e GOMAXPROCS=4 -v "$PWD/backend":/workspace -v /Users/lin/go/pkg/mod:/go/pkg/mod:ro -v sub2api-channel-sync-go-cache:/root/.cache/go-build -w /workspace golang:1.26.6-bookworm sh -c 'go test -race -tags=unit ./internal/shared/apicompat -run "Test(CCChain|ResponsesAnthropicEventToSSE|AnthropicToResponses|ChatChunkToSSE|ResponsesStreamEvent)" -count=1'
```

结果：两批 race 均通过，分别为 `ok .../application/service 1.090s` 和 `ok .../shared/apicompat 1.027s`。`-parallel=1` 排除了既有并行测试反复调用 Gin 全局 `SetMode` 的夹具竞态；CN singleflight 用例内部仍并发 16 个跟随请求并断言仅一次出站。

### 前端

```sh
docker run --rm -v "$PWD":/workspace -v sub2api-channel-sync-node-modules:/workspace/frontend/node_modules -w /workspace/frontend node:24-alpine sh -c 'corepack enable && corepack prepare pnpm@11.17.0 --activate >/dev/null && pnpm lint:check && pnpm typecheck'
docker run --rm -e NODE_OPTIONS=--max-old-space-size=4096 -v "$PWD":/workspace -v sub2api-channel-sync-node-modules:/workspace/frontend/node_modules -w /workspace/frontend node:24-alpine sh -c 'node_modules/.bin/vue-tsc -b && node_modules/.bin/vite build --config vite.config.ts'
docker run --rm -e CI=true -v "$PWD":/workspace -v sub2api-channel-sync-node-modules:/workspace/frontend/node_modules -w /workspace/frontend node:24-alpine sh -c 'corepack enable && corepack prepare pnpm@11.17.0 --activate >/dev/null && pnpm test:run'
```

- lint、typecheck 和 production build 均 exit 0。
- 全量 Vitest：368 个文件中 364 个通过；2,237 个用例中 2,230 个通过。
- 7 个失败均在原始 `df540cea9` 复现：`UsersPage.spec.ts` 4 个 Pinia fixture、`UserEditDialog.spec.ts` 1 个 Pinia fixture、`settingsGatewayResilienceModularization.spec.ts` 1 个旧模板哈希、`RegisterViewCredentialStorage.spec.ts` 1 个注册跳转断言。
- 本次新增和相关门禁复跑为 93/93 通过；创建、编辑和用量组件最终矩阵为 79/79 通过。

### 镜像与运行态

- 最终镜像：`sub2api-channel-sync-20260916:candidate`，manifest/image ID `sha256:5e6d9f1e318acf8ed0a32228c6c5a2189208a2f221ddbf2cc2bdd68e43d6f0fb`，大小 46,201,691 bytes。
- 隔离栈：`diagnostics/upstream-channel-sync-20260916/compose.yaml`，PostgreSQL 18 + Redis 8 + candidate，应用仅绑定 `127.0.0.1:18080`。
- `/health` 返回 HTTP 200 `{"status":"ok"}`；`/ready` 返回 HTTP 200，`ready=true`、`current_version=0.1.204`、`draining=false`。
- 三个容器均为 `healthy` 且 restart count 0；Redis 返回 `PONG`。
- `schema_migrations` 包含 `244_add_opencode_go_platform.sql`；`user_platform_quotas`、`composite_model_routes`、`channel_monitors`、`channel_monitor_request_templates` 四个实际 CHECK 定义均包含 `opencode_go`。

### 浏览器

- 候选容器完成真实管理员登录、桌面 1440x1000 和手机 390x844 检查。
- Kimi adaptive 三端点、OpenCode Go 三端点与 5 条规则、28 个模型、账号创建后编辑回填均可见且一致。
- 手机规则采用两行布局，`Anthropic Messages` 完整显示，删除按钮和固定底栏无重叠。
- API Key 创建器显示 Anthropic/OpenAI/国产模型/其他 provider 初筛，并保留有序多分组说明。

## 差异关闭

跟踪合并 `0fd90b4bd1fa590cf1a68be7e6c481aebcdbb839` 的父提交为 `b4675ae5d39036951cb77260ec3f30453bd73bff` 和 `881f3202694c6bc932446931a30c27d9675178b9`。合并前后 tree 均为 `340e1082e3789bc59c284a1e165988fe227131d4`，`git diff HEAD^1 HEAD` 为空，`git rev-list --count HEAD..upstream/main` 为 0。分支尚未推送。
