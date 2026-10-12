# API Key 删除与计费竞争条件审查

日期：2026-10-06（Asia/Shanghai）。审查源码：`9600705ea6a6ce9fc02512582e86ef57f9242f63`。

## 结论

**当前仓库已修复图片中的“请求中途删除 API Key，生成继续但费用不扣”漏洞，本次 Docker 测试未复现免扣费。修复前的结算逻辑确实存在该问题。**

保护来自 2026-10-05 的提交 `afc6432dceab4401f8c625dc158534026662d9ba`，对应已移植的上游 #7816，见 [同步记录](../../docs/UPSTREAM_SYNC_20261005.md)。本次没有修改生产计费逻辑；新增真实数据库回归测试与可重复执行的验证材料。

本结论针对上述源码和本地 Docker 二进制。没有访问线上实例，也没有确认线上镜像是否包含该提交；线上运行修复前的实现时仍有风险。图片中的他站用户、费用数字和版本说法没有作为本仓库运行事实。

## BILL-001：删除在途 Key 导致整笔结算回滚（高风险，当前已修复）

影响：旧实现下，普通用户可以让已生成、上游已有成本的请求不消耗自己的余额或订阅额度。

触发条件是请求已经通过鉴权并开始生成，Key 在最终结算前被删除，而且原 Key 的额度或费用窗口需要递增。用户创建 Key 时可设置这些限制。删除后继续生成本身是正常的在途请求行为；漏洞出在账务事务如何处理已消失的 Key。

旧逻辑先更新余额/订阅，再更新 Key 额度。Key 被软删除后，额度更新返回 `ErrAPIKeyNotFound`，旧代码将它当作整笔事务失败，从而回滚用户扣费；持久队列逐笔重试也会遇到同一错误。网关在同步结算报错时会把用量日志的 `ActualCost` 改为 0，因此可能呈现图片中的“token 和基础费用都有，实际费用为 0”。

相关源码（行号对应本次审查版本）：

- [usage_billing_repo.go](../../backend/internal/infrastructure/repository/usage_billing_repo.go)：174 行起为完整账务效果；190–200 行仅跳过已删除 Key 自身计数的 `ErrAPIKeyNotFound`，其余数据库错误仍使事务失败。
- [api_key_repo.go](../../backend/internal/infrastructure/repository/api_key_repo.go)：382 行起使用 tombstone 与软删除，保留旧 ID 对应的账务关联，释放原密钥字符串。
- [gateway_usage_billing.go](../../backend/internal/application/service/gateway_usage_billing.go)：842 行起在结算失败时记录 0 实际费用；524 行起将结算 context 与客户端取消分离。
- [openai_gateway_usage.go](../../backend/internal/application/service/openai_gateway_usage.go)：459 行起具有相同的失败记录语义。

当前代码继续按请求开始时的用户、订阅和旧 Key ID 结算。用相同密钥字符串创建新 Key 会获得不同 ID，既不会免除旧请求费用，也不会把旧请求计数写到新 Key。

### 历史逻辑回放

以当前其余代码为基准，仅用 Go `-overlay` 替换结算 repository 为 `afc6432dc^` 的原文件，旧文件来自提交 `f64e146d258f037ea88208b66011fbe421036c8d`。没有把旧实现写回当前源码，也没有运行到任何线上服务。

四个回归子场景全部按预期失败：同步余额/订阅报 `API_KEY_NOT_FOUND`；队列两笔各 $1.25 的结算后，余额仍为 **$100（应为 $97.50）**，订阅用量仍为 **$0（应为 $2.50）**。这证明测试能检测旧漏洞。见 [历史回放日志](docker-pre-fix-replay.log)、[原文件](pre-fix-usage-billing.go)、[overlay](pre-fix-overlay.json)。

## 当前代码的 Docker 验证

环境：Linux arm64，Go 1.26.6，PostgreSQL 18.1，Redis 8.4。Testcontainers 设置 `CI=true`，Docker 不可用时会失败，不会静默跳过。HTTP 服务从上述源码在 Docker 中编译，见 [二进制版本](runtime-version.log)。

