# 认证

所有网关端点都用站点签发的 API Key 认证。Key 只通过请求头传递。

## 传递 Key 的方式

网关按下面的顺序读取 Key，取到第一个非空值为止。

| 请求头 | 示例 | 常见来源 |
| --- | --- | --- |
| `Authorization` | `Authorization: Bearer sk-your-api-key` | OpenAI SDK、Codex CLI、Claude Code |
| `x-api-key` | `x-api-key: sk-your-api-key` | Anthropic SDK |
| `x-goog-api-key` | `x-goog-api-key: sk-your-api-key` | Google GenAI SDK、Gemini CLI |

`/v1beta` 下的 Gemini 原生接口优先读取 `x-goog-api-key`，并额外接受查询参数 `?key=`，以兼容只会这样传 Key 的 Gemini 客户端。

::: warning 不要把 Key 放进 URL
除 Gemini 原生接口的 `?key=` 外，查询参数 `key` 和 `api_key` 都会被拒绝并返回 `400 api_key_in_query_deprecated`。URL 容易进入访问日志和浏览器历史，请始终使用请求头。
:::

## 鉴权与计费检查

一次请求会依次通过下面的检查，任一失败即返回对应错误。

| 状态码 | `code` | 含义 |
| --- | --- | --- |
| 401 | `API_KEY_REQUIRED` | 请求头中没有 Key |
| 401 | `INVALID_API_KEY` | Key 不存在或格式错误 |
| 401 | `API_KEY_DISABLED` | Key 已被停用 |
| 401 | `USER_INACTIVE` | Key 所属用户被停用 |
| 403 | `ACCESS_DENIED` | 请求 IP 不在 Key 的白名单内，或命中黑名单 |
| 403 | `API_KEY_EXPIRED` | Key 已过期 |
| 403 | `GROUP_NOT_ALLOWED` | Key 绑定的专属分组不再对当前用户开放 |
| 403 | `INSUFFICIENT_BALANCE` | 账户余额不足 |
| 403 | `SUBSCRIPTION_NOT_FOUND` | 订阅型分组下没有有效订阅 |
| 429 | `USAGE_LIMIT_EXCEEDED` | 订阅的日、周或月额度已用完 |
| 429 | `INVALID_AUTH_RATE_LIMITED` | 同一来源的无效认证尝试过多，稍后重试 |
| 503 | `API_KEY_AUTH_OVERLOADED` | 认证服务暂时过载，可退避后重试 |

这些错误的响应体形如 `{"code": "INVALID_API_KEY", "message": "Invalid API key"}`。Key 自身的额度用完时返回 429：OpenAI 兼容端点使用 `insufficient_quota` 错误对象，其他端点使用 `API_KEY_QUOTA_EXHAUSTED`。Gemini 原生接口的鉴权错误改用 Google 的错误结构。完整格式见[错误处理](/guide/errors)。

## 只鉴权、不检查余额的端点

下列只读端点在余额用尽后仍可调用，方便客户端展示状态：

- [`GET /v1/usage`](/api/usage)
- [`GET /v1/sub2api/billing`](/api/usage#查询当前计费倍率)
- [`GET /v1/api-key-groups`](/api/usage#查询可用分组)
- [`GET /v1/images/tasks/{task_id}`](/api/images-async)

多模态分组的视频任务查询与下载同样不检查余额，已受理的任务在余额用完后仍可取回结果。

## 安全建议

- 服务端调用时从环境变量或密钥管理服务读取 Key。
- 浏览器、移动端和桌面端应用应通过自己的后端转发，不要内置 Key。
- 为不同用途创建不同的 Key，并按需设置额度、过期时间和 IP 白名单。
- Key 泄露后立即在控制台停用或删除。
