# Modules

`modules` 放置边界清晰、可独立测试和演进的垂直领域能力。

| 模块 | 作用 |
| --- | --- |
| `activitycenter/` | 活动配置、用户可见性、抽奖资格、事务抽奖与参与记录领域规则 |
| `chat/` | 在线客服会话、消息、未读状态、撤回事件、保留清理和实时广播 |
| `payment/` | 支付金额、币种、渠道注册与提供商适配 |
| `securityaudit/` | Prompt 审计、同步防护、队列和审计策略 |

活动类型与状态的纯白名单判断集中在 `activitycenter/campaign_state.go`。

模块通过公开接口与 application/transport 连接。禁止模块通过读取其他模块的内部状态形成隐式耦合。
