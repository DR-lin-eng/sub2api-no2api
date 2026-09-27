# 上游主线增量审查（2026-09-27）

## 冻结边界及兼容原则

- 本项下游基线 `516e30762b0ecf3453742f57569d05d2b71f3690`，前次已审查上游边界 `7c0a2a556c440836c54b3f3135f033755f15ecb7`，本次冻结 `a3eb7ef302961cba716dc78b39b93b60c467db0e`。范围为 15 个 first-parent 提交、11 个合并 PR，另有 4 个版本/文档/测试直接提交。
- 按语义移植，不复制上游旧 `internal/service`、`internal/repository`、`frontend/src/views`、生成的 `wire_gen.go`、VERSION、赞助内容或迁移编号。保留现有 API/错误、SSE/WS、计费、调度、加密密钥和管理端权限契约。
- 此表中“已覆盖”关闭同一行为的重复审查；“已移植”关闭本轮实际落地行为；“专项暂缓”只关闭本次评估范围，**不代表功能已合入**。后续须按 PR 编号重开，不能仅以 Git 祖先关系推断功能覆盖。

## 差异台账

| PR | merge SHA | 范围 / 当前 owner | 处理 | 兼容与性能结论 |
| --- | --- | --- | --- | --- |
| #7491 | `31a6e7477` | OpenAI 账号测试模型选择；`transport/http/handler/admin/account_handler.go` | **已覆盖映射分支** | 当前 `GetAvailableModels` 直接投影显式 `model_mapping` 的公开名，测试已覆盖 OAuth 映射与透传默认回退；不额外请求或解码上游模型目录，保持原默认回退契约。上游动态目录投影若需引入，另行评估。 |
| #7513 | `61e7d3160` | 视频价格预览；`features/channels-user/presentation/widgets` 和中英文 i18n | **已移植** | 仅在现有 pricing popover 展示视频单价、每秒单位和分档；不改服务端定价/结算、请求数或列表行数。基线回归 2 例失败，修复后通过。 |
| #6844 | `f53252e29` | 上游旧 `GroupsView` Pinia 测试 fixture | **已覆盖** | 本项目已使用模块化 admin-groups 组件/测试，不存在该旧测试路径；不把测试 fixture 回灌生产代码。 |
| #4867 | `6b71d75bd` | S3 SecretAccessKey 二次保存；`application/service/backup_service.go` | **已移植** | 解密后继承的明文必须重新加密再入库；保留无 secret 且无持久密钥时允许保存空配置的旧行为。仅管理员保存路径多一次加密，不触及备份流或网关热路径。基线回归失败，修复后验证三次保存且无二次套壳。 |
| #7288 | `1947add84` | 备份月度归档/保留；backup 应用/HTTP/UI | **专项暂缓，未移植** | 新 checkpoint、跨节点锁与删除保留策略影响历史备份恢复和存储用量；需要独立迁移/回滚、长周期保留和并发删除验证。 |
| #7090 | `b2d6b954a` | 账号费率自动回退；OpenAI 调度分值/缓存/设置/UI | **专项暂缓，未移植** | 本项目已有按候选一次扫描的 `openAIUpstreamCostFactors`、OAuth 倍率及粘性/并发选取逻辑；上游把倍率从数值改可空、改排序决策与管理默认值，不能直接替换，需同样候选集的基线/候选 benchmark 和选中账号分布对照。 |
| #7478 | `59a00631e` | simple mode API key 金额窗口；billing cache、HTTP/WS 准入及部署配置 | **专项暂缓，未移植** | 开关默认关闭，但启用后每次准入用 DB authoritative 读且 WS 每 turn 再检查，会增加数据库压力；本项目有可靠异步结算与多实例用量路径，需独立限流一致性/高并发压测。 |
| #7282 | `d8d5e1dd3` | 线下提现与 ledger operation ID；affiliate 仓储/handler/UI/migration | **专项暂缓，未移植** | 上游迁移 `240_affiliate_ledger_operation_id.sql` 与本项目既有 `240_add_group_distillation_mode.sql` 编号冲突；审批权限、幂等扣账和负载锁需要专项设计，不能整包照搬。 |
| #7509 | `4318a63bd` | GPT-6 Sol/Luna 与 Claude Opus 5.5 混合目录/协议/计费 | **Sol/Luna 已覆盖；余项专项暂缓** | 本项目已完整登记 Sol/Luna 的常量、别名、兜底价格、前端元数据及测试；不覆盖新目录与计费默认值。Opus 5.5 和协议转换变更尚未移植，需单独核实提供商费率和 SSE/WS 行为。 |
| #7532 | `afd069b16` | Claude CLI 自动版本同步；后台任务、设置、GitHub API、gateway 身份 | **专项暂缓，未移植** | 新每实例定时抓取虽有启动节流和版本单调护栏，但引入网络请求、持久设置、运行缓存和版本漂移；先审查多实例负载、禁用/回滚与 UA 兼容性。 |
| #5616 | `fd80b08c9` | OpenCode Go 用量窗口与更多平台；仓储、服务、管理 DTO/UI | **现有额度解析已覆盖；余项专项暂缓** | 本项目已有 OpenCode Go 额度解析/冷却状态和平台迁移；新账号列表持久窗口、批量资格与调度传播尚未合入，需证明查询批量化、索引和到期更新上限，避免按账户 N+1 请求或无界探测。 |

直接提交 `5e244e738` 为上游调度参数调整后的测试修复，随 #7090 暂缓；`033047b7d`、`d68a68fd0` 是旧文档清理，不引入；`a3eb7ef30` 是上游 VERSION 0.2.8，本项目版本独立，不覆盖。

## 验证和发布边界

回归、Docker 命令/结果及副本回滚的四件工件在 `diagnostics/upstream-sync-20260927/`；实际测试与结论以同目录 `VERIFICATION.txt` 为准。对选中的上游 SHA 只做 tree-preserving tracking merge，以关闭本次已审查提交范围；不据此声称七项暂缓功能已合入。发布需要区分 Docker 候选验证、推送的 PR 精确 SHA、合并后的主线 CI 与镜像工作流。
