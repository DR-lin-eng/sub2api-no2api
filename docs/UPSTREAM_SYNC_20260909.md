# 上游同步审查记录（2026-09-09）

本批冻结并审查上游 `main` 的 `14e0a49e17afebf62c5f788f4ef1dc8eef56ac76..270eac6973049fe1b50eb75560a74a029e82884c`。本项目基线为 `dd31d9958d7d62c9e212e2c1ac54967cbd9d733a`（远端 `origin/main`，版本 0.1.196），上游新增 70 个提交、30 个合并 PR。模块化目录仍是事实源；不把上游 legacy `internal/service`、`internal/handler` 或 `frontend/src/views` 整树合入。

## 选择性移植

| 上游 PR | 处理 | 升级与性能边界 |
| --- | --- | --- |
| #6811 proxy partial update | 在 `application/service` 合并已存在代理状态；DTO 用 `NullableInt64Field` 区分省略与显式 null，导入流程显式清空字段。 | PUT 省略字段保持旧值；显式 null 才清空；只增加一次既有代理查询，不增加探测请求。 |
| #6815 directed proxy backups | Ent 源 schema 增加 `primary_proxies` 反向边，移除生成层错误的唯一约束；基线迁移 149 已是普通 FK/index，Ent 只补 inverse edge；不新增 SQL。 | 一个备用代理可被多个主代理共享；旧 `backup_proxy_id` 数据无需转换，迁移可重复执行。 |
| #6816 repeated proxy fallback | 过期回退更新使用 `COALESCE(proxy_fallback_origin_id, $1)`，并处理已经回退过的账号。 | 保留首次 origin，手动回切仍能恢复原代理；SQL 仍是单次批量 UPDATE。 |
| #6810 / #6836 date range | 服务端拒绝 `time.Time` 年份超出 JSON 可表达范围；管理端日期输入限制到 `9999-12-31`。 | 旧合法日期和 null 不变，避免升级后响应序列化失败。 |
| #6814 go-redis 9.22 | 升级 go-redis、cpuid、atomic，并将有序集合范围读取改为 `ZRangeArgs`。 | 只改变 Redis 客户端 API 调用，Lua、键和 TTL 不变；减少弃用路径，未增加往返。 |
| #6320 runtime block cooldown | 保留现有本地 block 的权威恢复与 generation 合同；不接入会清理新写入保护的旧快路径。 | 需上游明确 generation/持久状态契约后重开。 |
| #6754 client disconnect drain | 流式 Chat→Responses 上游请求使用独立可取消 context，并在关闭 body 前先取消。 | 客户端断开后及时停止上游读取，非流式和已提交语义输出路径不变。 |
| #6839 Grok external web access | Grok 原生 Chat 直转路径复用递归不支持字段清洗，移除 `external_web_access`。 | 无字段时只做 byte contains 快速分支；有字段时单次 JSON 清洗，Responses 行为保持一致。 |
| #6838 Claude probe any model | 将解析出的 `max_tokens` 带入验证 body，并允许任意模型的显式 `max_tokens=1` 探测绕过。 | 仍要求 Claude CLI UA；只放行单 token 探测，不放宽普通 messages 校验。 |
| #6812 model plaza subscriptions | 登录用户的广场可见专属分组集合合并有效订阅分组。 | 仅广场低频查询增加一次已有订阅仓库读取；网关鉴权与可绑定权限不改变。 |
| #6819 registration visibility | 登录页仅在公共设置加载成功且 `registration_enabled=true` 时显示注册链接。 | 设置失败时隐藏入口；后端注册鉴权继续是最终约束。 |
| #6762 payment help Markdown | 支付帮助文本经 `marked` 和 `DOMPurify` 渲染到现有 Markdown 样式 owner。 | 仅展示路径解析文本；无网络、状态或支付协议变化。 |
| #6764 failed refresh selection | 批量刷新失败时保留失败账号 ID（无 ID 时保留原选择）。 | 不重复请求；只更新前端选择状态。 |
| #6821 Antigravity plan type | OAuth token DTO 暴露 `plan_type`，创建凭据时保留该字段。 | 可选字段为空时完全兼容旧响应；不改变 token 交换。 |
| #6513 Apple container subnet | 新增可选 `APPLE_CONTAINER_NETWORK_SUBNET`，创建网络时传递并校验已有网络冲突。 | 默认空值沿用自动分配；冲突时停止且保留容器与持久卷。 |

