# Admin API 调用文档

本文档说明如何使用管理员 JWT 或 scoped Admin API Key 调用 Sub2API 管理接口，并介绍 Admin API Key 的创建、权限范围、轮换和撤销方式。

## 基础信息

- 基础地址：`https://<your-domain>`
- Admin API 前缀：`/api/v1/admin`
- 账号列表 `GET /api/v1/admin/accounts` 支持 `oauth_quota` 筛选：`exhausted`（兼容值，任一 OAuth 窗口满额）、`has_quota`（有已知额度且当前没有满额窗口）、`with_reset`（OpenAI OAuth 有可用重置次数）、`5h_exhausted` 和 `7d_exhausted`（OpenAI OAuth 对应窗口满额）。筛选在数据库分页前检查已持久化快照；有效重置时间已过去的窗口按已重置处理，缺少快照或无法确认窗口的账号不会被误判为有额度或满额。
- JSON 请求头：`Content-Type: application/json`
- 机器调用认证头：`x-api-key: <admin-api-key>`
- 管理员页面调用：`Authorization: Bearer <admin-jwt>`

Admin API Key 适合服务间调用。管理员 JWT 适合浏览器管理页面和需要交互式 TOTP step-up 的操作。

## 创建 Admin API Key

首次创建建议在管理页面完成：

1. 使用管理员账号登录。
2. 进入“系统设置 -> 安全”。
3. 输入 Key 名称和有效期。
4. 勾选需要的权限范围。
5. 创建后立即保存完整 Key。

完整 Key 只会在创建或轮换时显示一次。服务端仅保存 SHA-256 摘要、前缀、后四位和权限元数据。

也可以使用管理员 JWT 调用创建接口：

```bash
BASE="https://<your-domain>"
ADMIN_JWT="<admin-jwt>"

curl -X POST "${BASE}/api/v1/admin/settings/admin-api-keys" \
  -H "Authorization: Bearer ${ADMIN_JWT}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "只读运维",
    "scopes": ["admin.read", "admin.ops.read"],
    "expires_at": "2026-12-31T23:59:59Z"
  }'
```

创建成功返回 `201`：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "key": "admin-<仅显示一次的完整密钥>",
    "metadata": {
      "id": "<key-id>",
      "name": "只读运维",
      "key_prefix": "admin-abcd",
      "last_four": "1234",
      "scopes": ["admin.read", "admin.ops.read"],
      "status": "active",
      "expires_at": "2026-12-31T23:59:59Z"
    }
  }
}
```

不要把返回的完整 Key 写入日志、URL、前端代码或 Git 仓库。

## 权限范围

| Scope | 允许的操作 |
| --- | --- |
| `admin.read` | 所有非敏感 Admin GET/HEAD/OPTIONS 请求 |
| `admin.write` | 未划分到专属资源的 Admin POST/PUT/PATCH/DELETE 请求 |
| `admin.users.read` | 用户管理读取接口 |
| `admin.users.write` | 用户管理写入接口 |
| `admin.accounts.read` | 上游账号管理读取接口 |
| `admin.accounts.write` | 上游账号管理写入接口 |
| `admin.settings.read` | 系统设置读取和 Admin API Key 列表 |
| `admin.settings.write` | 系统设置修改、Key 创建、编辑、轮换和撤销 |
| `admin.backups.read` | 备份元数据读取，不包含下载链接 |
| `admin.backups.write` | 备份创建、恢复和删除 |
| `admin.system.read` | 系统状态和版本读取 |
| `admin.system.write` | 系统生命周期操作 |
| `admin.audit.read` | 审计日志读取 |
| `admin.audit.write` | 审计日志管理操作 |
| `admin.ops.read` | 运维监控读取 |
| `admin.ops.write` | 运维规则、告警和配置修改 |

读权限不会自动授予写权限，写权限也不会自动授予读权限。需要完整访问时必须同时授予对应的 read 和 write scope。

以下敏感读取无论授予什么 scope 都不允许 Admin API Key 调用：

- `GET /api/v1/admin/accounts/data`
- `GET /api/v1/admin/proxies/data`
- `GET /api/v1/admin/backups/:id/download-url`

这些操作必须使用管理员 JWT，并通过接口要求的 TOTP step-up。

旧版单一 Admin API Key 仅保留只读兼容能力，不能修改设置、轮换自身或执行其他写操作。建议重新创建 scoped Key 后撤销旧 Key。

## 使用 scoped Key 调用 Admin API

```bash
BASE="https://<your-domain>"
ADMIN_API_KEY="admin-<your-key>"
```

读取用户，需要 `admin.users.read` 或 `admin.read`：

```bash
curl -sS "${BASE}/api/v1/admin/users/123" \
  -H "x-api-key: ${ADMIN_API_KEY}"
