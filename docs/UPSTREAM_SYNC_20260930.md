# 上游主线同步审查（2026-09-30）

本次已完成记录的冻结范围为 `a3eb7ef302961cba716dc78b39b93b60c467db0e..96f4c115c9749078f90cbf210a01d39baf3f53b6`。此页从随 PR #106 提交的机器台账恢复缺失的文档入口，不重新处理原有差异。

完整文件、diff 哈希与关闭标志以 [原始 PR 台账](../diagnostics/upstream-sync-20260930/UPSTREAM_PRS.json) 为准；验证与性能证据见 [验证记录](../diagnostics/upstream-sync-20260930/VERIFICATION.txt)。`fully_closed=false` 项仍是未合入范围，Git tracking merge 不等同于功能已合入。

| PR | 处理 | owner | 已记录结论 |
| --- | --- | --- | --- |
| [#7622](https://github.com/Wei-Shaw/sub2api/pull/7622) | 已移植 | core/utils/ccswitchImport.ts | Codex 导入沿用根端点并去掉尾斜杠，默认模型与认证参数继续使用当前配置。 |
| [#7636](https://github.com/Wei-Shaw/sub2api/pull/7636) | 已移植 | bootstrap/setup/setup.go | 首次生成配置省去未消费的 rate_limit 字段；现有配置、限流与数据库均继续兼容。 |
| [#7628](https://github.com/Wei-Shaw/sub2api/pull/7628) | 已覆盖 | features/keys/presentation/widgets/UseKeyDialog.vue | 当前配置生成器没有 model_catalog_json 或 %userprofile% 模型目录字段，Windows 相对路径缺陷不存在；保留现有 provider 配置。 |
| [#7611](https://github.com/Wei-Shaw/sub2api/pull/7611) | 已移植 | features/admin-accounts/presentation/widgets/UsageProgressBar.vue | 空闲窗口有已知重置时间时展示倒计时；仅无时间时显示现在，定时器数量不变。 |
| [#7497](https://github.com/Wei-Shaw/sub2api/pull/7497) | 已移植 | features/admin-groups/presentation/widgets | 分组倍率/RPM 对话框卸载时清理 document listener 和待执行搜索，避免反复挂载积累。 |
| [#7549](https://github.com/Wei-Shaw/sub2api/pull/7549) | 已移植 | core/utils/ccswitchImport.ts、features/keys/presentation/pages/KeysPage.vue | CC Switch 用量脚本统一规范化末尾 /v1，根 URL、/v1 和尾斜杠均只请求一次 /v1/usage。 |
| [#7635](https://github.com/Wei-Shaw/sub2api/pull/7635) | 已移植 | shared/apicompat/chatcompletions_to_responses.go | Chat role item 显式声明 type=message；保留 reasoning 与 function_call 顺序。 |
| [#7393](https://github.com/Wei-Shaw/sub2api/pull/7393) | 已移植 | shared/antigravity/schema_cleaner.go | const 转 enum 后才进入已有过滤器；字符串 const 与现有 enum 求交集，保留递归与其他 schema 清理规则。 |
| [#7585](https://github.com/Wei-Shaw/sub2api/pull/7585) | 已移植 | shared/apicompat、shared/antigravity | data URI input_file 转 document，再转 Gemini inlineData；空数据与 file_id-only 保持既有丢弃契约。 |
| [#7570](https://github.com/Wei-Shaw/sub2api/pull/7570) | 已移植 | shared/apicompat/anthropic_to_responses_response.go | 完整 tool input 作为 seed，实际 delta 覆盖；终态 arguments 重复已发 delta，保留客户端工具恢复与空参数回退。 |
| [#7569](https://github.com/Wei-Shaw/sub2api/pull/7569) | 已移植 | shared/apicompat/responses_to_chatcompletions.go | 终态 message 无可用文本时补回已累计 delta；保留非空终态权威值及本项目重复 call ID/custom tool 保护。 |
| [#7568](https://github.com/Wei-Shaw/sub2api/pull/7568) | 已移植 | shared/apicompat/anthropic_to_responses.go | 按数值代际识别 GPT>=5，避免 GPT-6/6.1 兼容请求发送不支持的采样参数；数值溢出返回未知。 |
| [#7613](https://github.com/Wei-Shaw/sub2api/pull/7613) | 已覆盖零费用落库；Free Fast 前置项暂缓 | application/service/openai_gateway_usage.go | 当前 missing-pricing 分支已记录零费用用量，未引入会再次因 Standard 定价失败而丢行的 Free Fast 重算分支。 |
| [#7597](https://github.com/Wei-Shaw/sub2api/pull/7597) | 专项暂缓，未移植 | shared/claude、application/service/billing_service.go | 依赖上一轮未移植的 Opus 5.5 effort catalog 和签名 thinking；需与模型目录、提供商定价及协议限制一起验证。 |
| [#7572](https://github.com/Wei-Shaw/sub2api/pull/7572) | 已移植 | application/service/openai_alpha_search.go | Responses SSE 搜索只接受有效 response.completed；失败、不完整、裸 DONE 或 EOF 不作为成功/可计费用量。 |
| [#7571](https://github.com/Wei-Shaw/sub2api/pull/7571) | 已移植 | application/service/openai_apikey_responses_probe.go | 明确模型不可用的 400/404 保持 unknown，优先探测 GPT 文本模型；保留 15s/256KiB/512 token 原预算及 failed/incomplete 规则。 |
| [#7624](https://github.com/Wei-Shaw/sub2api/pull/7624) | 已移植调度分支 | application/service/openai_gateway_scheduling.go | 陈旧快照存在已知未来重置时间时继续暂停；过去重置或未知时间仍允许原自愈。本项目没有上游 account_scheduling_threshold_eval owner。 |
| [#7619](https://github.com/Wei-Shaw/sub2api/pull/7619) | 已移植 | application/service/account_stats_pricing.go | 默认模型文件统计成本读取 OpenAI 账号长上下文开关；自定义规则和应用客户售价的优先级不变，其他平台保留原默认。 |
| [#7524](https://github.com/Wei-Shaw/sub2api/pull/7524) | 已移植 | application/service/channel_plaza.go、transport/http/handler/model_plaza_handler.go、features/model-plaza | 在当前广场链路增加视频独立倍率投影、每秒单位与展示；客户端字段可缺省，滚动升级旧响应继续用分组倍率；不修改结算。 |
| [#7595](https://github.com/Wei-Shaw/sub2api/pull/7595) | 专项暂缓，未移植 | application/service/group_model_allowlist.go、features/admin-groups | 当前允许精确与末尾通配，所有准入仍由后端执行；上游任意位置通配会改变 listing 交集和热路径分配，需在当前缓存 owner 中设计编译/匹配和 benchmark。 |
| [#7573](https://github.com/Wei-Shaw/sub2api/pull/7573) | 专项暂缓，未移植 | application/service/billing_service.go、model_pricing_resolver.go | 上游把渠道空图片价格从显式免费/文本回退改为目录继承，会改变既有账号实收；当前测试明确保留 nil 与显式 0 契约，须独立配置迁移与账单对照。 |
| [#7610](https://github.com/Wei-Shaw/sub2api/pull/7610) | 已移植并保留本地调优 | deploy/docker-compose*.yml | 三份 Compose 的 Redis 改 exec 列表；保留正式/本地 save 空、AOF 关闭、内存/客户端上限；六组真实容器验证空密码和 shell 特殊字符密码均 PONG。 |
| [#7609](https://github.com/Wei-Shaw/sub2api/pull/7609) | 499 基础已覆盖；其余专项暂缓 | transport/http/handler、application/service/*transport_error.go | 当前 failoverClientGone 与 concurrencyErrorResponse 已标记未提交响应 499；上游更广的 Google/Antigravity/Ops 分类仍需逐协议对照，保留现有异常记录。 |
| [#7615](https://github.com/Wei-Shaw/sub2api/pull/7615) | 专项暂缓，未移植 | application/service/openai_ws_forwarder_ingress.go、openai_codex_continuation.go | 本项目有独立 continuation owner、账号身份重写和重放策略；根据客户端 window ID 切断链必须同时验证 native/HTTP bridge、跨账号亲和和恢复，不能替换旧 ingress。 |
| [#7555](https://github.com/Wei-Shaw/sub2api/pull/7555) | 专项暂缓，未移植 | infrastructure/repository/scheduler_cache.go、OpenAI quota 应用服务 | 本项目未接入上游自动用 reset-credit 后台服务；缓存投影/通知修复依赖该前置项。上游新增全局 sync.Map 冷却未清理，接入时应改为有界 owner 缓存。 |
| [#7617](https://github.com/Wei-Shaw/sub2api/pull/7617) | 已移植 | application/service/openai_gateway_request_build.go、openai_gateway_service.go | HTTP Responses 允许 caller beta，OAuth 去掉旧 experimental token、保留独立 beta；Messages bridge 仍清除 beta，API Key 由 caller 控制。 |
| [#7526](https://github.com/Wei-Shaw/sub2api/pull/7526) | 专项暂缓，未移植 | application/service/gateway_service.go | 当前目录同时使用显式映射和后台 OAuth capability snapshot；上游静态默认集合补齐会改变 pending snapshot 可见性，需保留本项目已有测试后重新设计合并规则。 |
| [#7562](https://github.com/Wei-Shaw/sub2api/pull/7562) | 已覆盖主要行为；provider 前缀变体暂缓 | application/service/openai_deepseek_compat.go | 当前账号/目标模型 DeepSeek semantics 与 Chat pipeline 已补 reasoning_content，OpenCode 的普通 deepseek-* 模型命中；提供商前缀变体须单独确认模型规范化，不重复改写已覆盖路径。 |
| [#7607](https://github.com/Wei-Shaw/sub2api/pull/7607) | 已移植到 typed stream owner | application/service/antigravity_gateway_compat_stream_session.go | 只有正文/thinking/tool 被视为语义输出；message_stop、stop_reason、signature-only 不提交 HTTP，MALFORMED 空流可换号。保留有界事件/字节预算和 typed sink。 |
| [#7538](https://github.com/Wei-Shaw/sub2api/pull/7538) | 已移植 | shared/apicompat、application/service/openai_compat_model.go | 显式 thinking=disabled 优先于 effort；Chat/Responses 同为 none，禁用时不添加 auto summary，也不被 max 覆盖。 |
| [#7638](https://github.com/Wei-Shaw/sub2api/pull/7638) | 已移植 | shared/claude/constants.go、application/service/gateway_upstream_request.go | OAuth mimic 仅保留明确请求的 structured-outputs 兼容 beta；策略 drop 仍优先，未知 beta 和固定默认列表不扩大。 |
| [#7683](https://github.com/Wei-Shaw/sub2api/pull/7683) | 专项暂缓，未移植 | shared/claude、application/service、frontend 模型元数据 | Sonnet 5.5 整包依赖上一轮 Opus 5.5 signed thinking/effort 基础，涉及强制工具选择、Bedrock/Vertex、签名重试及计费；需独立官方价格和多协议验收。 |
| [#7701](https://github.com/Wei-Shaw/sub2api/pull/7701) | 已移植并量化性能 | application/service/gateway_tool_rewrite.go | 收集原 body 的绝对位置后一次重写，保留非目标字段与 deferred tool 缓存断点；1000 历史调用 benchmark 从 61.6ms/137MB 降为 0.428ms/394KB。 |
| [#7646](https://github.com/Wei-Shaw/sub2api/pull/7646) | 专项暂缓，未移植 | infrastructure/repository/usage_log_repo_trend.go、admin-dashboard owner | 新消费 Top 用户排序会增加统计查询维度和缓存键；本项目 snapshot-v2/水位聚合为独立 owner，需比较 SQL plan 和切换期间缓存隔离。 |
| [#7643](https://github.com/Wei-Shaw/sub2api/pull/7643) | 专项暂缓，未移植 | application/service/antigravity_gateway_compat_stream.go | 15s 首内容前 ping 会提交 HTTP 200，随后空流/失败不再可换号；保留当前提交前重试契约和预算，后续需独立配置、协议错误及首 token 计时设计。 |
| [#7542](https://github.com/Wei-Shaw/sub2api/pull/7542) | 已移植 | features/admin-accounts/presentation/widgets/ModelWhitelistSelector.vue、create/edit/bulk 字段 owner | 手工白名单条目与非身份映射同源时提示并保留原目标；映射作为显式 prop 经现有字段组件传入，不增加页面状态或请求。 |
| [#7378](https://github.com/Wei-Shaw/sub2api/pull/7378) | 专项暂缓，未移植 | transport/http/handler/openai_responses_websocket.go、OpenAI 调度 | 上游新增 account_model 归属 gate 和连接级路由 resolver；本项目 composite routes 与 HTTP/WS identity/MapRequestModel owner 不同，需完整每 turn、sticky、legacy/advanced 调度对照并重生成 Wire。 |
| [#7380](https://github.com/Wei-Shaw/sub2api/pull/7380) | 专项暂缓，未移植 | application/service/gateway_forward_as_*、openai_gateway_anthropic_native_pump.go | 上游无视 include_usage 转发已接收 usage，并重新规范化 cache 桶；本项目共享 native pump、可靠结算和终态投影不同，需四条流式/非流式线验证 billing/outward 数值一致后移植。 |
| [#7684](https://github.com/Wei-Shaw/sub2api/pull/7684) | 专项暂缓，未移植 | Anthropic quota 应用/管理端模块 | 新增原生重置次数 GET、独立 HTTP client、路由、Wire 与 UI；须复用本项目账号级 proxy/IPv6/TLS/权限 owner，避免独立客户端绕开出口。 |
| [#7680](https://github.com/Wei-Shaw/sub2api/pull/7680) | 当前无本地目录依赖已覆盖 | features/keys/presentation/widgets/UseKeyDialog.vue | 当前 OpenAI Codex 配置不写 model_catalog_json，也不要求保存目录文件；本轮浏览器已核对配置内容，无需重复处理。 |
| [#7679](https://github.com/Wei-Shaw/sub2api/pull/7679) | 专项暂缓，未移植 | modules/securityaudit、application 内容审计、transport settings | 信任用户白名单改变本地处罚、HTTP/WS gate、后台累计与审计记录；需在本项目 coordinator/权限设置链统一实现，不能移植旧 settings/handler 单一路径。 |
| [#7678](https://github.com/Wei-Shaw/sub2api/pull/7678) | 专项暂缓，未移植 | features/keys/presentation/widgets/UseKeyDialog.vue | Claude Code only 的 tabs 与下游 fallback、API Key 多分组/首选绑定关联；需要当前有效 group 的 DTO/UI/服务端端点矩阵一起定义。 |
| [#7579](https://github.com/Wei-Shaw/sub2api/pull/7579) | 专项暂缓，未移植 | transport/http/handler/gateway_*、api_key_group_routing.go | 允许 Claude-only 兼容端点退回 fallback 会影响实际分组倍率、并发与多分组路由；需验证本项目请求内分组切换和可靠结算，不仅移除 handler 403。 |
| [#7752](https://github.com/Wei-Shaw/sub2api/pull/7752) | 专项暂缓，未移植 | platform/config、API Key 应用/仓储 | 上游默认 200 个/60 次会改变升级后的创建行为；CountByUserID 再 Create 存在并发窗口，并含媒体工坊内部 Key。应在当前事务 owner 设计用户锁、内部凭据资格与默认兼容开关。 |
| [#7681](https://github.com/Wei-Shaw/sub2api/pull/7681) | 专项暂缓，未移植 | billing 准入与可靠异步结算模块 | 默认打开 Redis 余额预占，准入/释放/续期新增调用与每流 goroutine，Lua 按用户预占集合求和；与本项目 Redis Stream pending 用量和多实例结算衔接须独立一致性/高并发测试。 |
| [#7736](https://github.com/Wei-Shaw/sub2api/pull/7736) | 专项暂缓，未移植 | features/keys 配置与模型目录 datasource | 上游默认 model_catalog_url 依赖较新 Codex 并引入远程目录/1MiB 回退；当前配置支持旧/新版 env_key，需显式客户端版本选择与完整目录预算验证。 |
| [#7730](https://github.com/Wei-Shaw/sub2api/pull/7730) | GPT-6.1 目录价格已覆盖；套餐和 Ultrafast 暂缓 | shared/openai、application pricing、features/keys/plan metadata | 下游 #105 已在冻结基线登记 GPT-6.1 Sol 常量、别名、官方价格、metadata 与测试；新 Codex 套餐、Ultrafast 和更广验证矩阵尚未移植。 |
| [#7726](https://github.com/Wei-Shaw/sub2api/pull/7726) | 专项暂缓，未移植 | Anthropic reset 独立模块、管理权限/审计 | 消费一次性上游 grant，新增机构级 Redis lease、幂等持久 fence、POST 与 UI 确认；依赖 #7684，须验证并发、超时未知结果、跨节点重试与回退恢复。 |