## 已覆盖或关闭重复差异

- #6791 的 `groups.model_allowlist` 自愈迁移依赖上游列重命名，而本项目继续使用 `models_list_config` 的兼容 schema；直接移植会破坏旧数据库，保留为独立迁移设计项。
- #6235 channel cache invalidation 已移植为 Redis pub/sub 双节点失效，带 generation、singleflight 重试和 Stop 幂等测试。
- #6434 client disconnect drain：当前 WS 取消读可能丢失 partial usage；上游 blind drain 不适配本 fork，保留为需要 websocket close/read 语义设计的后续项。
- #6424 Ops access log 已由 request middleware 写入 `ops_system_log_skip`，sink 队列有界；不重复加入上游默认持久化开关。
- #6281 long-stream HTTP/2 keepalive 已由 `HTTPUpstreamProfileOpenAIStream`、显式 HTTP/2 PING 和 30 秒 WS sweep 覆盖；上游新增未使用的 profile 不引入。
- #6488 Grok media eligibility：后端已有能力，专用 UI/GET-PUT owner 尚缺，保留后续；#6821 plan 解析、#6798 账号菜单浮层、#6659 非活跃分组保护已移植到模块化 owner。

## 逐 PR 关闭台账

冻结区间共 30 个合并 PR；下表保留每个差异的唯一关闭结论，后续只从 `270eac6973049fe1b50eb75560a74a029e82884c` 之后重新审查。

| PR | 结论 |
| --- | --- |
| #6811, #6815, #6816, #6810, #6836, #6814 | 已移植并有单元/集成回归；代理迁移无物理 schema 变化。 |
| #6372, #6754, #6839, #6838, #6812, #6819 | 已移植并验证边界行为。 |
| #6762, #6764, #6821, #6513, #6798, #6659, #6706 | 已移植到当前前端/部署 owner；保留现有权限和 sandbox 约束。 |
| #6235, #6424 | 已按本项目 Redis 失效和日志 retention owner 重构；不复制 legacy 默认值。 |
| #6320, #6434, #6488 | 部分实现或延期；缺少与当前 runtime/WS/UI owner 同构契约，已关闭本轮差异。 |
| #6791, #6758, #6759, #6760, #6775, #6281 | 不适用或延期；涉及不同 schema、平台、统计窗口、插件树或未接线 profile。 |

## 暂缓并关闭本批差异

#6758 MiniMax 首类平台、#6759 channel-monitor 用户排行开关、#6760 周期成本估算、#6775 Windows ZIP 读取器、以及 #6810 之外的完整 announcement/proxy legacy UI 均需要独立的 feature owner、协议或迁移设计；当前 owner 没有同构缺口，本批不引入。上游 README sponsor 更新和 VERSION-only 提交也不进入 fork。

## 性能与平滑升级边界

- 热路径没有新增无界缓存、goroutine 或 ticker；Redis 读取仍是单次命令，Grok 清洗无字段时为常数级快速分支。
- 代理 schema 迁移只删除可能存在的 `backup_proxy_id` 唯一约束并确保普通索引；不改现有账号绑定值。
- 运行时 block 使用进程内锁和 generation，持久 cooldown 仍由调度快照负责；滚动升级时旧实例可继续读取旧字段。
- Payment Markdown 和登录入口均以旧默认值保持可用，前端 API JSON 字段向后兼容。

## 验证与差异关闭

基线和修改后命令、字面输出、退出状态、Docker 镜像摘要、运行时升级/回退和独立副本回滚记录在 [`diagnostics/upstream-sync-20260909/VERIFICATION.txt`](../diagnostics/upstream-sync-20260909/VERIFICATION.txt)。完成验证后以 tree-preserving tracking merge 关闭固定上游 SHA；以后只从该 SHA 之后重新审查。

## 增量批次：上游 PR #6874（2026-09-10）

