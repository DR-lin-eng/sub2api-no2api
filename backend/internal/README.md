# Internal Architecture

`internal` 是后端不可被外部模块导入的实现区，采用“应用层 + 基础设施 + 传输层 + 垂直模块”的模块化单体结构。

## 文件夹索引

| 文件夹 | 允许包含 | 禁止包含 |
| --- | --- | --- |
| `application/` | 用例、业务编排、端口接口 | Gin handler、SQL/Redis 实现 |
| `domain/` | 领域值、纯规则 | 网络、数据库、配置读取 |
| `infrastructure/` | application 端口的具体实现 | HTTP 响应格式 |
| `transport/` | 协议解析、鉴权接入、响应映射 | 直接数据库访问 |
| `modules/` | 有清晰边界的领域能力 | 无归属的通用 helper |
| `platform/` | 配置、安全和运行时基础设施 | 产品业务流程 |
| `shared/` | 低层复用包、协议转换 | 对上层目录的反向依赖 |
| `bootstrap/` | 初始化和升级流程 | 常驻请求业务逻辑 |
| `integration/` | 跨组件集成测试 | 生产实现 |
| `testutil/` | 测试 fixture 和 stub | 生产调用路径 |

## 新模块准入

一个功能同时拥有自己的状态、策略和外部接口，且能通过少量端口接入主流程时，建立 `modules/<domain>`。只有无业务状态、被多个模块复用的代码才进入 `shared`。

目录迁移期间 package 名保持稳定，以避免把结构重排与 API 重命名混成同一次风险变更。
