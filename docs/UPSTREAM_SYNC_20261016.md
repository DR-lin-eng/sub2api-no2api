# 上游主线同步审查（2026-10-16）

本轮以本项目 main `ed58484ab78497352259aea855e713df20b6a115` 为基线，沿用上轮冻结边界 `1b2243a4f50621508e79aef4b498a2937a6238ee`，审查上游 main 至 `b8cfb7884084393e08f0e2c4027afc3755357b98`。范围为 8 个提交、2 个已合并 PR。实现留在现有 application、payment provider、HTTP handler 和 billing feature owner；不恢复上游旧目录，不修改 Ent/Wire、依赖、数据库结构或本项目版本。

## 逐 PR 决定

| PR | 决定 | 当前实现与适配 |
| --- | --- | --- |
| [#8122](https://github.com/Wei-Shaw/sub2api/pull/8122) | 适配并关闭冻结差异 | 创建与履约仅允许 balance/subscription；未知类型不充值，失败状态写入出错时仍返回错误。pending 退款保留已扣余额/订阅权益，状态与扣减审计同事务保存；成功不再扣减，失败以 CAS 认领后在同一事务归还并写审计。保留旧审计语义；新增显式旧版兼容标记与软删除订阅恢复。Alipay 保存真实退款请求号、查询终态并保留十分钟无记录宽限。Stripe 只读本订单的本地恢复快照，URL 不再携带或提供 client secret。 |
| [#8133](https://github.com/Wei-Shaw/sub2api/pull/8133) | 适配并关闭冻结差异 | 易支付查询同时传递 query/form 参数，兼容字符串/数字标量与 null 回退，显式失败码和订单回显冲突失败关闭。验签实例负责入账前确认，要求上游已支付、有限正金额一致和交易号一致；失败返回 502、保持待支付并留审计。确认放在本地校验之后，重复已完成回调不查询上游。补充普通主机/userinfo/RawPath 校验和签名字段分隔符检查。 |

[机器台账](../diagnostics/upstream-sync-20261016/UPSTREAM_PRS.json) 保存 merge SHA、原始 diff SHA-256、owner、覆盖子项与重开条件。[冻结输入](../diagnostics/upstream-sync-20261016/UPSTREAM_REVIEW_INPUT.json) 记录前轮台账哈希。#7802 的充值优惠、#7673 的匿名查询限流、#7888 的回调 PID/重复参数/sign_type/交易号保护、#7819 的 Stripe 卸载保护继续沿用现有实现；不重放对应冻结补丁。其它历史暂缓项不因本轮支付改动自动重开。

## 资金与性能审查

上游 pending 审计对 `deductionRollbackOK=false` 使用 omitempty，旧读取器却以 true 为缺省值，会造成旧节点在成功查询时再次扣款。本项目始终写出该布尔值；原始旧二进制的同输入探针验证成功查询不再扣减。旧版不能归还新版持有的失败退款，混合版本与回退须遵守下节的管理操作边界。

本项目订阅撤销使用软删除，不能直接对已删除订阅延长天数。扣减凭证增加 `subscriptionRevoked`；失败退款通过原有 `RestoreSubscription` 恢复撤销前到期时间，部分权益扣减仍由原有延长接口归还。资金、失败状态和审计在 PostgreSQL 中同事务提交；审计写入失败时三者回滚。并发查询只有一个 CAS 认领者归还资金。

易支付新增一次最长十秒的订单确认，仅发生于尚可转为 PAID 的验签回调；上游不可用时依赖回调重试或原有对账。已完成/已退款的重复回调、金额不符和未知订单不新增远端查询，原有查询对账不再重复确认。模型网关热路径不新增查询、worker、goroutine 或重试。原有 EasyPay HTTPS、loopback HTTP 开发例外、响应大小限制与禁止重定向保持；运行夹具使用应用网络命名空间的 loopback，不放宽生产策略。

查询微基准使用相同 Docker Go 1.26.9、linux/arm64、2 CPU/3 GiB、相同合法响应与 3000 次请求，保留三轮原始样本、分配量和恢复后样本于 [PERFORMANCE.json](../diagnostics/upstream-sync-20261016/PERFORMANCE.json)。这些数字包含本机 HTTP 往返，不代表全网关吞吐。重复回调的实际 HTTP 测试确认追加远端查询数为零。

## 平滑升级和回退

无数据库迁移，运行矩阵迁移数量为 `303 → 303 → 303 → 303`。同一 PostgreSQL/Redis 的 baseline → upgraded → rollback → restored 已验证健康/就绪、嵌入前端、加密登录、API Key、旧订单、订阅到期时间与持久快照一致；替换节点传入失效的首次安装参数也不会改写已有管理员。

退款管理必须与普通请求分开安排：

1. 滚动升级期间暂停管理员 `POST /api/v1/admin/payment/orders/:id/refund` 与 `/:id/refund/query`，等待已发起的 REFUNDING 操作结束，再替换全部节点；完成后恢复退款管理。网关请求、充值回调和查询对账继续使用原有入口。
2. 二进制回退前先暂停新退款，用新版将 pending 退款查询到终态；人工核实旧 Alipay 请求号等不支持查询的记录。确认下面查询无结果，再暂停查询操作并切回旧镜像。不能让旧节点处理新版仍持有扣减的失败退款。
3. 回退不删除审计或变更数据库。旧审计没有 `deductionHeld` 时继续执行原有归还/再扣路径；无法解析的审计返回错误，不猜测资金状态。旧 Alipay 交易号不被伪装成可查询请求号。

在目标数据库只读核对：

```sql
SELECT id, status
FROM payment_orders
WHERE status IN ('REFUNDING', 'REFUND_PENDING')
ORDER BY id;
```

Stripe 旧同浏览器订单仍使用既有恢复快照；新页面忽略 URL secret。无匹配、已过期、resume token 不符或浏览器禁止存储时，在 SDK 加载前显示原有缺参提示，需回到支付/恢复入口重新进入。

## 验证证据

Docker 全量后端 unit/integration、支付 race、真实 PostgreSQL 退款事务、前端 lint/typecheck/247 个关键用例、billing 的 154 个用例及生产镜像构建均有独立记录。PostgreSQL 新增用例覆盖成功/失败收敛、审计失败原子回滚、四个并发查询只归还一次，以及退款失败恢复已撤销订阅。四阶段实际 HTTP 场景中，未知类型为 `200 → 400 → 200 → 400`；未支付易支付回调为 `200/COMPLETED → 502/PENDING → 200/COMPLETED → 502/PENDING`；未知类型履约为 `COMPLETED → FAILED → COMPLETED → FAILED`。

原有 `payment_fulfillment_test.go` 为 1201 行，精确基线与候选的 `make check-layout` 均以相同错误退出 2；该文件字节保持，不扩大 allowlist。原始失败、夹具修正、命令、输入、stdout/stderr、退出状态与哈希见 [VERIFICATION.txt](../diagnostics/upstream-sync-20261016/VERIFICATION.txt)。

四个可重建角色为 [MODIFIED_FILE.tar.gz](../diagnostics/upstream-sync-20261016/MODIFIED_FILE.tar.gz)、[DIFF_FILE.patch](../diagnostics/upstream-sync-20261016/DIFF_FILE.patch)、上述验证账本和 [ROLLBACK.sh](../diagnostics/upstream-sync-20261016/ROLLBACK.sh)。源码回退接受独立副本，恢复 sibling baseline 中的原始字节、删除本轮新增源码，并验证全部基线哈希和补丁重施。源码回退不替代前述运行中退款结清要求。
