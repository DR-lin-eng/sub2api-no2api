# Setup Flow

| 文件 | 作用 |
| --- | --- |
| `cli.go` | 命令行初始化交互 |
| `handler.go` | setup HTTP 入口 |
| `setup.go` | 初始化状态与核心流程 |
| `setup_test.go` | 初始化回归测试 |

本包仅用于未完成初始化的部署，成功后不得进入正常请求热路径。
