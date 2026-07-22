# Backend

本目录包含 Sub2API 的 Go 后端、数据库迁移、嵌入资源和维护脚本。后端采用模块化单体结构：业务调用仍在同一进程内完成，但目录按依赖方向和职责划分，便于独立修改、测试和审查。

## 目录索引

| 目录 | 作用 |
| --- | --- |
| `cmd/` | 可执行程序入口，只负责参数解析和依赖装配 |
| `internal/application/` | 应用用例、业务编排和端口接口 |
| `internal/domain/` | 不依赖传输和存储的领域值、枚举和规则 |
| `internal/infrastructure/` | 数据库、Redis、上游访问等端口实现 |
| `internal/transport/` | HTTP 路由、handler、中间件和前端静态资源 |
| `internal/modules/` | 可独立演进的垂直领域模块 |
| `internal/platform/` | 配置、限流、凭据保护等运行平台能力 |
| `internal/shared/` | 无业务流程的复用基础包和协议适配工具 |
| `internal/bootstrap/` | 首次启动、初始化和升级引导 |
| `ent/` | Ent 生成代码；仅修改 `ent/schema/` 后重新生成 |
| `migrations/` | 按版本执行的数据库迁移 |
| `resources/` | 运行时静态数据 |
| `scripts/` | 构建、检查和运维脚本 |

## 依赖方向

推荐依赖方向为 `transport -> application -> domain`。`infrastructure` 实现 application 定义的端口，并只在 Wire/启动装配处与具体实现绑定。`shared` 不得依赖 application、transport 或 infrastructure。

现有 `application/service` 仍是较大的兼容包。新增独立功能优先放入 `modules/<domain>`，再通过小接口接入 application；不要继续创建新的横向万能包。

## 维护规则

- 文件名使用 `<domain>_<responsibility>.go`，测试与实现保持同名前缀。
- handler 不直接访问 repository；application 不直接导入 infrastructure。
- 手写 Go 文件目标不超过 1200 行。现有例外记录在 `.source-layout-allowlist`，只允许减少。
- 纯拆分不得改公开签名、序列化结构、SQL、goroutine 数量或热路径分配。
- 生成文件不手工格式化或拆分，使用 `make generate` 更新。

## 验证

```sh
make check-layout
go test ./...
golangci-lint run ./...
```

项目级验收使用仓库固定 Go 版本的 Docker 镜像执行，避免宿主机工具链差异。
