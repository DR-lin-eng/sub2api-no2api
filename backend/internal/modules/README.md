# Modules

`modules` 放置边界清晰、可独立测试和演进的垂直领域能力。

| 模块 | 作用 |
| --- | --- |
| [activitycenter](activitycenter/README.md) | 活动配置、可见性、抽奖、签到、兑换膨胀和参与记录领域规则 |
| [chat](chat/README.md) | 在线客服会话、消息、未读状态、撤回事件、保留清理和实时广播 |
| [egress](egress/README.md) | 账号 IPv6 出口池、绑定、探测和 HE 隧道管理 |
| [qualityrender](qualityrender/README.md) | 内嵌本地 HTML 渲染与 HTML/SVG 代码匹配，无额外服务配置 |
| [payment](payment/README.md) | 支付金额、币种、渠道注册与提供商适配 |
| [securityaudit](securityaudit/README.md) | Prompt 审计、同步防护、队列和审计策略 |

活动类型与状态的纯白名单判断集中在 `activitycenter/campaign_state.go`。

模块通过公开接口与 application/transport 连接。禁止模块通过读取其他模块的内部状态形成隐式耦合。

用户流程和全部功能入口见 [功能索引](../../../docs/FEATURES.md)。新增模块时同步新增目录 README，并在本页与功能索引登记。
