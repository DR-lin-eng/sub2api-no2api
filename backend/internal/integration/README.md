# Integration Tests

跨 application、infrastructure 和外部依赖边界的集成测试。测试通过 build tag 控制，不包含生产实现。

运行：`go test -tags=integration ./internal/integration/...`。需要 PostgreSQL/Redis 的用例应使用 Testcontainers 并确保清理资源。
