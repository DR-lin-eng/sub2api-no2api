# Modules

`modules` 放置边界清晰、可独立测试和演进的垂直领域能力。

| 模块 | 作用 |
| --- | --- |
| `payment/` | 支付金额、币种、渠道注册与提供商适配 |
| `securityaudit/` | Prompt 审计、同步防护、队列和审计策略 |

模块通过公开接口与 application/transport 连接。禁止模块通过读取其他模块的内部状态形成隐式耦合。
