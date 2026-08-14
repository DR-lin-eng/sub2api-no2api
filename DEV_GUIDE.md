# Sub2API 开发指南

本文面向本地开发、功能二开和提交前验证。架构边界见 [架构总览](docs/ARCHITECTURE.md)，按功能定位文件见 [代码地图](docs/CODE_MAP.md)。部署生产实例请使用 [deploy/README.md](deploy/README.md)，不要把本地开发配置直接用于生产。

## 环境要求

| 工具 | 版本来源 | 用途 |
| --- | --- | --- |
| Go | `backend/go.mod`，当前为 1.26.6 | 后端构建、生成和测试 |
| Node.js | `.github/workflows/backend-ci.yml`，当前为 24 | 前端工具链 |
| pnpm | `frontend/package.json`，当前为 11.17.0 | 前端依赖和脚本；不要混用 npm/yarn |
| PostgreSQL | `deploy/docker-compose.dev.yml` | 持久化数据和迁移 |
| Redis | `deploy/docker-compose.dev.yml` | 缓存、调度、队列和并发状态 |
| Docker + Compose | 推荐使用当前稳定版 | 一致的依赖环境与整栈验证 |

版本发生变化时，以这些机器可读文件为准，不要只更新本文中的数字。

## 首次阅读

建议按以下顺序建立上下文：

1. [文档中心](docs/README.md)
2. [架构总览](docs/ARCHITECTURE.md)
3. [代码地图](docs/CODE_MAP.md)
4. `backend/README.md` 或 `frontend/README.md`
5. 目标目录最近的 `README.md`、入口实现和相邻测试

代码代理还应读取根目录 `AGENTS.md`。

## 快速启动

### 方式一：Docker 构建本地源码

这是最接近交付镜像的开发验证方式。Compose 文件要求显式提供 PostgreSQL 密码：

```sh
cd deploy
cp .env.example .env
# 编辑 .env，至少设置 POSTGRES_PASSWORD；开发机密钥也不要提交。
docker compose -f docker-compose.dev.yml up --build
```

默认访问 `http://127.0.0.1:8080`。查看服务状态：

```sh
cd deploy
docker compose -f docker-compose.dev.yml ps
docker compose -f docker-compose.dev.yml logs -f sub2api
```

该方式会从当前工作区构建前后端，适合最终回归，不提供源码热更新。

### 方式二：前后端分别热更新

先准备 PostgreSQL、Redis 和 `backend/config.yaml`。配置模板来自 `deploy/config.example.yaml`；从 `backend/` 启动时，配置文件放在该目录即可被加载。

```sh
cp deploy/config.example.yaml backend/config.yaml
# 编辑 backend/config.yaml，指向本机 PostgreSQL/Redis，并设置开发用密钥。
```

启动后端：

```sh
cd backend
go run ./cmd/server
```

启动前端：

```sh
cd frontend
pnpm install --frozen-lockfile
pnpm run dev
```

Vite 默认监听 `3000`，并把 `/api`、`/v1`、`/setup` 代理到 `http://localhost:8080`。需要修改时使用：

```sh
VITE_DEV_PORT=3001 VITE_DEV_PROXY_TARGET=http://127.0.0.1:8080 pnpm run dev
```

`backend/config.yaml`、`deploy/.env` 和本地数据目录均被忽略，不得提交真实凭据。

## 常用命令

### 仓库根目录

```sh
make build                 # 构建后端和前端
make test                  # 后端完整检查 + 前端检查
make test-backend          # backend/Makefile test
make test-frontend         # lint + typecheck + 关键 Vitest
make test-frontend-critical
make check-docs            # 核心文档和链接检查
```

### 后端

以下命令在 `backend/` 执行：

```sh
make build
make check-layout
make test-unit
make test-integration
make test-e2e
go test ./internal/application/service/...
go test ./internal/transport/http/...
golangci-lint run ./...
```

`make test` 会先运行目录结构约束，再执行 `go test ./...` 和 golangci-lint。集成/E2E 测试可能需要 Docker 或本地依赖。

### 前端

以下命令在 `frontend/` 执行：

```sh
pnpm run dev
pnpm run lint:check
pnpm run typecheck
pnpm run test:run
pnpm exec vitest run src/path/to/example.spec.ts
pnpm run build
```

前端生产构建输出到 `backend/internal/transport/webassets/dist/`，由 Go 后端嵌入。该目录是生成产物，不直接修改。

## 生成代码

### Ent

修改 `backend/ent/schema/` 后：

```sh
cd backend
go generate ./ent
```

检查生成差异，并同步新增的生产迁移。Ent 生成代码不能代替 `backend/migrations/` 中的升级路径。

### Wire

修改构造器、provider set 或 `backend/cmd/server/wire.go` 后：

```sh
cd backend
go generate ./cmd/server
```

`backend/cmd/server/wire_gen.go` 是生成结果，不手工维护。

一次性更新两者可运行：

```sh
cd backend
make generate
```

## 后端开发流程

### 新增 API

1. 在 `internal/transport/http/server/routes/` 的所属域绑定路径与 middleware。
2. 在 handler 完成参数校验、context 提取和协议响应。
3. 在 application service 编排用例并定义所需端口。
4. 在 infrastructure repository 实现存储或外部访问端口。
5. 同步 DTO、前端 API/types、路由测试和业务测试。

handler 不直接访问 repository，route 闭包不实现业务。