本轮从已关闭边界 `270eac6973049fe1b50eb75560a74a029e82884c` 继续审查到上游 `98d86915becae9fe9491a91ffc6defd5235c8d2b`。区间内唯一功能 PR 是 #6874（Image 2.5 OAuth），随后仅有上游版本提交 `0.2.4`；版本号不进入本项目，继续保留当前 `0.1.196` 兼容契约。

| 上游差异 | 当前 owner 与处理 | 升级与性能边界 |
| --- | --- | --- |
| #6874 Image 2.5 模型目录 | `internal/shared/openai` 与管理端模型白名单加入 `gpt-image-2.5-flare`、`gpt-image-2.5-sunburst`；图片定价资源加入两个模型及 `2026-09-08` 日期快照。 | 仅增加静态目录项；显式账号/分组白名单不会自动扩展。 |
| #6874 OAuth Images 主控 | 图片 Responses 主控默认从已下线的 `gpt-5.4-mini` 切换为 `gpt-5.6-luna`；`SUB2API_IMAGES_MAIN_MODEL` 可在 Compose 重新创建容器后覆盖。图片工具模型与文本主控保持独立。 | 读取一次短环境变量并复用既有请求构造；无迁移、无额外 I/O、无新增 goroutine。 |
| #6874 图片用量与错误 | `tool_usage.image_gen.input_tokens_details.image_tokens` 进入 `ImageInputTokens` 并受总输入 token 上限约束；主控模型被拒时直接透传错误，避免错误冷却图片能力；管理端测试显示主控/图片模型并识别 HTTP 200 SSE 错误。 | 保留现有计费分离和失败切换边界；只在错误或图片路径读取结构化字段。 |

### 差异关闭

上游 PR #6874 已按本项目 `transport -> application -> domain` owner 选择性移植；上游 legacy `internal/service`、`internal/pkg`、旧前端路径和 `VERSION=0.2.4` 不直接合入。上一批 `14e0a49e..270eac697` 的差异继续关闭，后续只审查 `98d86915b` 之后的新提交。

### 性能结论

图片主控覆盖只影响图片请求和以图片模型作为顶层模型的 Responses 归一化；显式文本主控保持原值。目录精确命中仍为 map 查找，缺失时沿用原有模糊匹配链路。Docker 现有大图片意图解析与定价索引 microbenchmark 对比记录在 [`diagnostics/upstream-sync-20260909-image25/VERIFICATION.txt`](../diagnostics/upstream-sync-20260909-image25/VERIFICATION.txt)，这两个基准的分配数保持不变。代码审查未发现新增数据库/网络访问；这些基准不代表真实出图吞吐或延迟。

### 增量验证

本轮基线、修改后、Docker 构建与 PostgreSQL 18 + Redis 8 滚动升级/回退，以及独立副本文件回滚证据均记录在 [`diagnostics/upstream-sync-20260909-image25/VERIFICATION.txt`](../diagnostics/upstream-sync-20260909-image25/VERIFICATION.txt)。

### 发布与最终回退

本项目 [PR #39](https://github.com/DR-lin-eng/sub2api-no2api/pull/39) 已合并为 `596b57446ce4b087b809ea116b2770729a318c81`，该 SHA 的 [CI](https://github.com/DR-lin-eng/sub2api-no2api/actions/runs/34383935613)、[Security Scan](https://github.com/DR-lin-eng/sub2api-no2api/actions/runs/34383935817) 和 [Docker Image](https://github.com/DR-lin-eng/sub2api-no2api/actions/runs/34383935558) 均通过。GHCR `sha-596b574` 摘要为 `sha256:8d638da1dea34fdaa092220fb6d8ab7b33f2911c8b4bf2ae30fe95bf58179832`，包含 amd64/arm64。

合并时保留了同期主线 `a65ac5551` 的分组模型白名单。该功能自带的迁移使最终基线从 292 条变为 293 条；这不属于 Image 2.5 批次新增迁移。最终文件回滚仅反向应用本批 16 个源码、测试和部署文件的 patch，保留 HEAD、同期主线提交、其他本地改动和未跟踪文件。审查台账和证据保留；冲突会在写入前使回滚停止。
