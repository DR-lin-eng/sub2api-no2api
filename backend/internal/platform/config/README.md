# Configuration

| 文件 | 作用 |
| --- | --- |
| `config.go` | 顶层配置、部署、日志和外部接入类型 |
| `config_runtime_types.go` | 令牌刷新、计费、安全、并发和服务端运行配置 |
| `config_gateway_types.go` | 网关、OpenAI WebSocket、调度和用量队列配置 |
| `config_storage_ops_types.go` | 数据库、Redis、运维、缓存和清理配置 |
| `load.go` | Viper 加载、环境绑定和运行模式归一化 |
| `source.go` | 显式配置文件与默认搜索路径的统一优先级 |
| `defaults.go` | 配置默认值注册顺序编排 |
| `defaults_runtime.go` | 运行模式、部署、服务端、日志、CORS 和安全默认值 |
| `defaults_billing.go` | 计费与可靠结算队列默认值 |
| `defaults_identity.go` | Turnstile 与第三方身份接入默认值 |
| `defaults_storage.go` | 数据库、Redis、批量图片和对象存储默认值 |
| `defaults_ops.go` | 运维、鉴权、定价、缓存、聚合和幂等默认值 |
| `defaults_gateway.go` | 网关传输、WebSocket、调度和用量队列默认值 |
| `defaults_maintenance.go` | 令牌刷新、Gemini 和订阅维护默认值 |
| `defaults_env.go` | 仅由环境注入的配置键可达性默认值 |
| `validate.go` | 主配置校验顺序编排 |
| `validate_runtime.go` | 部署、服务端、鉴权、日志和运行时校验 |
| `validate_identity.go` | LinuxDo、微信和 OIDC 身份接入校验 |
| `validate_storage.go` | 计费、存储、批处理、聚合和幂等校验 |
| `validate_gateway.go` | 网关传输、WebSocket、调度和用量队列校验 |
| `validate_ops.go` | 运维、并发和钉钉接入编排校验 |
| `helpers.go` | URL、JWT 和通用归一化辅助函数 |
| `validate_dingtalk.go` | 钉钉配置专项校验 |
| `wire.go` | 配置 provider |
| `*_test.go` | 环境可达性、图片存储和配置回归 |

新增配置必须定义默认值、环境变量映射和校验；面向前端的公开设置还需同步 DTO 与 HTML 注入结构。

`gateway.min_output_tokens` / `GATEWAY_MIN_OUTPUT_TOKENS` 控制兼容转换中的输出下限，默认 128、至少 16。滚动升级和回退先保留默认值，所有节点更新后再显式修改；原生 Responses/WS 不经过该转换下限。

`gateway.max_same_account_retries_gemini` / `GATEWAY_MAX_SAME_ACCOUNT_RETRIES_GEMINI` 是 Gemini 三条转发路径的同账号重试次数，不含首次请求，默认 4、有效范围 0–10；0 关闭瞬时错误重试，签名兼容降级独立计数。
