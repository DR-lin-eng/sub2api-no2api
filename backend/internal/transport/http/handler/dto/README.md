# HTTP DTO

本目录定义传输层请求/响应结构以及 application model 到 API JSON 的映射。

| 文件/前缀 | 作用 |
| --- | --- |
| `types.go` | 通用 API DTO |
| `settings.go` | 公共与管理设置 DTO |
| `mappers.go` | 核心模型映射 |
| `credentials_redact.go` | 凭据输出脱敏 |
| `announcement.go`, `channel_monitor.go` | 对应资源 DTO |

DTO 不进入 application/repository 接口，避免 HTTP 字段变更扩散到业务层。
