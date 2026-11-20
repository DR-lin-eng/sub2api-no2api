# Site migration

站间迁移的快照、合并和会话协议。`login-quota-keys` 保留普通用户的登录、余额和个人 Key；`clone_policy.go` 在数据离开源站前排除管理账号及凭据。命令及操作边界见[双站迁移](../../../../docs/SITE_MIGRATION.md)。

- `panel*.go` 管理网页任务、恢复租约、对端地址校验以及归属发起管理员的加密 CSV。
- `session.go` 和 `client.go` 负责临时密钥协商、加密分块、PostgreSQL 检查点与本地报告。HTTP 对端需显式选择，复用 [migrationhttp](../../shared/migrationhttp/README.md) 对请求和响应加密认证。
- `snapshot.go`、`policy.go` 和 `catalog.go` 控制源端投影、准入数据表和敏感字段再加密。
- `merge.go` 与 `references.go` 在同一事务中检查自然键、重建关系，记录拒绝或需要管理员处理的行。
- `clone.go` 与 `pages.go` 恢复独立的新站数据库和私有页面目录。建库和本地 schema 初始化由 infrastructure 注入。

独立 Go 测试覆盖加密记录、CSV、关系映射、凭证投影和静态页面。数据库专项使用已经运行仓库 migrations 的隔离 PostgreSQL。连接串使用 `SITE_MIGRATION_TEST_DSN` 和 `SITE_MIGRATION_POLICY_TEST_DSN`。
