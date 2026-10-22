# 上游主线同步审查（2026-10-13）

本轮以本项目 main `8773001a188f0b1fed3dff5d23b1700c70e3cf5e` 为基线，延续上次已审查边界 `ab80abc6d1f929a6864c8804fd077b509d21fda2`，冻结上游至 `4d9ac3e482106d2da3a39fe3e546d8d6801f1cc1`。范围为 7 个新合并 PR 和上游 VERSION 提交。执行使用独立可写副本，原 e3d4 工作区保持原始字节；本项目版本线不被上游 VERSION 覆盖。

## 逐 PR 结论

| PR | 决定 | 当前 owner 与兼容边界 |
| --- | --- | --- |
| [#7828](https://github.com/Wei-Shaw/sub2api/pull/7828) | 适配并关闭冻结差异 | 账号规则新增可选 `scope`，覆盖限流策略、运行态阻断、Bedrock、重试耗尽、count_tokens 和 admin-accounts 创建/编辑。未设置范围保持既有行为；凭据失效仍保留账号保护。 |
| [#8046](https://github.com/Wei-Shaw/sub2api/pull/8046) | 适配并关闭冻结差异 | Antigravity 图片模型不继承或写入 Gemini 文本模型族冷却；模型自身的冷却仍生效。模型类型只归一化一次。 |
| [#8038](https://github.com/Wei-Shaw/sub2api/pull/8038) | 适配并重构，关闭冻结差异 | 复用 Gemini Messages converter，在单次遍历中提取工具结果的文字和 base64 图片，functionResponse 后追加图片。不新增远程图片下载。 |
| [#7991](https://github.com/Wei-Shaw/sub2api/pull/7991) | 适配并关闭冻结差异 | 精确解析 Kimi 官方 HTTPS 主机及 coding 路径，支持 `api.kimi.ai`；默认白名单和示例配置同步。保留现有账号代理/IPv6 额度出口。 |
| [#7810](https://github.com/Wei-Shaw/sub2api/pull/7810) | 适配并关闭冻结差异 | 仅在 auto、已确认支持 Responses 的 OpenAI API Key 账号上，完整且明确的 input-string 拒绝可回退一次原生 Chat。未提交响应、未取消才可回退；跨尝试使用原始开始时间。 |
| [#7957](https://github.com/Wei-Shaw/sub2api/pull/7957) | 专项暂缓，不标记关闭 | 本项目当前 native/reverse converter 缺少上游 Claude 5.5 的签名历史往返、adaptive/disabled thinking 与采样/工具策略前置能力。只添加目录和价格会公开一个协议不完整的模型。需在当前 owner 补齐流式/非流式签名、努力等级、强制工具、采样和 100k 计费边界矩阵后再注册 Haiku 5.5。 |
| [#8006](https://github.com/Wei-Shaw/sub2api/pull/8006) | 专项暂缓，不标记关闭 | cachedContents 引入新资源 API、按 token-hour 的存储计费、账号绑定、mutation lease 和恢复 worker；原迁移 243–245 与本项目渠道/OpenCode/代理健康迁移冲突。需建立原生缓存模块、接入当前可靠结算并使用新迁移编号，验证旧/新 writer、租约过期、取消、崩溃恢复和有界查询后显式开启。 |

机器台账 [UPSTREAM_PRS.json](../diagnostics/upstream-sync-20261013/UPSTREAM_PRS.json) 保存 merge SHA、原始 diff SHA-256、关闭结果及重开条件。此前台账不修改；包括 #7618、#7738、#8033 的独立暂缓，不因 tracking merge 变成已实现。上游广告和赞助内容不进入本项目三语 README。

## 平滑升级与性能

本轮没有 Ent/Wire 修改、数据库迁移或依赖升级。`scope` 是凭据 JSON 的可选字段，自动范围不序列化该字段。旧规则保持原有的“模型已知且非 401 使用模型冷却”行为；显式模型范围不会扩大为账号阻断，模型未知时仍按账号保护，OAuth/影子账号及明确失效的 OpenAI 凭据保留认证处置。

滚动升级时先更新全部后端节点和管理端，再设置新的显式 scope；旧版本忽略这个字段，回退前应恢复自动范围，避免不同版本执行不同范围。账号配置中未改动的凭据和字段继续由现有更新 owner 保留。Gemini 文本工具结果使用单次遍历，文本路径不分配图片数组；Antigravity 模型类型只归一化一次。Responses 拒绝分析仅在错误分支执行，错误体仍有读取上限且检查完整性，成功请求不运行正则；没有增加成功请求的 SQL、Redis 往返、后台任务或 goroutine。

性能与最终验证结果以 [VERIFICATION.txt](../diagnostics/upstream-sync-20261013/VERIFICATION.txt) 的原始命令、逐字输出、退出码和哈希为准。性能比较使用同一 Docker Go 1.26.9、linux/arm64、GOMAXPROCS=2、受限内存，按基线和候选顺序执行；不把微基准描述成全网关吞吐提升。

Docker 同条件三轮中位数：单块工具文字 `581.1 → 575.8ns`，1 MiB 工具文字 `615.39 → 583.513µs`，两者分配数和分配字节不变；Antigravity 文本限额检查 `123.6 → 118.7ns`，仍为 `48B / 2 allocs`。旧规则错误匹配 `113.3 → 131.1ns`，仍为 3 次分配，因可选 scope 的规则值增加 `160 → 192B`；这发生在错误匹配路径，保留这项实际开销，不宣称全路径零成本。

对 #8006 的额外静态审查发现：`GeminiExplicitCacheExclusions` 即使缓存已绑定账号也构造 O(N) 候选排除 map；专项实现应使用现有账号快照与分组准入直接验证绑定账号。恢复查询已有 batch 上限，但会增加周期性数据库 worker，需补多实例负载验证。`doUpstream` 使用上游旧的通用 HTTP 入口，接入时必须改用本项目完整账号代理/IPv6/TLS 出口并验证失败关闭。这些代码本轮没有进入运行路径。

## 验证与回退角色

可重建源码归档为 [MODIFIED_FILE.tar.gz](../diagnostics/upstream-sync-20261013/MODIFIED_FILE.tar.gz)，补丁为 [DIFF_FILE.patch](../diagnostics/upstream-sync-20261013/DIFF_FILE.patch)，执行记录为 [VERIFICATION.txt](../diagnostics/upstream-sync-20261013/VERIFICATION.txt)，回退脚本为 [ROLLBACK.sh](../diagnostics/upstream-sync-20261013/ROLLBACK.sh)。回退脚本仅用于独立源码副本，从 sibling baseline 归档恢复原字节并删除新增路径，不操作原工作区。

同一 Docker fixture 的基线表现为：国际 Kimi provider 为空、图片继承 Gemini 文本族限制、工具结果 0 张图片、显式 account 规则仍写模型冷却。候选表现为：provider=kimi、图片不继承文本族限制、保留 1 张工具结果图片、显式 account 规则写账号暂停。独立回退与重施的命令、行为及全部原文件哈希验证保存在同一证据目录。

后端维护门禁中的 `payment_fulfillment_test.go` 1201 行、`pricing_service.go` 1238 行在精确基线和候选一致；本轮不修改这两个既有问题或扩大 allowlist。新增 Gemini 图片测试独立成文件，账号规则解析收回 feature 内的纯规则 owner，编辑表单保持维护目标以内。Docker 生产镜像、同 PostgreSQL/Redis 数据的升级/回退、完整单元/集成、聚焦 race、前端必需门禁及远端 CI 分别记录，不把本地结果代替发布结果。

最终 Docker Go `make test-unit`、`make test-integration`、聚焦 `-race` 和 `golangci-lint` 均通过，lint 为 0 issues。初次并行运行中的编译进程被杀和限时用例超时保留为资源错误记录；受限内存、顺序运行后通过，不修改这些既有超时断言。

前端最终 `make test-frontend` 的 lint/typecheck 和 247 个必需用例通过，创建/编辑/范围兼容的 80 个聚焦用例通过。初次 3 GiB 容器自动将 Node heap 限到约 1.5 GiB 而耗尽；仅调整测试容器为 4 GiB、Node heap 为 3 GiB 后通过，不更改生产配置。

最终完整生产镜像构建通过。同一 PostgreSQL/Redis 上的 baseline → upgraded → rollback → restored 四阶段全部通过：ready/前端、加密登录、既有 API Key、旧订单和持久数据快照保持一致，迁移数量四阶段相同；译文与不带 scope 的旧规则兼容，新 scope 在镜像替换及旧 writer 更新其他字段后仍保留。该矩阵验证字段持久兼容；显式范围的跨旧/新节点执行差异仍按上述升级启用顺序约束。
