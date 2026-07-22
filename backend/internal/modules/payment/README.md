# Payment Module

支付领域的独立模型、金额规则、渠道注册和 provider 选择。

| 文件/目录 | 作用 |
| --- | --- |
| `types.go`, `currency.go`, `amount.go`, `fee.go` | 支付值对象与金额规则 |
| `crypto.go` | 支付配置敏感字段保护 |
| `registry.go`, `load_balancer.go` | provider 注册和实例选择 |
| `provider/` | Stripe、支付宝、微信、Airwallex、易支付适配器 |
| `wire.go` | 模块 provider 集合 |

订单生命周期和订阅履约仍由 application 编排；provider 不直接修改业务订单。
