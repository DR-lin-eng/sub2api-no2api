# Application

应用层负责用例编排、业务策略和基础设施端口定义，不感知 HTTP 路由和具体数据库客户端。

| 目录 | 作用 |
| --- | --- |
| `service/` | 现有应用服务、网关编排、计费和调度端口 |

新垂直模块优先进入 `internal/modules`；只有跨多个模块的应用编排才继续放入 `service`。

Claude 5.5 参数策略由 `service/gateway_claude55_policy.go` 负责。模型目录刷新由 `service/upstream_model_catalog.go` 编排并复用现有账号传输，纯描述规则见 [modelcatalog](../modules/modelcatalog/README.md)。