```

调整余额，需要 `admin.users.write`：

```bash
curl -X POST "${BASE}/api/v1/admin/users/123/balance" \
  -H "x-api-key: ${ADMIN_API_KEY}" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: balance-adjust-order-123" \
  -d '{
    "balance": 10,
    "operation": "add",
    "notes": "order 123"
  }'
```

读取运维状态，需要 `admin.ops.read` 或 `admin.read`：

```bash
curl -sS "${BASE}/api/v1/admin/ops/concurrency" \
  -H "x-api-key: ${ADMIN_API_KEY}"
```

## 按 OAuth 健康指标关闭调度

仓库附带的 `tools/disable_oauth_accounts.py` 会扫描 `type=oauth` 账号，并按最近一小时的账号统计判断：

- `avg_first_token_ms > 30000`；
- `success_rate < 0.60`（至少有 `--min-requests` 条请求，默认 1）；
- 账号返回的 OAuth 快照中，某个配额窗口达到 100% 且其重置时间仍有效。

脚本默认只预览；缺失 `hourly_usage` 时会中止并返回非零状态，不会把未知数据当成坏账号。它只将目标账号的 `schedulable` 设为 `false`，不会自动重新启用账号，也不会把 Admin API Key 写入文件或命令行参数。

```bash
export SUB2API_ADMIN_API_KEY='<admin-api-key>'

# 预览全部 OAuth 平台账号
python3 tools/disable_oauth_accounts.py --base-url "https://<your-domain>"

# 确认后执行；也可以加 --platform openai 只处理 OpenAI OAuth
python3 tools/disable_oauth_accounts.py --base-url "https://<your-domain>" --platform openai --apply
```

执行需要同时具备 `admin.accounts.read` 和 `admin.accounts.write`；旧版只读 Admin API Key 会被拒绝。脚本在本地按返回的快照和重置时间判定额度，不依赖远端筛选结果，也不需要读取或导出 OAuth 凭据。

管理面“账号巡检”使用同一套规则，接口如下：

- `GET /api/v1/admin/account-inspection`：读取设置、最近一次运行摘要和分页结果；支持 `status`、`type`、`search`、`page`、`page_size`。
- `PUT /api/v1/admin/account-inspection/settings`：保存自动开关、检查间隔和阈值。
- `POST /api/v1/admin/account-inspection/run`：按当前设置手动执行一次巡检。

自动 runner 默认关闭，需要管理员在页面开启；手动执行不依赖自动开关。`auto_disable` 开启时，异常且当前仍可调度的账号会通过现有调度快照同步路径批量停调。

## OpenAI OAuth 连续 429/502 熔断

“网关服务 -> 网关韧性 -> 429 回避与 OAuth 熔断”复用以下接口：

- `GET /api/v1/admin/settings/rate-limit-429-cooldown`
- `PUT /api/v1/admin/settings/rate-limit-429-cooldown`

请求中的 `auto_disable_enabled` 默认 `false`，`auto_disable_threshold` 允许 `1-100`。
开启后只统计 OpenAI OAuth 账号的账号级 429 与 502；API Key 对接账号、请求级上游过载、
参数错误和客户端取消不计入。成功请求会清零连续计数。达到阈值后，服务将账号的
`schedulable` 持久设为 `false`，账号列表通过 `scheduling_disabled_reason` 显示触发阈值和
最后状态码。该账号若绑定多个分组，会从所有相关调度候选中移除。

`auto_disable_quota_check_enabled` 默认 `false`。开启后，达到失败阈值时会触发实时主
Codex 额度检查，而不会直接停调；只有上游明确返回限额状态，或主额度窗口已用比例达到
`100%`，才会关闭调度。额度查询失败、缺少可判定数据、尚有额度，或仅未知/辅助额度桶
达到上限时都不会关闭账号；连续计数保留，后续账号级 429/502 会再次检查。该检查只读取主额度，不查询
重置额度明细或服务端 token 活动。

`auto_enable_after_quota_reset_enabled` 与 `auto_enable_when_quota_available_enabled` 也默认
`false`。前者会在额度检查记录的主 Codex 重置倒计时结束后，由分钟级后台任务恢复账号；
后者会在管理端主动刷新额度且主 Codex bucket 明确仍有额度时立即恢复账号。两者只处理由
上述 OpenAI OAuth 失败熔断写入专用恢复标记的账号；管理员手工停调会清除该标记，因此不会被
后台恢复。OpenAI API Key 账号以及其它平台/账号类型不参与自动恢复。

```bash
curl -X PUT "${BASE}/api/v1/admin/settings/rate-limit-429-cooldown" \
  -H "x-api-key: ${ADMIN_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "cooldown_seconds": 5,
    "auto_disable_enabled": true,
    "auto_disable_threshold": 3,
    "auto_disable_quota_check_enabled": true,
    "auto_enable_after_quota_reset_enabled": true,
    "auto_enable_when_quota_available_enabled": true
  }'
