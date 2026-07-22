# Commands

`cmd` 只包含可执行程序入口。业务逻辑必须下沉到 `internal`，入口负责读取参数、构造依赖、启动和优雅退出。

| 目录 | 入口用途 |
| --- | --- |
| `server/` | 主 HTTP 服务和后台任务进程 |
| `jwtgen/` | 生成维护所需的 JWT |
| `cleanup-ingress-reject-logs/` | 清理入口拒绝日志的运维命令 |

新增命令应建立独立子目录，不在 `cmd` 根目录放共享实现。
