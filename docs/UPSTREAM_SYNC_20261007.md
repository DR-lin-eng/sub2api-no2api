# 上游主线同步审查（2026-10-07）

下游基线为 `043203dd645eb996c339ba84a808f96f34f5a73b`，包含本项目 #114–#118 的模型字段、计费身份、Codex 语音和 lineage 修复。前次已审查上游边界为 `b8dece9000c68815a5b867ca5a1e6f236e173905`；本轮冻结至 `3f1a2ea0a760730e3bc528105c00b4ee4f23e469`，范围为 3 个主线合并 PR 和 1 个 VERSION 提交。

沿用按当前 owner 适配及保留代码树的 tracking merge，不覆盖上游旧目录、生成文件或本项目版本线。此前已关闭差异不重复处理，既有 `fully_closed=false` 专项保留原台账；Git 祖先关系不表示这些暂缓功能已经实现。

## 差异关闭台账

| 上游 PR | 处理结果 | 当前 owner 与边界 |
| --- | --- | --- |
| [#7851](https://github.com/Wei-Shaw/sub2api/pull/7851) | 当前配置无对应差异，关闭 | `features/keys/presentation/widgets/UseKeyDialog.vue` 没有 `model_catalog_url` 或远程目录模式，当前 HTTP/WS、API Key 与兼容配置不触发此问题。前置 #7736 仍为暂缓专项；启用它时必须同时接入本 PR 的 `api_key_model_discovery` 开关，并验证客户端版本、认证模式和目录预算。 |
| [#7888](https://github.com/Wei-Shaw/sub2api/pull/7888) | 已有修复覆盖，关闭 | 本项目 #111 已在 `modules/payment/provider/easypay.go` 和 `application/service/payment_resume_service.go` 移除用户查询串、限制回调字段，并进一步校验重复参数、商户 PID、签名类型、金额及交易号。保留现有实现。 |
| [#7893](https://github.com/Wei-Shaw/sub2api/pull/7893) | 按现有 owner 适配，关闭 | 凭据规则进入 `bootstrap/setup/admin_credentials.go`，CLI/Web/自动安装复用；Compose、准备脚本、三语部署说明和 setup feature 一致。Vue 的运行时及构建依赖同步到 3.5.43，保留本项目 source-map-js、SheetJS 和既有审计约束。 |

[机器台账](../diagnostics/upstream-sync-20261007/UPSTREAM_PRS.json) 保存精确 merge SHA、原始 diff SHA-256、文件集合、关闭标志与重开条件。上游直接提交 `3f1a2ea0` 只更新 VERSION，保留本项目版本线。#7851 的关闭表示当前配置没有该差异，远程目录协议继续按 #7736 的独立专项管理。

## 首次安装与平滑升级

自动安装只在决定创建第一个管理员后检查凭据。已有管理员或已有普通用户时继续跳过创建，安装完成标记存在时继续采用持久数据库；旧 `.env` 中的无效初始化邮箱、短密码不会重写账号或阻断升级。

新安装时，空邮箱生成 `admin-<12 hex>@sub2api.local`，空密码沿用安全随机生成，日志输出首次凭据。提供的邮箱先去除两端空白，要求同时通过邮箱解析及登录接口的 `binding:"required,email"` 规则；超长邮箱在解析前拒绝。提供的密码须为 8–72 个 UTF-8 字节，与 bcrypt 实际输入边界一致，密码本身保留原值。先检查提供值再生成随机凭据，失败不会插入管理员、写配置或安装锁；数据库初始化迁移仍可能已完成。

CLI 和 Web 向导使用相同后端规则，浏览器按钮及中英文提示按 UTF-8 字节计算。部署准备脚本生成随机登录邮箱；随机源返回错误或空值时在写 `.env` 前退出。四个 Compose 示例去掉固定邮箱默认值，原部署的持久管理员账号仍由数据库保存。

本轮没有 Ent schema、Wire 或数据库迁移变化，不改变 JSON/SSE/WS、账号出口、调度、可靠结算及支付回调的既有实现。新凭据作为普通邮箱和 bcrypt hash 保存，旧 binary 可继续登录；旧 binary 搭配新 Compose 做全新安装时会沿用其旧默认邮箱，应按运行镜像的日志确认登录凭据。

## 性能审查

首次管理员创建保留原有两次用户计数查询及一次 INSERT，没有新增查询轮数、网关调用、goroutine 或队列。新增校验只进入安装路径；EasyPay 和客户端配置的已覆盖差异没有改写。

相同 Docker Go 1.26.6、linux/arm64、`GOMAXPROCS=2`，预先完成 validator 初始化后，每轮 5000 次、三轮测量：有效邮箱校验中位数为 `133.9 -> 617.1 ns/op`，分配 `96 -> 128 B/op`、`5 -> 7 allocs/op`。增加约 0.48 微秒用于保持与登录校验一致，只在首次安装/向导校验执行。密码边界校验两版均无分配。该测量不是冷启动耗时或网关吞吐量测试；本轮未引入网关热路径开销，无需为安装校验替换登录规则。原始输出见 [基线](../diagnostics/upstream-sync-20261007/benchmark-baseline.txt) 和 [候选](../diagnostics/upstream-sync-20261007/benchmark-candidate.txt)。

## 验证与发布

Docker 完整后端 unit/integration、安装 race、部署脚本、前端 lint/typecheck 与全部必需关键测试、两版生产镜像、真实 PostgreSQL/Redis 运行矩阵分别验证，详见 [验证记录](../diagnostics/upstream-sync-20261007/VERIFICATION.txt)。四阶段复用同一数据库和 Redis，替换应用时使用全新数据目录及无效旧初始化环境变量；管理员 hash/邮箱/余额、Key 名称/配额、历史待支付订单和迁移数均保持一致。

新安装覆盖短密码、73 字节密码、多字节超限、不可登录邮箱、显示名称邮箱、空凭据随机生成和带空白邮箱/72 字节多字节密码。拒绝场景均没有管理员、配置、安装锁或完成标记；随机账号及边界账号可通过真实 Cookie + RSA-OAEP/AES-GCM 登录。

前端必需门禁 29 个文件、247 条测试通过；扩展客户端配置/网络/路由回归 216 条通过。OpenCode 的一条旧字节快照在精确基线及候选均为 `6297 != 5734`，三个 owner/测试文件逐字节未变。保留原代码和断言，证据见 [基线记录](../diagnostics/upstream-sync-20261007/PREEXISTING_TEST.txt)。生产依赖审计通过原有例外校验，没有新增或延长例外。

源码布局检查在精确基线和候选均报告同样三个既有超长文件：`openai_ws_v2/passthrough_relay.go` 1273 行、`payment_fulfillment_test.go` 1201 行、`pricing_service.go` 1238 行。本轮未改这些文件或扩充 allowlist。

本项目 PR、精确发布 SHA、CI 和主线镜像结果在最终交付中核对；不为追加状态记录改写已验证源码提交。
