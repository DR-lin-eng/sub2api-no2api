# 上游主线同步审查记录（2026-10-22）

本轮从项目主线 `0dd415b5569e306e33e6d194d672389210a06efd` 开始，冻结上游审查边界为 `21cc926933163e015d2b632be07055759c983541`，审查范围是 `8f2cbb53d9d1c40e1865b07ac460571f7d4c48de..21cc926933163e015d2b632be07055759c983541`。已有台账中的 `ported`、`covered`、`deferred` 和有意差异继续有效；只有上游语义变化或当前 owner 出现回归时才重新打开。

## 决策台账

机器可读台账见 [`diagnostics/upstream-sync-20261022/UPSTREAM_PRS.json`](../diagnostics/upstream-sync-20261022/UPSTREAM_PRS.json)。

| 上游 PR | 当前决定 | 适配结果 |
| --- | --- | --- |
| #8139 | ported | Responses 的 `response.queued` 与 `response.metadata` 保持前置元数据语义，不影响首次输出超时与重试。 |
| #8142 | ported | 管理端账号列表只持久化合法 platform/status 筛选，损坏或禁用 storage 时回退为空筛选。 |
| #8267 | covered | `follow-redirects`、Go 1.26.10 和 CI 版本已经由本项目主线覆盖；不重复回放依赖和工作流差异。 |
| #8166 | ported | service account 纳入账号配额判断、结算、通知、调度、DTO 和管理端编辑/展示；池模式仍只由原有 API key/Bedrock 判断。 |
| #8167 | ported | Gemini 系图片模型和 `responseModalities=IMAGE` 在入口及映射后执行分组门禁；保留 countTokens、媒体工坊和 composite 现有边界。 |
| #8210 | ported | Anthropic 兼容流在客户端断开后继续读取并合并最终用量，读间隔有界，部分用量进入结算。保留本项目工具恢复、签名思考和 Chat `include_usage` 语义。 |
| #8249 | ported | 单账号 Antigravity 判断改为请求内延迟探测并只执行一次，普通请求不提前枚举账号。 |
| #8250 | ported | 模型映射签名改为无排序、无分配的条目哈希，保留内容变化检测。 |
| #8251 | deferred | 上游进程内快照复用会在当前候选索引中引入十秒陈旧窗口、嵌套 map/指针共享和无界条目增长；当前 writer 没有对应修订号发布。需要独立的有界不可变快照设计和混合版本失效矩阵。 |
| #8269 | ported | 账号日/周配额累加按提示选择 SQL，并用数据库配置校验和完整 SQL 回退防止旧提示误导结算。 |
| #8224 | ported | DeepSeek Responses 图片输入统一为 `image_url`，移除重复 legacy `url`，保留 file_id-only 输入。 |
| #8188 | ported | Gemini 工具 schema 按 schema 节点与 property 名称分开清理，支持字符串 const/enum 交集并保留本地边界归一化。 |
| #8255 | ported | Cloudflare 1010 和 `browser_signature_banned` 作为边缘拦截处理，不增加账号惩罚；真实 403 策略保持不变。 |
| #8275 | covered | 上游 Go 1.27.3 不适用于当前项目已经验证的 Go 1.26.10 发行链，Docker、go.mod 和 CI 版本保持一致。 |

此前已关闭的账号配额、图片权限、搜索、支付、缓存用量和安全差异没有重复实现。赞助和合作方内容也没有进入根 README。

## 性能审查

基准使用同一 Docker Go 1.26.10 镜像、`GOMAXPROCS=2`、2 CPU、3 GiB 内存、每项 300 ms 三次采样，顺序执行于其它编译任务完成之后。映射签名在 8/128/1024 条目下分别约降低 62%/74%/81%；schema 清理约增加 1.8%；兼容 Chat/Responses 流分别约增加 14%/6%，启用超时路径约增加 16%/5%。字节级 SSE 解析将分配数降至基线以下，断连和慢客户端行为保持通过。该数据是固定夹具微基准，不代表生产网关吞吐。

#8251 的快照 memo 没有合入，原因是它会把陈旧数据和共享可变嵌套值放进调度热路径。等所有 writer 发布修订号、缓存有容量上限并完成 mixed-version 失效测试后再单独评估。

## 验证边界

- Docker 后端单测、协议/计费/图片/配额/限流 focused tests 和前端 lint、typecheck、641 项 feature/core 测试使用候选源码执行。
- Docker 运行态使用同一 PostgreSQL/Redis 数据库在 `BASELINE -> MODIFIED -> ROLLBACK -> RESTORED` 间切换，检查健康、就绪、前端、加密登录、API key、订阅、迁移和图片/流式协议行为。四态均通过；流式断连用量为基线 `10/0`、候选/恢复 `10/50`，图片门禁为基线 `200`、候选/恢复显式关闭 `400`，迁移从 305 前进到 306 并在回退镜像中保留。service account 详情接口在本 fixture 返回既有 `423` 凭据锁定边界，配额事实由数据库 JSONB 直接核对。
- 源码事务材料在 [`diagnostics/upstream-sync-20261022/`](../diagnostics/upstream-sync-20261022/) 中分别保存候选归档、补丁、命令记录、回退脚本、性能和运行态结果。回退副本恢复后再次应用补丁，原始主线工作区保持不变。
- race 并行运行同时复现了基线已有的 Gin `SetMode` 测试夹具竞态；候选以 `-parallel=1` 的同一 focused race 集合通过。
- 两个 1201 行文件（`openai_gateway_forward.go`、`payment_fulfillment_test.go`）在基线和候选中均未改动，继续作为已有布局债务记录，不扩大例外。

发布证据只在精确 commit、GitHub Actions 和 GHCR digest 检查完成后补充；本地 Docker 通过不等于远端发布完成。
