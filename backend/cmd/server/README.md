# Server Command

主服务进程入口和 Wire 依赖装配。

| 文件 | 作用 |
| --- | --- |
| `main.go` | 参数、日志、启动和优雅退出 |
| `wire.go` | Wire provider 声明 |
| `wire_gen.go` | Wire 生成结果，不手工编辑 |
| `VERSION` | 默认版本 |

修改 provider 后运行 `go generate ./cmd/server`，并检查 `wire_gen.go` 只包含预期路径变化。
