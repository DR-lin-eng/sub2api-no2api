# Configuration

| 文件 | 作用 |
| --- | --- |
| `config.go` | 顶层配置、部署、日志和外部接入类型 |
| `config_runtime_types.go` | 令牌刷新、计费、安全、并发和服务端运行配置 |
| `config_gateway_types.go` | 网关、OpenAI WebSocket、调度和用量队列配置 |
| `config_storage_ops_types.go` | 数据库、Redis、运维、缓存和清理配置 |
| `load.go` | Viper 加载、环境绑定和运行模式归一化 |
| `defaults.go` | 全部配置默认值 |
| `validate.go` | 主配置校验 |
| `helpers.go` | URL、JWT 和通用归一化辅助函数 |
| `validate_dingtalk.go` | 钉钉配置专项校验 |
| `wire.go` | 配置 provider |
| `*_test.go` | 环境可达性、图片存储和配置回归 |

新增配置必须定义默认值、环境变量映射和校验；面向前端的公开设置还需同步 DTO 与 HTML 注入结构。
