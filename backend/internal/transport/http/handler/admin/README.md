# Admin Handlers

管理端 HTTP 接口，负责权限已校验后的参数绑定、DTO 映射和 application 调用。

## 文件索引

文件按资源前缀组织：`account_*`, `user_*`, `group_*`, `setting_*`, `ops_*`, `usage_*`, `payment_*`, `channel_*`, `proxy_*`, `cluster_*`。`*_data.go` 只放请求/响应结构，`*_handler.go` 放端点实现，`*_test.go` 与对应前缀并排。

公共分页、ID 列表和幂等辅助仅在确有多个资源复用时保留；资源特有逻辑应回到对应前缀文件。

设置更新以 `setting_handler_update.go` 为编排入口：

| 文件组 | 职责 |
| --- | --- |
| `setting_handler_update_data.go` | 更新请求 DTO 与字段存在性语义 |
| `setting_handler_update_*_validation.go`, `setting_handler_update_prepare.go` | 按原始顺序解析旧值、执行安全门控并校验各设置域 |
| `setting_handler_update_mapping.go` | 将已校验请求映射为 application service 输入 |
| `setting_handler_update_persistence.go` | 持久化系统设置、独立策略和支付配置，并刷新支付 provider |
| `setting_handler_update_response*.go` | 回读最新设置、同步运行时副作用、审计并映射响应 DTO |
| `setting_handler_audit*.go` | 按设置域比较更新前后值，并按兼容顺序生成审计字段列表 |

设置更新的校验顺序、字段省略语义和持久化后副作用属于兼容性边界；扩展字段时应在对应阶段补齐映射与回归测试，不应重新堆回入口 handler。
