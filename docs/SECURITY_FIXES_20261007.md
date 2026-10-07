# 支付权限、公开入口和图片下载安全修复（2026-10-07）

本批修复支付管理权限组旁路、商户密钥明文保存、嵌入式前端截获网关别名、公开接口限流和图片结果下载边界。支付金额校验、原订单履约/退款边界及流式协议继续由原业务实现负责。

## 修复边界

| 问题 | 修复后的行为 | 主要 owner |
| --- | --- | --- |
| 支付管理端缺权限组校验 | `/api/v1/admin/payment/*` 要求显式 `payment.manage`；完整管理员保留原权限；其他已有权限组不会自动得到授权 | `routes/payment.go`、`middleware/admin_permissions.go`、`service/permission_groups.go` |
| 商户密钥明文落库 | 新写入使用既有 AES-256-GCM 格式；启动时分页、按原字节 CAS 重写存量 JSON；缺稳定密钥时拒绝写明文；不可解密配置报错而不被覆盖为空配置 | `service/payment_config_providers.go`、`payment/crypto.go` |
| 嵌入式前端截获 POST | 新旧 frontend middleware 都只处理 GET/HEAD 静态页；根网关别名和模型详情绕过 SPA | `webassets/embed_on.go` |
| 面板默认不限流 | 未保存策略时开启每用户 240/min、重查询 60/min、公开 IP 300/min；显式保存关闭仍生效；直连私网与回环也参与公开桶 | `service/setting_panel_rate_limit.go`、`middleware/panel_rate_limit.go` |
| OAuth2 失败不限流 | token/revoke 共享每 IP 20 次失败/分钟；成功请求不消耗失败桶，另有 300/min 入口桶；Redis 故障 fail closed；限流错误遵循 OAuth JSON 格式 | `middleware/oauth2_rate_limit.go` |
| auto_compat 信任任意私网 peer | 自动信任 loopback、Cloudflare 官方网段和显式代理；未列名私网与链路本地 peer 不能伪造 XFF；链中未列名私网跳同样不被跳过 | `shared/ip/resolver.go` |
| 上游图片结果 URL 拉取内网 | uploader 使用已有公开图片 HTTP port，验证全部 DNS 地址、固定连接 IP、逐跳重定向，保留账号代理/IPv6；管理员私网上游例外不放宽结果图片策略；base64/data URL 保留 | `service/image_storage.go`、`repository/http_upstream_public.go` |
| 低成本入口加固 | GET OAuth start/callback/bind/payment 共用 60/min；webhook 和 generated 各 120/min；resume resolve 60/min；webhook 在数据库实例选择前限流 | `routes/auth.go`、`payment.go`、`gateway.go` |
| 浏览器响应头 | COOP `same-origin-allow-popups` 保留 OAuth 窗口；Permissions-Policy 禁用相机、麦克风和定位；HSTS 只响应 TLS 或经可信 TCP peer 提供的单一 HTTPS scheme | `middleware/security_headers.go` |
| 匿名节点/版本信息 | `/ready` 保留 ready/reason，公开设置和 HTML bootstrap 不公开后端版本；完整信息仍在认证管理接口 | `routes/common.go`、`handler/setting_handler.go`、`service/setting_public.go` |
| 用户敏感操作 | 已启用 TOTP 的用户删 key、发送换绑邮箱验证码、换绑/解绑身份要求 session step-up；未启用 TOTP 用户保留既有密码和邮箱验证；前端弹窗验证后重试 | `middleware/step_up.go`、`routes/user.go`、keys/profile feature |
| 易支付商户密钥 HTTP 出网 | apiBase 必须 HTTPS，只有显式 loopback 开发夹具可用 HTTP；API 请求拒绝重定向 | `payment/provider/easypay.go` |
| 取消订单无限期恢复 | CANCELLED 与 EXPIRED 都仅在现有 5 分钟 grace 内接受付款成功恢复；超过 grace 保持原状态；合法竞态付款仍可恢复 | `service/payment_fulfillment.go` |
| URL 默认宽松及降级跳转 | config、Compose 和 `.env.example` 默认关闭 HTTP/私网例外；HTTP port 的私网规则独立于 host allowlist 开关；初始/重定向都校验 scheme；CRS 也遵守该规则 | config、`repository/http_upstream.go`、`service/crs_sync_service.go` |
| Gin 尾斜杠提前重定向 | 关闭自动 trailing-slash/fixed-path redirect，API 使用规范路径并经过全局链 | `server/router.go` |