| 验证范围 | 结果 | 证据 |
| --- | --- | --- |
| 删除 Key、账务错误与幂等相关单元测试 | PASS | [日志](docker-unit.log) |
| 真实 PostgreSQL/Redis，删除并重建同一密钥；余额、订阅、账号额度、用户平台额度、重复提交及队列故障恢复 | PASS | [集成日志](docker-integration.log)、[新增回归测试](../../backend/internal/infrastructure/repository/usage_billing_key_deletion_integration_test.go) |
| 最终回归测试与 Redis 丢失恢复，Go `-race` | PASS | [日志](docker-race.log) |
| 同步结算的完整 HTTP 请求，12 个场景 | 12/12 PASS | [日志](runtime-direct.log)、[逐场景账务变化](runtime-direct.json) |
| 持久异步结算的完整 HTTP 请求，12 个场景 | 12/12 PASS | [日志](runtime-queued.log)、[逐场景账务变化](runtime-queued.json) |

HTTP 矩阵为 3 个入口（`/v1/responses`、`/v1/chat/completions`、`/v1/messages`）× 2 种账务模式（余额、订阅）× 2 种响应（JSON、SSE）× 2 种结算方式，共 24 个场景。

每个场景都先让本地上游阻塞最终完成；SSE 场景确认客户端已收到事件；通过真实 JWT 用户接口删除 Key，确认旧 Key 新请求返回 401；再用同一字符串创建新 Key；最后放行上游并核对账务。合成 JWT 只用于本任务隔离环境，仍经过实际 JWT 校验、用户状态、token version 和删除所有权检查。模型上游及价格文件均在本地容器中，未消耗真实模型额度。

每个请求的固定用量是 1000 输入 token、500 输出 token。测试价格文件使 OpenAI 请求每笔 $0.00045，Anthropic 请求每笔 $0.0105；这些是复现用固定价格，不是当前官方费率声明。24 笔均有正实际费用，余额/订阅扣费与日志一致，账号额度和余额用户的平台额度正常累计，每笔只有一个持久幂等记录。删除的 Key 与新建 Key 的自身计数为 0，旧请求费用仍由原用户承担。

异步模式另核对了 12 个 [Redis 完成标记](runtime-queue-completed.log)、0 个 [待处理 overlay](runtime-queue-pending.log)，以及 0 个待处理账单/死信。

## INFO-002：异步批量路径的性能边界

[usage_billing_queue_consumer.go](../../backend/internal/infrastructure/repository/usage_billing_queue_consumer.go) 187 行起在批量效果更新失败时回滚整个批次，随后调用逐笔结算。批量效果函数仍会因已删除 Key 的计数更新失败而触发该回退；逐笔路径使用已修复的规则，所以本次确认最终扣费成功、没有死信。

这意味着批量遇到大量已删除 Key 时会退化为逐笔处理，是性能优化空间；本次没有将它判为免扣费漏洞，也没有修改该路径。

## 日志排查与范围

单看“0 实际费用”不能确认漏洞。免费倍率、未配置价格、失败请求等也可能产生 0 费用，应该结合基础费用、倍率、结算幂等记录、待处理任务和死信判断。[audit.sql](audit.sql) 是只读排查查询，筛选近 7 天有 token、基础费用为正且倍率为正的 0 实际费用记录，并联查在线/归档幂等、待处理及死信；本次测试数据库返回 [0 行](runtime-audit-sql.log)。没有执行线上历史账目审计。

本次实际 HTTP 验证使用 OpenAI 与 Anthropic API Key 上游账号。Gemini、Grok、OAuth、WebSocket、图片与批量任务未逐一做 HTTP 复现；它们共用的结算 repository 已纳入数据库测试。本次结论限于图片所示 Key 删除竞争条件。

复现入口是 [VERIFY.sh](VERIFY.sh)，从仓库根目录执行 `./diagnostics/billing-key-deletion-20261006/VERIFY.sh`。它创建并清理专用 Compose 项目，只使用合成数据；编译产物 `server` 已忽略。默认使用本机模块缓存和 Docker socket，可通过 `AUDIT_MODULE_CACHE`、`AUDIT_DOCKER_SOCKET` 覆盖。执行日志、固定价格、上游 fixture 和源码指纹保留在本目录。
