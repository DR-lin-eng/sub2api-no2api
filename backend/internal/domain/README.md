# Domain

领域层保存不依赖 HTTP、数据库或运行配置的值对象和纯规则。

| 文件/目录 | 作用 |
| --- | --- |
| `announcement.go` | 公告领域规则 |
| `constants.go` | 稳定领域常量 |
| `models_list_config.go` | 模型列表配置值 |
| `openai_messages_dispatch.go` | 消息分发领域枚举 |
| `reasoning_effort.go` | reasoning effort 规范化 |
| `model/` | 共享领域模型 |

领域代码不得导入 Gin、Ent、Redis 或 platform/config。
