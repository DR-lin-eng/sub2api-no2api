# OpenAI WebSocket V2

OpenAI Responses WebSocket v2 的协议状态机和直通 relay。

| 文件/前缀 | 作用 |
| --- | --- |
| `connection*` | 客户端连接生命周期 |
| `passthrough*` | 上下游帧转发与终止语义 |
| `state*` | 会话状态和响应关联 |

本包只处理 WS 协议细节；账号选择、计费和重试仍由上层 application service 编排。
