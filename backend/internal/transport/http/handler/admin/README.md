# Admin Handlers

管理端 HTTP 接口，负责权限已校验后的参数绑定、DTO 映射和 application 调用。

## 文件索引

文件按资源前缀组织：`account_*`, `user_*`, `group_*`, `setting_*`, `ops_*`, `usage_*`, `payment_*`, `channel_*`, `proxy_*`。`*_data.go` 只放请求/响应结构，`*_handler.go` 放端点实现，`*_test.go` 与对应前缀并排。

公共分页、ID 列表和幂等辅助仅在确有多个资源复用时保留；资源特有逻辑应回到对应前缀文件。