### 修改数据库

1. 修改 `backend/ent/schema/`。
2. 新增向前迁移到 `backend/migrations/`。
3. 运行 Ent 生成。
4. 更新 repository、DTO/mapper、备份恢复和 fixture。
5. 运行 repository integration 和迁移相关测试。

迁移按文件名字典序执行并记录 checksum。生产回滚依赖备份恢复或补偿迁移，不应假定自动 down migration。

### 修改网关或调度

先确认真实请求路径和平台分流，再从 handler 追到 service。至少检查：

- API Key、分组和订阅 context
- 用户槽位与账号槽位的获取/释放
- 粘性会话与候选过滤
- 失败账号排除和最大 failover 次数
- 流式/非流式响应与错误格式
- 用量记录、计费和缓存失效

调用链详见 [关键请求链路](docs/REQUEST_LIFECYCLES.md)。性能相关改动要保留已有 benchmark，并报告改动前后结果。

### 修改计费

统一成本入口是 `BillingService.CalculateCostUnified`，网关用量入口分别位于 `gateway_usage_billing.go` 和 `openai_gateway_usage.go`。计费修改至少覆盖：

- 余额和订阅两种模式
- token、按次、图片等涉及的模式
- request ID 幂等与指纹冲突
- 并发提交与 Redis 队列恢复
- PostgreSQL 事务和账务缓存一致性

关键结算不得使用静默丢弃或无界内存队列。

## 前端开发流程

常规调用顺序：

```text
main / core routes -> feature presentation -> feature data datasource -> core networks -> backend route
```

- 业务默认归属 `frontend/src/features/<domain>/`；页面、领域组件、交互和 Store 分别进入 `presentation/pages/`, `widgets/`, `composables/`, `stores/`。
- 领域 HTTP 调用进入所属 feature 的 `data/datasources/`，统一通过 `frontend/src/core/networks/client.ts`。
- 跨业务复用 UI、页面、composable 和 UI 类型放 `frontend/src/common/`；应用级 Router、HTTP/session、i18n、主题、全局 Store、服务、常量和工具放 `frontend/src/core/`。
- `frontend/src/api/`、`frontend/src/stores/` 仍保留迁移期兼容导出；`frontend/src/types/` 是稳定共享类型入口。新代码按 owner 路径导入，不继续扩充兼容 barrel。
- 新增用户可见文案时同步 `frontend/src/core/i18n/locales/`。
- 管理端可见性不能代替后端权限检查。

更多约定见 [frontend/README.md](frontend/README.md)。

## 测试选择

| 改动范围 | 最低验证 |
| --- | --- |
| 纯文档 | `make check-docs` |
| 后端单个纯函数/规则 | 相邻 package test |
| handler、middleware、路由 | 对应 HTTP package test |
| repository、迁移、Redis | unit + integration/Testcontainers |
| 调度、并发、计费 | 相关 service/repository 测试；必要时 race/benchmark/整栈 |
| 前端组件或页面 | 相邻 Vitest + typecheck |
| 前端共享 API/store/router | 全部相关 spec + lint + typecheck |
| 生成代码或跨层契约 | 生成检查 + 后端/前端构建 |
| 发布或部署行为 | Docker 源码构建和运行探针 |

提交前优先运行仓库根目录 `make test`。若某项因缺少 Docker、外部服务或受限网络未运行，应在交付说明中明确列出。

## 常见问题

### Go 缓存不可写

如果测试在编译前因宿主机缓存权限失败，使用可写临时缓存：

```sh
cd backend
GOCACHE=/tmp/sub2api-go-build go test ./path/to/package
```

也可使用 Docker 验证。不要把 `.gocache/` 或模块缓存提交到仓库。

### interface 修改后大量测试编译失败

Go interface 新增方法会影响所有 mock/stub。先搜索 interface 定义和测试实现：

```sh
rg -n 'type .* interface' backend/internal
rg -n 'type .*Mock|type .*Stub' backend/internal -g '*_test.go'
```

评估是否真的需要扩大共享 interface；只有调用方需要的最小端口通常更容易维护。

### 前端依赖或 lockfile 漂移

只使用 pnpm。修改 `package.json` 后运行：

```sh
cd frontend
pnpm install
pnpm install --frozen-lockfile
```

提交 `pnpm-lock.yaml` 的对应变化。不要提交 `node_modules/`、Vite 缓存或 tsbuildinfo。

### 精确接口或配置与文档不一致

- HTTP 路由以 `backend/internal/transport/http/server/routes/` 为准。
- 配置字段以 `backend/internal/platform/config/`、`deploy/config.example.yaml` 和 `deploy/.env.example` 为准。
- 前端路由以 `frontend/src/core/routes/index.ts` 为准。
- 构建/测试命令以根 `Makefile`、`backend/Makefile` 和 `frontend/package.json` 为准。

确认漂移后，在同一改动中修正文档。

## 提交检查清单

- [ ] 改动位于正确的层和目录。
- [ ] 没有手改生成文件或构建产物。
- [ ] 新配置有默认值、环境映射、校验和示例。
- [ ] 新 API/字段有兼容策略与测试。
- [ ] 新文案已进入 i18n。
- [ ] 相关目录 README、代码地图或链路文档已同步。
- [ ] `make check-docs` 通过。
- [ ] 与风险匹配的后端、前端或 Docker 验证通过。