网关 alias 的旧 `200 text/html` 是请求被 SPA 提前终止；该行为没有执行模型业务。修复后无凭证的请求进入 API 链并被鉴权拒绝。

## 升级与兼容

1. 保存每个节点原有的稳定 `TOTP_ENCRYPTION_KEY`。商户配置使用旧版本已支持的 `iv:authTag:ciphertext` 格式，不新增数据库列；不能用重启后变化的临时 key 保存密文。
2. **混跑或回滚前，先为所有旧、新节点配置相同、独立的 `PAYMENT_RESUME_SIGNING_KEY`**，例如单独生成一份 32 字节 hex 值。旧版本已支持此变量；先重启旧节点确认变量生效，再升级。未显式配置时，新版本用 TOTP master 派生专用 HMAC key，继续验证有有效期限的旧 token，但旧版本不认识新派生签名，不能在未配置共享签名 key 的情况下混跑或回滚。
3. 私网 Docker/Nginx/Caddy 反代需要把**实际 TCP peer IP/CIDR**加入 `server.trusted_proxies` 或管理端可信代理列表。代理必须覆盖客户端转发头，不能把整个可达内网无条件授权为代理。
4. 私有模型服务需要显式开启 `allow_private_hosts`；HTTP 模型服务还需显式开启 `allow_insecure_http`。这些管理员例外不适用于上游返回的结果图片 URL。
5. 等全部节点完成升级，再给支付运营人员授予 `payment.manage`。回退旧版本前从权限组中移除新权限键；旧版本不认识该权限。已有组默认不变，完整 admin 可完成迁移/恢复。
6. `step_up_enabled` 继续控制原管理员敏感操作的可选策略。已启用 TOTP 用户的新增凭据/身份门控独立于该管理员开关，避免关闭全局策略绕过用户 2FA。

公开业务查询、webhook 和 generated 的 Redis 故障维持 fail open，避免付款结果和上游重试被缓存故障阻断。OAuth2 凭据入口使用 fail closed；认证后面板桶仍保留管理员豁免。以上均为明确运行策略，不代表所有端点都使用失败关闭。

## 验证

Docker 使用固定 `golang:1.26.6-bookworm` 工具链，正式多阶段 Dockerfile 构建包含实际 Vite 资源。真实 HTTP 对照使用隔离 PostgreSQL 18、Redis 7、合成角色/商户配置/用户会话；图片地址测试使用三条 internal Docker 网络及本地模拟公开、私网、元数据地址，不连接真实云元数据。

运行检查的仓库入口（从根目录执行）：

```sh
make check-docs
```

后端相关检查从 `backend/` 执行：

```sh
go test -tags=unit ./internal/application/service ./internal/modules/payment/... ./internal/transport/http/... ./internal/platform/config ./internal/shared/ip ./internal/infrastructure/repository
go test -tags=unit,embed ./internal/transport/webassets ./internal/transport/http/server/...
```

前端相关检查从 `frontend/` 执行：

```sh
pnpm run typecheck
pnpm run lint:check
pnpm exec vitest run src/core/routes/__tests__/permissionGroups.spec.ts src/features/profile/__tests__/ProfileIdentityBindingsSection.spec.ts src/features/keys src/features/admin-settings/__tests__/permissionGroups.spec.ts
```

真实命令、逐项状态、原始 stdout/stderr、SHA 和源码回滚对照保存于随修复生成的 `VERIFICATION.txt`。源码 `ROLLBACK.sh` 恢复副本字节；运行数据和新权限/签名 key 的回退必须遵守上述迁移顺序。
