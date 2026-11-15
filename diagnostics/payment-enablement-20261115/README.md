# 充值提示“支付系统已关闭”排查（2026-11-15）

基于 `39ea47746` 排查。线上仅使用用户授权的只读管理员 API；本地测试使用 Docker 内部网络、临时 PostgreSQL/Redis 和模拟易支付服务，没有发起真实支付。

## 线上证据

读取 `https://www.fhl.mom` 的 `/api/v1/admin/settings`、`/api/v1/admin/payment/config`、`/api/v1/admin/payment/providers`：

- 初次检查 `payment_enabled=false`，2 号易支付实例已启用且支持支付宝。这会直接触发全局关闭错误。
- 用户打开总开关后，再次读取确认 `payment_enabled=true`、`payment_balance_disabled=false`。
- 两次读取的 `payment_visible_method_alipay_enabled`、`payment_visible_method_wxpay_enabled` 均为 `false`，对应来源字符串为空。

凭据、原始设置响应、商户配置均未写入本目录。

## 回归原因及处理

提交 `b83123d9d` 给 `PaymentService.CreateOrder` 增加旧可见方式开关校验，并将其拒绝原因复用为 `PAYMENT_DISABLED`。这两个字段默认关闭，普通系统设置保存也会持久化旧值，但当前前端既不展示也不提交它们。充值页的可用方式则根据已启用服务商和 `supported_types` 生成，因而出现“可以选择支付宝，下单却提示系统已关闭”的矛盾。

本次移除旧开关的下单门禁、默认初始化、设置写入和无调用方的旧过滤函数，使用充值页与负载均衡现有的服务商状态。保留旧 API 字段的格式及读取兼容，历史值不再参与支付决策；无需数据库迁移。总开关、余额充值开关、服务商启停、支付方式选择及商户事务预算继续生效。

管理页面已有 **设置 → 支付设置 → 服务商管理** 中的实例“启用”开关及支付宝、微信方式按钮，能够直接控制收款。本次补充中英文操作说明，不再增加重复开关。

## Docker 复现与验证

仓库根目录执行：

```sh
python3 diagnostics/payment-enablement-20261115/run.py --run '^TestPaymentCheckoutEnablementPostgres$'
python3 diagnostics/payment-enablement-20261115/run.py
```

脚本使用 Go 1.26.11、PostgreSQL 18.1 和 Redis 8.4，自动清理自己创建的容器与网络；只挂载当前仓库源代码和 Go 缓存。默认执行 `TestPayment.*Postgres` 集成测试。

修复前，新回归用例中充值页展示支付宝与微信支付，而 `alipay`、`wxpay`、`alipay_direct`、`wxpay_direct` 的真实 HTTP handler 均返回 HTTP 403：

```json
{"code":403,"message":"payment method is disabled","reason":"PAYMENT_DISABLED"}
```

修复后验证：

- 以上四种请求均成功创建待支付订单，并调用模拟网关；尚未付款的订单保持 `paid_at` 为空。
- 总开关关闭仍返回 `PAYMENT_DISABLED`，余额充值关闭仍返回 `BALANCE_PAYMENT_DISABLED`。
- 在服务商取消微信方式后，充值页隐藏微信，直接请求也被拒绝，支付宝仍可下单。
- 关闭服务商后，对应方式从充值页消失，直接请求被拒绝。
- 支付 PostgreSQL 集成测试覆盖退款、返利追回、恢复窗口、并发预算、回调竞争及 Stripe 商户校验。
- 19 组支付 PostgreSQL 集成测试全部通过，包含子测试共 82 项通过记录。
- Docker 后端支付相关单元测试与设置 API 契约通过；前端 lint、类型检查及 27 个文件中的 234 项支付/设置/词表测试全部通过。
- `make check-docs`、`make -C backend check-layout` 和 `git diff --check` 通过。命令摘要及压缩日志见 [验证记录](records/results.json)。

线上未执行写入或真实下单；代码修复需要部署后才会作用于网站。
