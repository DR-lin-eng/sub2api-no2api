# Security Audit Module

本模块负责 Prompt 审计、同步防护、审计队列、策略探测和审计结果持久化接入。

## 文件索引

| 前缀 | 职责 |
| --- | --- |
| `prompt_config*`, `prompt_types*` | 配置和公共模型 |
| `prompt_runtime*`, `prompt_worker*`, `prompt_enqueue*` | 运行时、队列和 worker |
| `prompt_client*`, `prompt_probe*`, `prompt_qwen3guard*` | 审计客户端和探测 |
| `prompt_sync_guard*`, `prompt_outbound*` | 同步阻断和出站策略 |
| `prompt_handler*`, `prompt_middleware*` | 模块 HTTP 接入 |
| `prompt_payload_store*`, `prompt_repository*` | 载荷和审计记录端口 |
| `wire.go` | 模块依赖装配 |

同步热路径必须保持有界等待；异步审计不可因队列拥塞无限创建 goroutine。
