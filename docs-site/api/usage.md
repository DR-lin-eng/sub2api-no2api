# 用量、计费与分组查询

这组只读端点用 API Key 本身认证，不需要登录控制台。它们不检查余额，余额用完后仍可调用。

## 查询额度和用量

<Endpoint method="GET" path="/v1/usage" />

```bash
curl -sS "https://api.example.com/v1/usage" \
  -H "Authorization: Bearer $SUB2API_API_KEY"
```

### 查询参数

| 参数 | 说明 |
| --- | --- |
| `days` | 每日用量返回的天数，范围 1 到 90 |
| `timezone` | 划分每日用量时使用的时区，例如 `Asia/Shanghai` |
| `start_date` | 模型统计的起始日期，格式 `YYYY-MM-DD`，默认为 30 天前 |
| `end_date` | 模型统计的结束日期，格式 `YYYY-MM-DD`，默认为今天 |

### 响应

`mode` 表示这把 Key 的限制方式。

| `mode` | 条件 | 主要字段 |
| --- | --- | --- |
| `quota_limited` | Key 设置了总额度或限速窗口 | `quota`、`rate_limits`、`expires_at` |
| `unrestricted` | Key 本身没有限制 | 订阅分组返回 `subscription`，否则返回 `balance` |

::: code-group

```json [quota_limited]
{
  "mode": "quota_limited",
  "isValid": true,
  "status": "active",
  "quota": {"limit": 50, "used": 12.5, "remaining": 37.5, "unit": "USD"},
  "remaining": 37.5,
  "unit": "USD",
  "rate_limits": [
    {"window": "5h", "limit": 10, "used": 2.4, "remaining": 7.6, "window_start": "2026-01-01T08:00:00Z"}
  ],
  "expires_at": "2026-12-31T23:59:59Z",
  "days_until_expiry": 120,
  "usage": {
    "today": {"requests": 42, "input_tokens": 18000, "output_tokens": 6400, "total_tokens": 24400, "cost": 0.61, "actual_cost": 0.61},
    "total": {"requests": 980, "input_tokens": 410000, "output_tokens": 150000, "total_tokens": 560000, "cost": 12.5, "actual_cost": 12.5},
    "average_duration_ms": 2300,
    "rpm": 3,
    "tpm": 1800
  }
}
```

```json [unrestricted（余额）]
{
  "mode": "unrestricted",
  "isValid": true,
  "planName": "钱包余额",
  "remaining": 23.8,
  "balance": 23.8,
  "unit": "USD"
}
```

```json [unrestricted（订阅）]
{
  "mode": "unrestricted",
  "isValid": true,
  "planName": "Pro",
  "remaining": 18.2,
  "unit": "USD",
  "subscription": {
    "daily_usage_usd": 1.8,
    "weekly_usage_usd": 9.4,
    "monthly_usage_usd": 31.0,
    "daily_limit_usd": 20,
    "weekly_limit_usd": 100,
    "monthly_limit_usd": 300,
    "daily_reset_at": "2026-01-02T00:00:00Z",
    "unlimited": false,
    "expires_at": "2026-02-01T00:00:00Z"
  }
}
```

:::

`rate_limits` 的窗口有 `5h`、`1d` 和 `7d` 三种，只返回 Key 上设置过的窗口。`usage`、`daily_usage` 和 `model_stats` 是统计数据，在取不到时会省略，客户端应按可选字段处理。`cost` 是按标准价格计算的金额，`actual_cost` 是乘以倍率后的实际扣费。

## 查询当前计费倍率

<Endpoint method="GET" path="/v1/sub2api/billing" />

返回这把 Key 当前生效的 token 计费倍率。

```bash
curl -sS "https://api.example.com/v1/sub2api/billing" \
  -H "Authorization: Bearer $SUB2API_API_KEY"
```

```json
{
  "object": "sub2api.key_billing",
  "schema_version": 1,
  "billing_scope": "token",
  "group_rate_multiplier": 1,
  "resolved_rate_multiplier": 1,
  "peak_rate_enabled": true,
  "peak_start": "09:00",
  "peak_end": "18:00",
  "peak_rate_multiplier": 1.5,
  "applied_peak_multiplier": 1.5,
  "effective_rate_multiplier": 1.5,
  "timezone": "Asia/Shanghai",
  "observed_at": "2026-01-01T02:00:00Z"
}
```

| 字段 | 说明 |
| --- | --- |
| `group_rate_multiplier` | 分组的默认倍率 |
| `user_rate_multiplier` | 管理员为你单独设置的倍率，没有时省略 |
| `resolved_rate_multiplier` | 分组倍率和用户倍率合并后的结果 |
| `peak_rate_enabled` | 分组是否启用了高峰时段倍率 |
| `applied_peak_multiplier` | 查询时刻实际应用的高峰倍率 |
| `effective_rate_multiplier` | 查询时刻最终生效的倍率 |
| `observed_at` | 本次结果的观测时间（UTC） |

`peak_start`、`peak_end`、`peak_rate_multiplier`、`applied_peak_multiplier` 和 `timezone` 只在分组启用高峰倍率时返回。响应带有 `Cache-Control: no-store`。站点以简易模式运行时此端点返回 404，Key 未绑定分组时返回 403。

## 查询可用分组

<Endpoint method="GET" path="/v1/api-key-groups" />

返回这把 Key 的所属用户可以使用的分组，以及每个分组对该用户生效的倍率。`bound` 为 `true` 的一项是这把 Key 当前绑定的分组。

```bash
curl -sS "https://api.example.com/v1/api-key-groups" \
  -H "Authorization: Bearer $SUB2API_API_KEY"
```

```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "api_key_id": 12,
      "group_id": 3,
      "group_name": "Claude",
      "platform": "anthropic",
      "rate_multiplier": 1,
      "group_rate_multiplier": 1,
      "source": "group_default",
      "bound": true
    }
  ]
}
```

`source` 为 `group_default` 表示使用分组默认倍率，`user_group_rate` 表示使用管理员为该用户单独设置的倍率。`platform` 是分组的平台类型，可用来判断这把 Key [能调用哪些端点](/guide/overview#分组决定可用端点)。