```

该操作需要 `admin.settings.write`。两个恢复开关关闭时，自动停调仍保持原有的人工恢复行为；
管理员在账号管理中单个或批量更新调度开关时，暂停原因、自动恢复标记和 Redis 连续失败计数会清除。

## Key 管理接口

### 查询列表

`GET /api/v1/admin/settings/admin-api-keys`

需要管理员 JWT，或具有 `admin.settings.read` / `admin.read` 的 Admin API Key。

```bash
curl -sS "${BASE}/api/v1/admin/settings/admin-api-keys" \
  -H "Authorization: Bearer ${ADMIN_JWT}"
```

返回值只包含脱敏信息，不包含完整 Key 或摘要。

### 编辑名称、权限和有效期

`PUT /api/v1/admin/settings/admin-api-keys/:id`

```bash
curl -X PUT "${BASE}/api/v1/admin/settings/admin-api-keys/<key-id>" \
  -H "Authorization: Bearer ${ADMIN_JWT}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "用户只读同步",
    "scopes": ["admin.users.read"],
    "expires_at": "2027-01-31T00:00:00Z"
  }'
```

将 `expires_at` 设置为 `null` 可以取消有效期限制。

### 轮换 Key

`POST /api/v1/admin/settings/admin-api-keys/:id/rotate`

```bash
curl -X POST "${BASE}/api/v1/admin/settings/admin-api-keys/<key-id>/rotate" \
  -H "Authorization: Bearer ${ADMIN_JWT}"
```

旧 Key 会立即失效，新 Key 只在本次响应的 `data.key` 中返回。

推荐轮换顺序：

1. 调用轮换接口获取新 Key。
2. 将新 Key 写入密钥管理系统。
3. 更新调用方配置并重启或热加载。
4. 使用只读接口验证新 Key。

### 撤销 Key

`DELETE /api/v1/admin/settings/admin-api-keys/:id`

```bash
curl -X DELETE "${BASE}/api/v1/admin/settings/admin-api-keys/<key-id>" \
  -H "Authorization: Bearer ${ADMIN_JWT}"
```

撤销后，使用该 Key 的后续请求返回 `401 INVALID_ADMIN_KEY`。

## 状态码与错误

| HTTP 状态码 | 错误码 | 说明 |
| --- | --- | --- |
| `401` | `UNAUTHORIZED` | 未提供认证信息 |
| `401` | `INVALID_ADMIN_KEY` | Key 不存在、已撤销或已过期 |
| `403` | `ADMIN_API_KEY_SCOPE_REQUIRED` | Key 缺少当前接口需要的 scope，或接口禁止机器 Key 调用 |
| `403` | `FORBIDDEN` | JWT 用户不是管理员 |
| `423` | `ADMIN_COMPLIANCE_ACK_REQUIRED` | 管理员尚未确认当前版本的部署与运营合规承诺 |

错误响应示例：

```json
{
  "code": "ADMIN_API_KEY_SCOPE_REQUIRED",
  "message": "Admin API key does not have permission for this operation"
}
```

## 安全建议

- 每个外部系统创建独立 Key，不要多人或多服务共享。
- 只授予实际需要的 scope，并设置合理的 `expires_at`。
- 将 Key 保存在 Vault、KMS 或部署平台的 Secret 中。
- 不要通过 query string 传递 Key。
- 不要在错误日志、审计 payload 或监控标签中记录完整 Key。
- 定期检查 `last_used_at`，撤销长期未使用的 Key。
- 对余额、兑换、支付回调等接口使用稳定的 `Idempotency-Key`。
- 对系统更新、备份恢复、凭证导出等高影响操作使用管理员 JWT 和 TOTP step-up。

支付集成的具体接口请参阅 [ADMIN_PAYMENT_INTEGRATION_API.md](./ADMIN_PAYMENT_INTEGRATION_API.md)。
