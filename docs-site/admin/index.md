# Admin API

Admin API 面向站点管理员，用于以程序方式管理用户、上游账号、分组、系统设置和运维数据。它与网关 API 使用不同的前缀和凭据。

本页是概要。完整的接口说明、敏感操作限制和权限组配置见仓库的 [Admin API 调用文档](https://github.com/DR-lin-eng/sub2api-no2api/blob/main/docs/ADMIN_API.md)。

## 基础信息

| 项 | 值 |
| --- | --- |
| 前缀 | `https://api.example.com/api/v1/admin` |
| 机器调用 | `x-api-key: <admin-api-key>` |
| 管理页面会话 | `Authorization: Bearer <admin-jwt>` |
| 请求体 | `Content-Type: application/json` |

::: warning Admin API Key 不是网关 API Key
Admin API Key 以 `admin-` 开头，在「系统设置 → 安全」创建，只能调用 `/api/v1/admin`。用户的网关 API Key 不能调用管理接口。
:::

## 创建 Admin API Key

1. 用管理员账号登录站点。
2. 进入「系统设置 → 安全」。
3. 填写名称和有效期，勾选需要的权限范围。
4. 创建后立即保存完整 Key。完整 Key 只显示这一次。

## 权限范围

按最小权限授予。读权限不包含写权限，写权限也不包含读权限。

| Scope | 允许的操作 |
| --- | --- |
| `admin.read` | 所有非敏感的读取请求 |
| `admin.write` | 未划分到专属资源的写入请求 |
| `admin.users.read` / `admin.users.write` | 用户管理 |
| `admin.accounts.read` / `admin.accounts.write` | 上游账号管理 |
| `admin.settings.read` / `admin.settings.write` | 系统设置 |
| `admin.backups.read` / `admin.backups.write` | 备份 |
| `admin.system.read` / `admin.system.write` | 系统状态与生命周期 |
| `admin.audit.read` / `admin.audit.write` | 审计日志 |
| `admin.ops.read` / `admin.ops.write` | 运维监控 |
| `admin.redeem.secrets` | 读取明文卡密，需要单独勾选 |

部分敏感操作只允许管理员 JWT 会话调用，任何 scope 的 Admin API Key 都不能执行，例如导出账号数据、获取备份下载链接，以及管理 Admin API Key 本身。

## 调用示例

读取用户，需要 `admin.users.read` 或 `admin.read`：

```bash
curl -sS "https://api.example.com/api/v1/admin/users/123" \
  -H "x-api-key: $ADMIN_API_KEY"
```

调整余额，需要 `admin.users.write`。写操作建议带上 `Idempotency-Key`，避免重试造成重复执行：

```bash
curl -sS -X POST "https://api.example.com/api/v1/admin/users/123/balance" \
  -H "x-api-key: $ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: balance-adjust-order-123" \
  -d '{"balance": 10, "operation": "add", "notes": "order 123"}'
```

## 响应结构

管理接口统一使用下面的外层结构，`code` 为 `0` 表示成功，业务数据在 `data` 中：

```json
{
  "code": 0,
  "message": "success",
  "data": {}
}
```

## 相关文档

- [Admin API 调用文档](https://github.com/DR-lin-eng/sub2api-no2api/blob/main/docs/ADMIN_API.md)
- [支付集成 API](https://github.com/DR-lin-eng/sub2api-no2api/blob/main/docs/ADMIN_PAYMENT_INTEGRATION_API.md)
- [Management MCP](https://github.com/DR-lin-eng/sub2api-no2api/blob/main/docs/MANAGEMENT_MCP.md)
- [OAuth2 Provider](https://github.com/DR-lin-eng/sub2api-no2api/blob/main/docs/OAUTH2_PROVIDER.md)
