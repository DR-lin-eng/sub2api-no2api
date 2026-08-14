# Sub2API Repository Guide

本文件为代码代理和新贡献者提供最小、稳定的仓库上下文。开始修改前先读本文件，再按任务进入对应文档和源码；源码与测试始终是最终事实来源。

## 开始位置

1. 阅读 `docs/README.md` 选择文档入口。
2. 阅读 `docs/ARCHITECTURE.md` 确认层级和依赖方向。
3. 使用 `docs/CODE_MAP.md` 按任务定位文件，不要从全仓库盲搜开始。
4. 涉及网关、计费或管理端请求时，再读 `docs/REQUEST_LIFECYCLES.md`。
5. 进入目录后，优先读取该目录最近的 `README.md`。

## 仓库结构

- `backend/`: Go 1.26.6 模块化单体。入口在 `backend/cmd/server/`。
- `frontend/`: Vue 3、TypeScript、Vite、Pinia，使用 pnpm。
- `deploy/`: Docker Compose、安装脚本和运行配置示例。
- `docs/`: 架构、API、功能和运维专题文档。
- `openspec/`: 正在设计或交付中的规格及证据，不代表所有功能的长期入口。

后端推荐依赖方向为 `transport -> application -> domain`。`infrastructure` 实现 application 定义的端口，并在 Wire 装配时绑定。`shared` 不得反向依赖业务层。新增独立业务优先放入 `internal/modules/<domain>`，不要继续扩大万能 `service` 包。

## 任务定位

| 任务 | 首要入口 |
| --- | --- |
| 新增或修改 HTTP 路由 | `backend/internal/transport/http/server/routes/` |
| 修改请求/响应协议 | `backend/internal/transport/http/handler/` |
| 修改网关编排、调度、计费 | `backend/internal/application/service/` |
| 修改 PostgreSQL、Redis 或上游访问 | `backend/internal/infrastructure/repository/` |
| 修改表结构 | `backend/ent/schema/` 与 `backend/migrations/` |
| 修改管理端或用户端页面 | `frontend/src/features/<domain>/presentation/pages/`，再追到同域 `widgets/`、`data/`、`stores/` |
| 修改部署参数 | `deploy/config.example.yaml`、`deploy/.env.example` 和 `deploy/README.md` |

更细的功能到文件映射见 `docs/CODE_MAP.md`。

## 修改约束

- 不在 handler 中直接访问 repository；不在 application 中导入 infrastructure。
- 不手改 `backend/ent/` 生成代码或 `backend/cmd/server/wire_gen.go`；修改源定义后运行生成命令。
- 保持 API JSON、SSE、WebSocket 和错误格式的兼容性。流式与非流式路径必须分别验证。
- 计费是正确性关键路径。不得以丢弃、采样或无界内存排队代替可靠结算。
- 调度变更必须同时检查账号筛选、并发槽位、失败排除、粘性会话和释放路径。
- 前端 HTTP 调用统一通过 `frontend/src/core/networks/client.ts`；浏览器权限控制不能替代后端鉴权。
- 新增用户可见文本时同步 `frontend/src/core/i18n/locales/`。
- 只改与当前任务相关的文件；保留工作区中已有的用户改动。

## 验证命令

从仓库根目录执行：

```sh
make check-docs
make test-backend
make test-frontend
```

按风险缩小验证范围时：

```sh
cd backend && go test ./internal/application/service/...
cd backend && go test ./internal/transport/http/...
cd frontend && pnpm exec vitest run path/to/file.spec.ts
cd frontend && pnpm run typecheck
```

修改 Ent schema 或 Wire 图后：

```sh
cd backend && make generate
```

后端命令在 `backend/` 执行，前端命令在 `frontend/` 执行。宿主机 Go 缓存不可写时，使用仓库 Docker 流程或把 `GOCACHE` 指向可写临时目录。

## 文档维护

- 结构、入口、命令或跨层链路变化时，同一改动中更新对应文档。
- 文档链接到稳定目录或入口文件，避免复制大段易漂移的路由/配置清单。
- 新专题先加入 `docs/README.md`；目录职责变化时更新最近的目录 README。
- 提交前运行 `make check-docs`。
