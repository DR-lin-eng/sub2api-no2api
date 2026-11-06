# 同步验证（2026-11-07）

基线为 `e0ecc7c85ce7c2f8e561e8da12f6cdcaae7c489c`，冻结上游至
`0eee80ba87aa4f3aff7282bf1695aa6dc0471c36`。范围与关闭/专项状态见 [台账](UPSTREAM_PRS.json)。

## Docker 与运行验证

固定 Go 1.26.10、Node 24、pnpm 11.17.0，Docker Linux arm64。Go 验证使用 2 CPU、4 GiB、
GOMAXPROCS=2、GOMEMLIMIT=1400MiB、GOGC=50、CGO_ENABLED=0、GOFLAGS=-p=1。

| 检查 | 结果和边界 |
| --- | --- |
| 全包单元分段验证 | 首轮全包的其他包通过，service 的适配差异修正后完整 service 集通过（229.924 秒）；最终 handler、cmd/server、apicompat、httputil 和 config 再验通过。精确提交的完整单命令结果由 CI 提供。 |
| 官方 MCP Go SDK OAuth | 独立 tag 的互操作用例通过。 |
| 全包 integration | 一次性 PostgreSQL/Redis 全包通过，包含邮箱探针、批量禁用、管理员保护、MCP 撤销/并发事务及既有计费/缓存测试。首次读取公网 TLS 检测站失败，最终完整重跑通过。 |
| 前端 ESLint / typecheck | 通过。 |
| 本轮相关前端 | 5 文件、74 用例通过：名单输入/编辑器、主设置页、路由语言覆盖及鉴权词表。 |
| Makefile 必需前端集 | 31 文件、280 用例通过。 |
| 扩展前端回归 | 53 文件、377 用例通过；另外两项失败用同一 Docker 镜像的冻结主线源码复现，详见下文。 |
| 正式 Docker 构建 | 基线和候选构建通过；本机默认 2 GiB Node heap 首次 OOM，候选用 4 GiB build heap 通过，并把此设置限定在正式 Dockerfile 的前端编译步骤。 |
| 同库升级与回退 | BASELINE → MODIFIED → ROLLBACK → RESTORED 四阶段、30 个 HTTP 场景通过，见 [result.json](runtime/result.json)。 |
| 实际浏览器组件 | 中文 1280×800；中文确认框和英文编辑框 390×844，无页面横向溢出，批量导入提示与保存后确认符合预期。零控制台错误；见 [UI.json](UI.json)。 |
| JSON 闭合条件预检 fuzz | 10 秒、185,493 次执行通过，原 `encoding/json.Valid` 为独立判定基准；不是对整个协议状态空间的证明。 |

编译后的最终流片段优化额外通过 apicompat 全包、解压边界和定向回归。解压测试覆盖超过 64 MiB 的输出及
恰好 64 MiB 输出后的损坏 gzip trailer；后者不能因读取到长度上限而跳过校验和。Wire 由源图重新生成。Gemini 旧字符串算法仅在测试 oracle 中保留；原有思考语义测试直接覆盖新的 builder owner。

这些验证只涉及本机 Docker 与假上游，没有部署设备或生产节点，没有禁用真实用户或向真实收件人发送通知。

## 升级与结算

四阶段复用同一 PostgreSQL/Redis，迁移数始终为 **313**。已有用户身份/密码摘要、API Key、订阅和历史用量
摘要保持一致；每个成功请求恰好一条用量，`余额 + 累计 actual_cost = 初始余额` 每阶段成立。
图片均实际返回两张并结算 $0.20；Responses JSON/SSE 均记录 1000/100 token、$0.004。

新节点验证空名单默认、Gmail 别名拒绝、历史用户不自动禁用、管理员和对照用户保护、确认批次及重复提交。
回退前通过 API 清空名单；旧节点仍能登录和完成既有请求。测试夹具的首次恢复阶段重复使用邮箱导致唯一约束
冲突，改为每阶段独立地址后完整四阶段通过；这次失败没有修改产品数据库约束。

## 性能

相同 benchmark 源文件被分别复制到冻结主线与最终候选的容器内。Docker 禁网、2 CPU/4 GiB、benchmark
固定 1 CPU，200ms × 3，中位数；保存完整样本，不把 B/op 当作峰值 RSS。

[最终结果](PERFORMANCE.json) 与 [初次候选](PERFORMANCE_BEFORE_COMPLETION_GATE.json) 保留优化前后的证据。
初次测量显示 Read 未闭合参数的重复 `json.Valid` 仍然消耗约 4 秒/512 KiB 分片夹具，因此补充必要条件预检：
对象、数组和字符串须可能有闭合符才进入原校验器，标量继续保持原行为。括号位于字符串内部时仍由校验器拒绝，
不会仅凭最后一个字符接受 JSON。

| 固定夹具 | 主线中位数 → 最终候选 | 累计分配 B/op |
| --- | --- | --- |
| 8 MiB，clone + 两次不变刷新 | 33.460 → 1.879 ms | 232 → 240 |
| 8 MiB，首次解析 | 11.346 → 12.063 ms | 216 → 408 |
| 8 MiB，cache-control 扫描 | 11.591 → 11.349 ms | 8,397,059 → 110 |
| 1 MiB，无需修改的 signed thinking | 3.789 → 1.355 ms | 1,050,956 → 8 |
| 512 KiB，64 字节 Read 未闭合分片累计 | 4,017.388 → 1.127 ms | 2,179,899,816 → 2,604,208 |


请求重试夹具从已经解析的同一 body 出发，比较 clone 与两次未变化刷新。它不包含首次解析、网络或上游推理。
首次解析因独立字符串和指纹增加 192 B/op；4 KiB/1 MiB/8 MiB 的 CPU 分别增加约 10.6%/8.6%/6.3%。后续复用收益和这项成本均列在原始结果中，不把 helper 的
变化外推为整站吞吐。基准包含结果 sink 的固定开销；未修改的对照 helper 也存在运行时抖动。Read 夹具只测量未闭合参数的分片累计，不包含最终工具执行；复杂无效输入和反复出现
候选闭合符的情况仍可能触发原完整校验。

## 既有门禁与 CI

`make check-docs` 仍报告主线索引指向缺失的 PR169 文档；布局门禁仍仅报告
`payment_fulfillment_test.go` 1201 行及 `pricing_service.go` 1218 行。两文件均未修改，也没有扩大 allowlist。
本轮新增文档链接与 diff 空白另行核对。

扩展前端的 `settingsGatewayResilienceModularization` 模板摘要和 `RegisterViewCredentialStorage` 旧夹具
在基线也同样失败，未混入修复。关键前端集和本轮新增用例通过，基线重放日志保存在 records。

PR 和合并后的精确 SHA CI、镜像状态单独记录；本地分段结果不替代远端 CI，不自动等同于生产发布或真实负载验证。
