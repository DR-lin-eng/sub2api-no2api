# 降智账号 API

## 接口用途

管理员可以读取当前质量巡检中被判定为降智的账号，并同时查看最近一次
HTTP 401 认证失败的 OAuth 账号。接口只返回邮箱和本地 `account_id` 等诊断字段，
不会返回 access token、refresh token、API Key、完整凭据或请求正文。

质量巡检本身只检测启用的 OpenAI/Gemini OAuth 账号，因此本接口也只返回这两类
OAuth 账号。API Key、service account 和其他平台账号即使存在旧的降智标记或 401
错误，也不会出现在结果中。

## 请求

```http
GET /api/v1/admin/account-quality/degraded-accounts
Authorization: Bearer <admin-jwt>
```

也可以使用具备 `admin.accounts.read` scope 的 Admin API Key：

```http
GET /api/v1/admin/account-quality/degraded-accounts
x-api-key: <scoped-admin-api-key>
```

管理员 JWT 还受现有管理员权限组、合规确认和审计中间件保护。自定义管理员角色
需要 `accounts.manage` 权限；Admin API Key 需要 `admin.accounts.read`，只有
`admin.read` 不足以访问该接口。

接口为只读请求，不接受请求体，不触发新的上游探测，也不会修改账号状态。

## 成功响应

HTTP 状态码为 `200`，外层使用项目统一响应 envelope：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "items": [
      {
        "account_id": 101,
        "email": "degraded@example.com",
        "quality_status": "degraded",
        "reason": "degraded",
        "last_checked_at": "2026-09-14T01:00:00Z"
      },
      {
        "account_id": 102,
        "email": "expired@example.com",
        "quality_status": "error",
        "reason": "unauthorized",
        "http_status": 401,
        "error_message": "Authentication failed (401): token expired"
      }
    ],
    "total": 2
  }
}
```

### 字段说明

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `items` | array | 降智或 401 账号列表，按账号仓储返回顺序排列 |
| `total` | integer | `items` 数量 |
| `account_id` | integer | Sub2API 本地账号 ID |
| `email` | string | OAuth 凭据中的邮箱；历史凭据没有邮箱时返回空字符串，账号不会被隐藏 |
| `quality_status` | string | 当前质量状态，通常是 `degraded`；只有 401 错误时为 `error` |
| `reason` | string | `degraded` 表示质量阶段失败，`unauthorized` 表示检测记录中发现 401 |
| `http_status` | integer | 仅 401 项返回，固定为 `401` |
| `error_message` | string | 脱敏后的 401 错误摘要（截断在上游正文之前，最多 240 个 Unicode 字符）；其他降智项不返回该字段 |
| `last_checked_at` | string | 质量巡检最近检查时间；无法解析历史时间时省略 |

同一账号同时满足降智和 401 条件时只返回一项，并将 `reason` 设为
`unauthorized`、`http_status` 设为 `401`，避免重复计数。

401 的识别来源包括账号当前 `error_message` 和质量巡检最近 24 次历史中的错误
文本。匹配独立数字 `401` 或 `Unauthorized`；普通 400、403、429、超时和网络错误
不会被误判为 401。

## 空结果

没有符合条件的账号时仍返回成功响应，不返回 404：

```json
{
  "code": 0,
  "message": "success",
  "data": { "items": [], "total": 0 }
}
```

## 错误响应

错误继续使用统一 envelope：

| HTTP | `message`/原因 | 场景 |
| ---: | --- | --- |
| `401` | `Authorization required`、`Invalid admin API key` 等 | 缺少或无效管理员 JWT/API Key |
| `403` | `Permission required: accounts.manage` 或 scope 错误 | 管理员角色或 Admin API Key 权限不足 |
| `500` | `account inspection is unavailable` | 质量服务未装配或账号仓储不可用 |

注意：表格中的 `http_status: 401` 是某个被检测账号的上游认证失败，属于
HTTP 200 响应中的数据；HTTP 401 错误响应则表示调用本接口的管理员未通过认证。
两者含义不同，客户端应分别处理。

## curl 示例

```bash
curl --fail-with-body \
  -H 'Authorization: Bearer ADMIN_JWT' \
  'https://gateway.example.com/api/v1/admin/account-quality/degraded-accounts'

curl --fail-with-body \
  -H 'x-api-key: ADMIN_ACCOUNTS_READ_KEY' \
  'https://gateway.example.com/api/v1/admin/account-quality/degraded-accounts'
```

不要把响应写入公开日志、URL、Issue 或前端 bundle；邮箱和账号 ID 仍属于管理
诊断数据。生产调用建议使用 TLS、短期管理员凭据和最小 scope。
