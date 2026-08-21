# Sub2API 文档中心

本页是开发、二次开发和代码代理的统一入口。面向部署用户的快速说明仍在根目录三语 README；本目录重点回答“系统如何组成、代码在哪里、改动应验证什么”。

## 新贡献者阅读顺序

1. [开发指南](../DEV_GUIDE.md)：搭建环境、运行项目、测试和生成代码。
2. [架构总览](ARCHITECTURE.md)：进程、层级、依赖方向和数据边界。
3. [代码地图](CODE_MAP.md)：按功能和任务快速定位实现与测试。
4. [关键请求链路](REQUEST_LIFECYCLES.md)：API Key 网关、管理端 API、计费与前端构建链路。
5. [后端目录索引](../backend/README.md) 或 [前端目录索引](../frontend/README.md)：进入具体子系统。

代码代理还应先读取根目录 [AGENTS.md](../AGENTS.md)。进入子树后继续读取 [backend/AGENTS.md](../backend/AGENTS.md) 或 [frontend/AGENTS.md](../frontend/AGENTS.md)；这些文件只保存高频、稳定的操作约束，细节以本目录和源码为准。

## 接口与集成

| 文档 | 内容 |
| --- | --- |
| [Admin API](ADMIN_API.md) | Admin API Key、权限范围和管理接口调用方式 |
| [Cloudflare 接口分类与边缘规则建议](CLOUDFLARE_EDGE_RULES.md) | 按浏览器页、自动化 API、纯 API 和强人机验证入口分类，方便编写 CF/WAF 规则 |
| [管理端支付集成 API](ADMIN_PAYMENT_INTEGRATION_API.md) | 外部支付页面与余额/兑换集成 |
| [异步图片任务](ASYNC_IMAGE_TASKS.md) | 异步 Images API 的启用、提交和轮询 |
| [批量图片 MVP](BATCH_IMAGE_MVP.md) | 批量图片任务的接口与状态模型 |

路由的最终事实来源是 `backend/internal/transport/http/server/routes/`。文档不维护一份完整路由副本，以免与代码产生双重事实。

## 功能设计

| 文档 | 内容 |
| --- | --- |
| [组合分组](COMPOSITE_GROUPS.md) | Composite group 的平台解析和使用约束 |
| [Codex OAuth 模拟的有意差异](codex/intentional-divergences.md) | 固定 Codex 源码 revision、纯 Go A/B 身份与 continuation 边界、暂缓的传输层差异 |
| [CPA 多号池动态负载](CPA_POOL_DYNAMIC_LOAD_BALANCING_CN.md) | CPA 凭据容量、动态分流和运维检查 |
| [调度候选索引优化](SCHEDULER_CANDIDATE_INDEX_OPTIMIZATION_CN.md) | 实验调度引擎、索引一致性和回退行为 |
| [账号级 IPv6 出口](IPV6_EGRESS.md) | 稳定账号源地址、失败关闭、HE 6in4 和 Linux/Docker 路由边界 |
| [支付系统（中文）](PAYMENT_CN.md) / [English](PAYMENT.md) | 支付服务商、订单、回调和配置 |
| [上游同步审查记录（2026-08-19）](UPSTREAM_SYNC_20260819.md) | 冻结上游 SHA、语义移植、复用和迁移冲突决策 |
| [上游同步审查记录（2026-08-21）](UPSTREAM_SYNC_20260821.md) | 最新上游 PR 的选择性移植、性能边界、Docker 与发布验证 |
| [上游同步审查记录（2026-08-22）](UPSTREAM_SYNC_20260822.md) | 增量 PR 的兼容性筛选、性能边界、Docker 与精确 SHA 发布验证 |

## 长期优化计划

| 文档 | 内容 |
| --- | --- |
| [前端架构优化计划](FRONTEND_ARCHITECTURE_OPTIMIZATION_PLAN.md) | Feature owner 收口、DTO、Query/Action、依赖门禁与分阶段迁移路线 |

处于方案或交付阶段的功能记录在 `openspec/changes/`。使用这些文档前，应同时核对实现和测试，不能把 proposal 当作当前运行事实。

## 部署与运维

| 文档 | 内容 |
| --- | --- |
| [部署索引](../deploy/README.md) | 部署方式和配置入口 |
| [Docker 部署](../deploy/DOCKER.md) | Compose 部署与升级 |
| [多实例部署](../deploy/MULTI_INSTANCE.md) | 多实例拓扑、共享依赖与管理页指标速查 |
| [Redis 调优](../deploy/REDIS_TUNING.md) | 小机器默认值和高吞吐调优 |
| [边缘安全](../deploy/EDGE_SECURITY.md) | 反向代理与入口防护 |
| [Apple container](../deploy/APPLE_CONTAINER.md) | macOS container 部署 |

生产配置字段以 `deploy/config.example.yaml` 为准，Compose 环境变量以 `deploy/.env.example` 为准。

## 合规文档

- [中文部署与运营合规承诺](legal/admin-compliance.zh.md)
- [English deployment and operation compliance commitment](legal/admin-compliance.en.md)

## 文档维护约定

- 代码是事实源，文档负责解释边界、入口和决策，不复制大段实现。
- 新增专题文档时在本页登记，并说明目标读者和事实源。
- 修改目录结构、启动入口、验证命令、请求链路或配置来源时，同步更新相关文档。
- 每个后端目录 README 维护“职责和文件前缀”，不要逐个列出所有文件。
- 文档中的命令必须标明执行目录；示例凭据只能使用占位符。
- 提交前在仓库根目录运行 `make check-docs`。
