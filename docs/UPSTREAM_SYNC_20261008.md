# 上游主线同步审查（2026-10-08 冻结范围）

本轮从下游 `6fcc1541fae51d0f55b9d39fe2e6d144420e11fa` 出发，延续前次上游边界 `3f1a2ea0a760730e3bc528105c00b4ee4f23e469`，冻结至 `5fc0e486c3f6a8a191b8bd140f39b60457f611cf`。57 个主线合并 PR 的 GitHub 合并状态、merge SHA 与原始 diff 已核对；继续验收日期为 2026-10-09。

55 个 PR 的当前可用行为按本项目 owner 适配，2 个 PR 在本项目没有对应运行路径。57 条当前差异均登记关闭；51 条完全关闭，6 条保留独立前置功能的重开条件。`fully_closed=false` 的条目为 #7811、#7782、#7891、#7793、#7764、#7916，不表示本项目缺少本轮已经实现的修复，更不表示其未启用前置功能已经实现。此前 #7736 等独立暂缓台账原样保留。Git tracking merge 只登记审查祖先，不替代语义台账。

## 逐 PR 台账

| 上游 PR | 结果 | 当前实现与边界 |
| --- | --- | --- |
| [#7811](https://github.com/Wei-Shaw/sub2api/pull/7811) | 适配，关闭当前差异 | API Key manifest 中显式 null service_tiers 改为 []，不覆盖已有非空数组和未知字段；上游跨账号元数据求交尚无对应功能。 前置功能仍独立跟踪。 |
| [#7779](https://github.com/Wei-Shaw/sub2api/pull/7779) | 适配，关闭当前差异 | 后续 WS turn 经原认证缓存刷新选中分组；定价、利润门、结算同源。保留 Key/用户/订阅及连接选号，覆盖多分组和失败回退。 |
| [#7792](https://github.com/Wei-Shaw/sub2api/pull/7792) | 适配，关闭当前差异 | 透传 WS 图片输入/输出 token、image_gen 回填及累计计费；现有 usage parser 原样拆至小文件，避免扩大超长 relay。 |
| [#7785](https://github.com/Wei-Shaw/sub2api/pull/7785) | 适配，关闭当前差异 | Chat 命名工具选择扁平化；字符串和 Responses 结构保留。 |
| [#7786](https://github.com/Wei-Shaw/sub2api/pull/7786) | 适配，关闭当前差异 | 保留 developer 角色，不降为 user。 |
| [#7787](https://github.com/Wei-Shaw/sub2api/pull/7787) | 适配，关闭当前差异 | legacy function_call 和 function result 用稳定 call_id 配对并避开已有 ID，按名称 FIFO。 |
| [#7788](https://github.com/Wei-Shaw/sub2api/pull/7788) | 适配，关闭当前差异 | Responses refusal 在非流式、流式 delta 与缓冲结果中保留。 |
| [#7789](https://github.com/Wei-Shaw/sub2api/pull/7789) | 适配，关闭当前差异 | 四条 Anthropic 缓冲桥拒绝负 content_block index，保留现有 native pump。 |
| [#7869](https://github.com/Wei-Shaw/sub2api/pull/7869) | 适配，关闭当前差异 | thinking 块始终含 signature（允许空串），同步 Antigravity 与共享 JSON/SSE。 |
| [#7844](https://github.com/Wei-Shaw/sub2api/pull/7844) | 适配，关闭当前差异 | 保留客户端 1h TTL，将前序 ephemeral 断点升为 1h；最多四个断点的既有限制先执行。 |
| [#7827](https://github.com/Wei-Shaw/sub2api/pull/7827) | 适配，关闭当前差异 | 只透传客户端显式请求的两个工具变更 beta；不自动补 companion token，drop policy 优先。 |
| [#7910](https://github.com/Wei-Shaw/sub2api/pull/7910) | 适配，关闭当前差异 | namespace 无变化路径懒分配，用只读 string view 和一次 span-copy；非目标字节保持不变。 |
| [#7775](https://github.com/Wei-Shaw/sub2api/pull/7775) | 适配，关闭当前差异 | Grok Responses 空 completed 视为上游无语义输出，可在提交前换号，不成功记 0/0。 |
| [#7854](https://github.com/Wei-Shaw/sub2api/pull/7854) | 适配，关闭当前差异 | Anthropic ping 转为 SSE 注释并立即 flush，不泄漏 event: ping，不启动 TTFT。include_usage 保留本项目契约。 |
| [#7891](https://github.com/Wei-Shaw/sub2api/pull/7891) | 适配，关闭当前差异 | 统一账单探测限制 Retry-After 在 24h 内；本项目无独立 opencode_go_usage，现有通用 http_error 不把 403 误判为订阅缺失。 前置功能仍独立跟踪。 |
| [#7889](https://github.com/Wei-Shaw/sub2api/pull/7889) | 适配，关闭当前差异 | Zen qwen3.8-max 使用 Chat；显式自定义规则保留。通用入口和连接测试拒绝专属 Gemini/System One 模型，原生适配器不改。 |
| [#7887](https://github.com/Wei-Shaw/sub2api/pull/7887) | 适配，关闭当前差异 | xAI SSO 解析隐藏 consent_token，首跳携带 Origin/Referer；重定向可信域和 2MiB 限制保留。 |
| [#7877](https://github.com/Wei-Shaw/sub2api/pull/7877) | 适配，关闭当前差异 | 智谱官方根、Coding Plan paas/v4、转发 v1 三类探测路径区分，复用原 HTTP client/安全限制。 |
| [#7837](https://github.com/Wei-Shaw/sub2api/pull/7837) | 适配，关闭当前差异 | Antigravity 连通性模型显示显式 request-side 映射名，空映射使用原默认目录。 |
| [#7878](https://github.com/Wei-Shaw/sub2api/pull/7878) | 适配，关闭当前差异 | 连通性错误记录绑定账号/平台/test_id/source，先脱敏再限长，后台与 HTTP 保留请求关联。 |
| [#7782](https://github.com/Wei-Shaw/sub2api/pull/7782) | 适配，关闭当前差异 | 已有普通 Codex manifest 端点按显式账号映射投影，保留元数据、不改共享缓存，identity mapping 保留 ETag；无 pinned 聚合前置。 前置功能仍独立跟踪。 |
| [#7822](https://github.com/Wei-Shaw/sub2api/pull/7822) | 适配，关闭当前差异 | 裸 API 别名不落入嵌入 SPA，保留原 OAuth/网关前缀。 |
| [#7800](https://github.com/Wei-Shaw/sub2api/pull/7800) | 适配，关闭当前差异 | 只有空价格条目首次添加一个模型才自动填价，避免追加/批量模型静默重定价。 |
| [#7904](https://github.com/Wei-Shaw/sub2api/pull/7904) | 适配，关闭当前差异 | 分组倍率弹窗请求版本隔离切组/重开/卸载，保留独立 Query/Action 和原 300ms 搜索。 |
| [#7903](https://github.com/Wei-Shaw/sub2api/pull/7903) | 适配，关闭当前差异 | 后台 profile 刷新和迟到保存响应不覆盖编辑中的用户名草稿。 |
| [#7902](https://github.com/Wei-Shaw/sub2api/pull/7902) | 适配，关闭当前差异 | 错误透传开关以服务端返回 enabled 为准，避免并发反转本地值。 |
| [#7901](https://github.com/Wei-Shaw/sub2api/pull/7901) | 适配，关闭当前差异 | Airwallex SDK import/init 完成后检查卸载状态，禁止页面离开后挂载。 |
| [#7900](https://github.com/Wei-Shaw/sub2api/pull/7900) | 适配，关闭当前差异 | RPM 编辑使用完整 Number 并校验整数，拒绝小数、垃圾后缀、空值和负数。 |
| [#7843](https://github.com/Wei-Shaw/sub2api/pull/7843) | 适配，关闭当前差异 | 告警持续/冷却时长为范围内整数分钟，校验保持原 owner。 |
| [#7842](https://github.com/Wei-Shaw/sub2api/pull/7842) | 适配，关闭当前差异 | 运维设置只有全部读取成功后允许保存，读取失败/加载中保留禁用。 |
| [#7841](https://github.com/Wei-Shaw/sub2api/pull/7841) | 适配，关闭当前差异 | 告警事件筛选/翻页请求版本隔离，旧页结果和错误不修改新列表。 |
| [#7840](https://github.com/Wei-Shaw/sub2api/pull/7840) | 适配，关闭当前差异 | 监控模板请求版本隔离，关闭/卸载忽略旧结果。 |
| [#7839](https://github.com/Wei-Shaw/sub2api/pull/7839) | 适配，关闭当前差异 | 计划测试结果只归属当前展开计划，折叠/重开/卸载立即失效。 |
| [#7819](https://github.com/Wei-Shaw/sub2api/pull/7819) | 适配，关闭当前差异 | Stripe 页面卸载后禁止迟到 SDK callback、挂载、poll timer 和 redirect timer。 |
| [#7818](https://github.com/Wei-Shaw/sub2api/pull/7818) | 适配，关闭当前差异 | 复用本项目已有 markdownRenderRequestSeq 与清洗，仅补 nextTick 后检查和卸载失效。 |
| [#7833](https://github.com/Wei-Shaw/sub2api/pull/7833) | 适配，关闭当前差异 | 身份邮箱绑定保留用户输入草稿，切用户和成功绑定时明确重置。 |
| [#7897](https://github.com/Wei-Shaw/sub2api/pull/7897) | 适配，关闭当前差异 | 长上下文标记改为准确文案，移至 user 词表供用户与管理用量共用，不再显示统一 x2。 |
| [#7853](https://github.com/Wei-Shaw/sub2api/pull/7853) | 适配，关闭当前差异 | 紧凑账号筛选保留 OAuth 额度和 BPS，更多筛选展示有效计数与 aria；移动布局浏览器验证。 |
| [#7793](https://github.com/Wei-Shaw/sub2api/pull/7793) | 适配，关闭当前差异 | 组合分组列出 exact public aliases，复用现有缓存编译快照；没有复建旧 resolver 或启用 composite Codex catalog。 前置功能仍独立跟踪。 |
| [#7911](https://github.com/Wei-Shaw/sub2api/pull/7911) | 适配，关闭当前差异 | TPS 精确分位数融合现有 latency scan，覆盖 raw/preagg/snapshot；过滤图片/Live，字段可选，前端现有 OpsMetricsGrid 渲染。 |
| [#7703](https://github.com/Wei-Shaw/sub2api/pull/7703) | 适配，关闭当前差异 | 加密 thinking signature invalid 的窄错误条件共用原单次恢复；保留 continuation 禁止恢复边界。 |
| [#7732](https://github.com/Wei-Shaw/sub2api/pull/7732) | 适配，关闭当前差异 | 同一 controller 并发 step-up 共享待决 promise，取消/完成清除；不改变后端 step-up 开关。 |
| [#7765](https://github.com/Wei-Shaw/sub2api/pull/7765) | 适配，关闭当前差异 | 合规 store reset 后隔离未完成请求，旧结果不覆盖新用户状态。 |
| [#7764](https://github.com/Wei-Shaw/sub2api/pull/7764) | 当前无该运行路径 | 当前项目未启用插件配置 UI/UISession，不存在该运行路径；保留独立前置功能和关闭原因。 前置功能仍独立跟踪。 |
| [#7731](https://github.com/Wei-Shaw/sub2api/pull/7731) | 适配，关闭当前差异 | 只有最后打开的 dialog 响应 Escape，子弹窗禁用 Escape 时父弹窗不关闭；复用独立 bodyScrollLock。 |
| [#7729](https://github.com/Wei-Shaw/sub2api/pull/7729) | 适配，关闭当前差异 | 备份页卸载后禁止迟到结果重启轮询。 |
| [#7724](https://github.com/Wei-Shaw/sub2api/pull/7724) | 适配，关闭当前差异 | 清理任务请求版本隔离分页和关闭，不让旧轮询覆盖新任务。 |
| [#7723](https://github.com/Wei-Shaw/sub2api/pull/7723) | 适配，关闭当前差异 | 批量图片权限查询失败保持 loaded=false，允许后续重试。 |
| [#7722](https://github.com/Wei-Shaw/sub2api/pull/7722) | 适配，关闭当前差异 | 公告标已读返回实际成功状态，失败保留未读并显示错误。 |
| [#7720](https://github.com/Wei-Shaw/sub2api/pull/7720) | 适配，关闭当前差异 | 用户零倍率使用 ?? 而非 ||，避免显示成 1x。 |
| [#7719](https://github.com/Wei-Shaw/sub2api/pull/7719) | 适配，关闭当前差异 | 通知邮箱验证码迟到响应不为移除项或卸载组件创建定时器。 |
| [#7718](https://github.com/Wei-Shaw/sub2api/pull/7718) | 适配，关闭当前差异 | 批量用户限制更新复用标准归一化 API 错误文本。 |
| [#7716](https://github.com/Wei-Shaw/sub2api/pull/7716) | 适配，关闭当前差异 | 价格显示通过 Number 保留指数，避免正则截断科学计数法。 |
| [#7728](https://github.com/Wei-Shaw/sub2api/pull/7728) | 适配，关闭当前差异 | OAuth 账户表单卸载后不启动验证码 cooldown。 |
| [#7725](https://github.com/Wei-Shaw/sub2api/pull/7725) | 适配，关闭当前差异 | Select/ProxySelector 变为 disabled 时关闭展开列表并清代理搜索。 |
| [#7721](https://github.com/Wei-Shaw/sub2api/pull/7721) | 适配，关闭当前差异 | 小于 1 的 bytes/rate 单位索引至少 0，显示 B 不再 undefined。 |
| [#7916](https://github.com/Wei-Shaw/sub2api/pull/7916) | 当前无该运行路径 | 当前项目无上游 channel_monitor_v2 最小样本健康阈值模块；当前 passive 监控契约保持，不伪造 V2 完成。 前置功能仍独立跟踪。 |

完整 owner、关闭/重开条件、GitHub merge SHA、原始与本地 diff SHA-256 见 [机器台账](../diagnostics/upstream-sync-20261008/UPSTREAM_PRS.json)。已经重构的 native stream pump、组合路由缓存、可靠结算、统一探测和 feature Query/Action 保持原 owner；没有恢复上游旧 service/handler/components 目录。

## 计费、协议与升级边界

WS 后续 turn 通过原 API Key 认证缓存读取当前选中分组，并把该快照用于利润门与结算；不会修改连接已选账号、Key/用户/订阅身份，也不会因为 Key 改组而把已建连接迁移到其他组。多分组 Key 即使所选分组不是第一绑定也可以刷新其价格。刷新失败仍采用原连接分组。快照仅保留当前/上一轮，可靠计费任务捕获所属 turn 的快照。现有跨 turn channel/model、请求身份与幂等结算继续保留。

透传 WS 图片输入 token、工具 image_gen 的回填和累计进入现有用量结构；没有重写 relay 终态或流式结算。Chat developer、legacy 函数配对、命名工具选择、Responses refusal、thinking signature、负 block index、Anthropic TTL/beta、Grok 空终态和 SSE 注释在当前适配器中验证，继续保留 HTTP/WS 重试与 continuation 恢复禁止边界。

TPS 字段 `output_tps` 在混合版本期间可选，旧节点或统计超时显示 —；速率使用每条有效记录的输出 token / 总耗时（包含首字等待），不重复相加 reasoning token。图片、Live、无正输出/耗时记录排除。共享用量表的长上下文和速率文案放入 user 词表，管理员和用户路由均可加载。账号筛选保留本项目 OAuth 额度和 BPS，API 请求参数不变。后台刷新、弹窗/页面关闭和迟到保存不得覆盖当前草稿或重启旧定时器。

本轮没有 Ent schema、数据库迁移、Wire、公开必填字段、默认部署参数或原支付状态变更。PostgreSQL 18 / Redis 8 的同库矩阵从基线到候选、回退再候选，四阶段的管理员邮箱/hash/余额、Key 名称/限额和历史订单快照一致，迁移数均为 301；每次替换应用使用新数据目录及无效旧初始化环境变量。真实 Cookie 与 RSA-OAEP/AES-GCM 登录、健康/就绪及嵌入前端均通过。另有 7 个新安装边界场景通过。

## 性能证据

namespace 改写使用已有只读 string view 和一次 span-copy。相同 Docker Go 1.26.6、linux/arm64、GOMAXPROCS=2，8MiB 无需改写的 nested/保留工具调用历史，从约 25,190,448 B/op、4 allocs/op 降为 0 B/op、0 allocs/op；nested 中位数约 6.91ms -> 6.02ms。确需删除 namespace 时从约 33.59MB/op 降至 16.79MB/op，单次延迟保持约 8.5–8.8ms。样本为 10 次/轮、3 轮，不等于全网关吞吐量；原始数据见 [基线](../diagnostics/upstream-sync-20261008/records/baseline-benchmark.stdout) 与 [候选](../diagnostics/upstream-sync-20261008/records/namespace-benchmark-reviewed.stdout)。

上游 TPS 方案为现有 overview 额外发起原始记录扫描。本项目将 TPS 分位数合并到原有 latency 查询，在 raw、preagg 和共享 snapshot 三条入口保持相同查询轮数与 2s 超时预算，不新增 goroutine、全量缓存或抽样。真实 PostgreSQL 10 万行中，独立 latency+TPS 两条查询中位数 144.06ms，合并查询 144.89ms；减少查询/扫描，没有宣称该样本中的延迟加速。精确样本、P5/P10/P50/Avg、无样本、图片/Live 排除与两个查询路径已验收，见 [PostgreSQL 记录](../diagnostics/upstream-sync-20261008/records/tps-real-postgres.stdout)。组合模型 alias 查询复用既有编译缓存，没有每次恢复旧 resolver 的数据库读取。

## 验证与回退

[VERIFICATION.txt](../diagnostics/upstream-sync-20261008/VERIFICATION.txt) 和 `records/` 保存实际命令、输入、逐字 stdout/stderr、退出码和 hash。Docker 全量后端 unit/integration、新增 WS race、golangci-lint、前端 lint/typecheck 与 29 个必需文件的 247 条测试通过；扩展前端 276 个文件、1739 条测试通过。真实桌面/390px 移动组件验证保留 BPS/额度筛选、有效计数和 TPS 展示，没有横向溢出，截图和观察值见 [浏览器记录](../diagnostics/upstream-sync-20261008/BROWSER_VERIFICATION.json)。

5 个既有扩展前端失败文件在精确基线上独立重现：adminAccountsModularization、settingsGatewayResilienceModularization、UserEditDialog、UsersPage、RegisterViewCredentialStorage，共 8 个失败用例；它们相关运行代码/原断言未改。扩展回归明确排除这 5 个文件，必需 CI 未排除任何用例。源码布局的基线有 3 个超长文件；本轮修改 WS relay 时按 usage parser 职责拆分，使其符合 1200 行限制。其余 payment_fulfillment_test.go 1201 行、pricing_service.go 1238 行为基线既有，逐字节未改，allowlist 未扩张。依赖审计仅沿用原 node-forge 例外，无新增/延期。

四个原生角色为源码 tar 包 `MODIFIED_FILE`、重建源码的 `DIFF_FILE`、验证记录 `VERIFICATION.txt` 及可执行 `ROLLBACK.sh`。回退脚本接受独立修改副本，利用同目录 `BASELINE_FILE` 与原始 hash 清单恢复全部改动的源码并移除新增文件；不会操作原工作区。相同命名工具 fixture 的基线行为为嵌套 function、回归退出 1，修改后为扁平 name、退出 0，独立回退后恢复嵌套行为、退出 1。回退源 hash 与基线逐项一致，随后重施 DIFF_FILE 重建的源码 hash 与 MODIFIED_FILE 一致。

本项目 PR、精确 head/main SHA、CI/Security/Docker 和多架构 GHCR 发布由 GitHub 单独核对，记录于最终交付与发布证据，不把本地验证当作主线发布完成。
