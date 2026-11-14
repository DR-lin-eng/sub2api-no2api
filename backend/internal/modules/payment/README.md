# Payment Module

支付领域的独立模型、金额规则、渠道注册和 provider 选择。

| 文件/目录 | 作用 |
| --- | --- |
| `types.go`, `currency.go`, `amount.go`, `fee.go` | 支付值对象与金额规则 |
| `recharge_bonus.go` | 默认关闭的充值优惠、最多 20 档阶梯校验、币种精度和实付/赠送报价 |
| `crypto.go` | 支付配置敏感字段保护 |
| `registry.go`, `load_balancer.go` | provider 注册和实例选择 |
| `budget.go` | 商户报价预留、UTC 日占用与硬性单笔/日限额规则 |
| `provider/` | Stripe、支付宝、微信、Airwallex、易支付适配器 |
| `wire.go` | 模块 provider 集合 |

订单生命周期和订阅履约仍由 application 编排；provider 不直接修改业务订单。

选路按限额筛选，配置或占用读取失败时拒绝继续；application 在创建订单事务中锁商户并重新准入。日占用包含当天已收款订单与跨日仍可恢复的未收款报价，退款不会释放当天已收款占用。恢复窗口常量由本模块统一定义，详见 [支付说明](../../../../docs/PAYMENT_CN.md)。
