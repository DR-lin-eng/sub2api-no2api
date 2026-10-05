# 上游主线同步审查（2026-10-05）

## 冻结边界与关闭规则

下游基线为 `f64e146d258f037ea88208b66011fbe421036c8d`；已审查上游边界为 `96f4c115c9749078f90cbf210a01d39baf3f53b6`；本轮冻结至 `b8dece9000c68815a5b867ca5a1e6f236e173905`。范围为 12 个主线合并 PR 和 3 个 VERSION 直接提交。8 个 PR 的行为按现有 owner 移植或重构，1 个上游专属文档差异关闭，3 个新协议/账务依赖项暂缓。

沿用语义移植与 tree-preserving tracking merge：保留本项目目录、公开协议、出口、计费与有序分组。上游 VERSION、生成代码和旧目录不覆盖本项目。机器台账的 `fully_closed=true` 关闭本轮已处理行为；`false` 是明确未合入的专项，后续按 PR 编号重开。Git 祖先关系不表示暂缓功能已经实现。前次台账不重复处理，本次仅从已提交记录恢复缺失的 9 月 30 日文档入口及忽略例外。

## PR 差异台账

| PR | 处理 | 当前 owner / 审查结论 |
| --- | --- | --- |
| [#7780](https://github.com/Wei-Shaw/sub2api/pull/7780) | 已移植 | shared/xai/cli_identity.go；repository/http_upstream.go；service/openai_gateway_grok.go。交互式 Grok 身份统一为 1.0.46，版本覆盖下限 1.0.13，按运行平台构造 UA；保留本项目固定默认版本、API 域名与路由/出口隔离。 |
| [#7425](https://github.com/Wei-Shaw/sub2api/pull/7425) | 专项暂缓（未合入） | 现有 shared/typesafe 仅为内部审核客户端；新原生网关需独立模块。原生 /v1/systemone 与平台枚举、计费/额度、审计和 UI 是新协议范围；迁移 241 与本项目 241_account_quality_runs.sql 冲突，上游 Do(proxyURL) 未携带本项目完整账号出口。需模块化路由与账户出口/失败/结算专项验证。 |
| [#7802](https://github.com/Wei-Shaw/sub2api/pull/7802) | 专项暂缓（未合入） | modules/payment；service/payment_order.go、payment_fulfillment.go。新增 bonus_amount 和阶梯报价改变到账金额、返利基数及退款边界；迁移 241 冲突。需在现有支付模块设计报价快照、实际支付/赠送拆分与升级回退，不将上游旧 service/Ent 生成代码整包覆盖。 |
| [#7673](https://github.com/Wei-Shaw/sub2api/pull/7673) | 已移植 | transport/http/server/routes/payment.go；platform/middleware/rate_limiter.go。匿名旧订单状态查询每 IP 每分钟 20 次，复用现有原子限流与受信客户端 IP；Redis 故障 fail-open。签名恢复令牌、回调和已登录订单路由保持原契约，不新增 Wire 图。 |
| [#7803](https://github.com/Wei-Shaw/sub2api/pull/7803) | 已移植 | repository/api_key_repo_sort.go；features/keys。按第一绑定分组名称在 SQL 分页前排序，无分组始终置后，ID 稳定排序；有序 fallback 绑定不变。提取排序 owner 以符合源码上限，查询次数和预加载不变。 |
| [#7674](https://github.com/Wei-Shaw/sub2api/pull/7674) | 已移植 | service/antigravity_upstream_error_sanitize.go、antigravity_gateway_gemini.go。只在客户端错误投影中清除项目/消费者/邮箱等账号池身份，保留 Gemini code/status/message 与 HTTP 状态；成功、重试、Ops 和计费路径不变。真实 stream/nonstream 转发回归覆盖，regex 预编译且不进入成功热路径。 |
| [#7676](https://github.com/Wei-Shaw/sub2api/pull/7676) | 重构后移植 | repository/email_cache_atomic.go；service/email_service.go、user_service.go。单 key 单次 Lua 完成旧 JSON 验证码校验、失败计数与成功消费，保留 TTL；重置链接原子消费支持旧明文及 sha256: 前缀。哈希写入需 password_reset_token_hash_storage_enabled=true，默认保持旧节点可读和原 resend 契约；无计数旁路 key、无网关 DB 查询。 |
| [#7773](https://github.com/Wei-Shaw/sub2api/pull/7773) | 文案已移植；依赖已覆盖 | core/i18n/locales/{en,zh}/admin/accounts.ts；frontend/package.json、pnpm-lock.yaml。修正文案说明账号错误处理与重试/切号相互独立；锁定 Axios 1.18.1 已高于上游升级要求，不重复变更依赖。 |
| [#7630](https://github.com/Wei-Shaw/sub2api/pull/7630) | 已移植并收敛生命周期 | features/admin-accounts/presentation/widgets/AccountPriorityCell.vue。连续点击 450ms 合并为 priority-only Action；输入、取消、失败恢复与卸载处理在同域 widget。离开页面清理未发出的保存，并忽略卸载后的返回，不触发额外轮询；直接导入本项目 updateAccount owner。 |
| [#7814](https://github.com/Wei-Shaw/sub2api/pull/7814) | 上游专属差异已关闭 | 上游 .github/SECURITY.md。不复制上游维护者邮箱、服务范围或披露承诺；本项目私密漏洞报告 API 返回 enabled=false，不添加无效的本项目报告链接或修改仓库设置。该上游专属文档差异不再重复审查。 |
| [#7816](https://github.com/Wei-Shaw/sub2api/pull/7816) | 已移植 | repository/usage_billing_repo.go。仅 ErrAPIKeyNotFound 跳过 Key 自身额度和窗口；余额、订阅、账号、平台额度及持久幂等结算继续执行。其他数据库错误仍回滚；不增加 SQL 或丢弃计费任务。 |
| [#7813](https://github.com/Wei-Shaw/sub2api/pull/7813) | 专项暂缓（依赖 #7425，未合入） | service/upstream_billing_probe.go。当前没有原生 TypeSafe 账号平台；不能只添加计费探测资格而漏掉协议、迁移和账号出口。随 #7425 专项引入，不重复修改现有内部审核客户端。 |

精确 merge SHA、上游 diff SHA-256、文件集合及关闭标志见 [机器台账](../diagnostics/upstream-sync-20261005/UPSTREAM_PRS.json)。三个直接提交 `42bc7f6c`、`458b92ab`、`b8dece90` 只改变上游 VERSION，保留本项目版本线。

## 平滑升级与性能

- 不新增 Ent schema、Wire provider 或数据库迁移。Docker 基线、升级、回退、恢复使用同一 PostgreSQL/Redis；299 个迁移记录不变，旧 Key 的 ID/名称/配额、管理员登录与旧订单状态保持兼容。
- 验证码沿用原 JSON key 与字段，在一次 Lua 中检查、更新失败次数和消费成功码，保留原 TTL；注册和通知邮箱均覆盖并发错误上限与单次成功。旧二进制仍能读 JSON，完整的原子保证从请求由新节点处理时生效。
- 重置 token 消费支持旧 64 位明文和带 `sha256:` 的哈希，并保留已有链接至原 TTL。为避免新链接在滚动升级期间被旧节点拒绝，哈希写入开关 `password_reset_token_hash_storage_enabled` 默认为关闭；关闭时维持旧链接格式和重复发送复用行为，开启后签发新 token 只存前缀哈希。该设置仅在发信 worker 读取，不进入网关热路径。
- 所有认证节点均部署此实现后，运维可在 `settings` 中将该键设为 `true`。若启用后需要回退到旧二进制，先设回 `false` 并等待已发出的哈希链接 30 分钟 TTL 结束；已有哈希链接不能由旧二进制验证。默认关闭状态下本轮 Docker 回退无需此等待。
- 真实 Redis 的相同 benchmark（300 次，3 轮，linux/arm64）：成功路径中位数 `75.079µs -> 39.239µs`，失败路径 `85.278µs -> 51.206µs`；命令数 `2 -> 1`，成功分配 `1040 -> 424 B/op`，失败分配约 `1355 -> 440 B/op`。数据是隔离开发机的局部测量，命令数变化是稳定边界，不代表全网关性能增幅。
- 错误脱敏只执行于失败响应，正则预编译；结算修复不增加 SQL；分组排序在数据库分页前完成且不增加列表查询轮数；优先级控件只在用户修改后发出合并的 Action，不新增轮询或调度请求。

## 验证与发布证据

[验证记录](../diagnostics/upstream-sync-20261005/VERIFICATION.txt) 保存 Docker 命令、结果和已知基线限制；[回退脚本](../diagnostics/upstream-sync-20261005/ROLLBACK.sh) 只恢复独立临时副本。源码与关键文件 hash、真实 Redis benchmark、四阶段运行记录和浏览器截图同目录保留。

Docker 全量后端 unit/integration、Go lint、race、前端 lint/typecheck、31 个相关测试与 245 个关键测试、完整源码构建、升级/回退及文档门禁分别核对。两个旧源码上限问题（`openai_ws_v2/passthrough_relay.go`、`pricing_service.go`）在冻结基线已经存在；本次新增的排序 owner 不新增上限例外。

浏览器验收使用隔离的组件页面，加载生产 widget、样式与中文词表，Action/Toast 为测试替身；验证 450ms 后保存结果及 390px 宽度无横向溢出。实际 Docker API 登录/Key/订单/重置链接由运行测试单独验证。发布后的 PR 精确 SHA、CI 与主线镜像工作流以 GitHub PR 和最终交付链接为准，避免修改已验证提交来追加运行结果。
