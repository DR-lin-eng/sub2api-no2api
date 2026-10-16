# ChatGPT/Codex 桌面端深度分析报告
## ——遥测、网络请求全景与风控体系(静态逆向)

> **分析对象**
> - macOS:`Codex.dmg`(747MB)→ 挂载后为 `ChatGPT.app`,版本 `26.1007.71326`(build 20398)
> - Linux:`chatgpt_arm64.deb`(453MB)→ `/usr/lib/chatgpt/`,版本 `26.1007.71326`
> - 内部项目名 `openai-codex-electron`,bundle ID `com.openai.codex`,内部代号 **Owl**,构建自 `minirepo/codex/codex-apps/electron`
>
> **分析方法**:REA v4.0.1 静态重建(修复 2 个工具 bug 后完成)+ asar 头部解析 + bundle 字符串/AST 模式提取 + codesign/spctl 验证 + codex CLI(Rust)strings 分析。全部结论基于静态代码证据,未执行目标应用。
>
> **分析日期**:2026-10-14。证据文件见附录 C。

---

# 第一部分 真实性与完整性验证

## 1.1 macOS(dmg)

| 验证项 | 结果 |
|---|---|
| 公证(Gatekeeper) | ✅ `source=Notarized Developer ID` |
| 签名者 | `Developer ID Application: OpenAI OpCo, LLC (2DC432GLL2)` |
| 证书链 | Developer ID Certification Authority → Apple Root CA(完整) |
| 深度校验 | ✅ `codesign --verify --deep --strict` 通过,satisfies its Designated Requirement |
| 签名时间戳 | 2026-10-14 07:21:07(可信时间戳) |
| 指定要求(DR) | `identifier "com.openai.codex" and anchor apple generic and … certificate leaf[subject.OU] = "2DC432GLL2"` |

**唯一异常及定性**:asar 内部清单中 `darwin/permissions.node`(`@worklouder/wl-device-kit`)的声明 SHA256 与磁盘文件不符。定性依据:
1. 该文件本身带有效 OpenAI 签名(时间戳 07:18:23,**早于** bundle 总签名 07:21:07);
2. 整个 bundle 的 Apple 签名深度校验通过(该文件字节与 OpenAI 签名时一致);
3. Linux 版 asar 仅带 `native/linux/` 模块,不存在 darwin 模块,无此问题。

结论:**官方流水线在生成 asar 清单之后、对原生模块做了重签/重建(lipo 合并/加 hardened runtime),再整体签名**。是构建工艺瑕疵,不是篡改。REA 拒绝分析该 asar 是其 fail-closed 设计的正确行为。

## 1.2 Linux(deb)

| 验证项 | 结果 |
|---|---|
| 包元数据 | `Package: chatgpt`,`Version: 26.1007.71326`,`Architecture: arm64` |
| 维护者 | `OpenAI <support@openai.com>` |
| 主页 | `https://developers.openai.com/codex/app` |
| GPG | deb 带 `_gpgorigin` 签名源(本机无 gpg,未验证密钥) |
| asar 完整性 | ✅ REA 逐文件 SHA256 校验通过(经修复后的分析管线) |

## 1.3 entitlements(macOS,与风控相关部分)

```
com.apple.application-identifier          2DC432GLL2.com.openai.codex
com.apple.developer.aps-environment       production        ← APNs 推送
com.apple.developer.usernotifications.communication  true
com.apple.security.app-sandbox            false             ← 无系统沙箱(桌面应用常态)
com.apple.security.automation.apple-events true             ← 可发 AppleEvents(自动化其他应用)
com.apple.security.cs.allow-jit           true
com.apple.security.cs.allow-unsigned-executable-memory true ← Electron 必需
com.apple.security.device.audio-input     true              ← 麦克风
com.apple.security.device.camera          true              ← 摄像头
com.apple.security.personal-information.calendars true      ← 日历
com.apple.security.files.user-selected.read-write true
com.apple.security.network.client         true
com.apple.security.application-groups     [2DC432GLL2.com.openai.codex.notifications,
                                            2DC432GLL2.com.openai.sky.CUAService]  ← 计算机使用服务
keychain-access-groups                    [2DC432GLL2.*, 2DC432GLL2.com.openai.shared]
```

---

# 第二部分 应用架构速览

```
ChatGPT(Electron 主进程,入口 .vite/build/early-bootstrap.js)
 ├── early-bootstrap:开启 WebMCP/Blink 特性 → 网络启动自检(失败即退出)
 ├── startup-requirements:initializeNodeNetworkPermissions(Linux 需先 app.whenReady)
 ├── bootstrap-DOZRFVdo.js(461KB):Sentry 初始化、窗口/会话、托盘
 ├── main-CwxgTOle.js(3.8MB):主逻辑(IPC 网关 performDesktopFetch、DeviceCheck、
 │                        文件夹信任、MCP 应用沙箱、远程控制客户端…)
 ├── worker.js(2.4MB):git worker(worker_threads,815 处 git 逻辑)
 ├── preload.js / browser-page-preload.js / sandbox-preload.js:三套 preload
 └── 本地 app-server(JSON-RPC over ws://codex-app-server)
      ├── codex(254MB Rust 静态二进制,含 attestation.rs / installation_id.rs)
      ├── codex-code-mode-host(67MB Rust)
      └── cua_node(定制 Node 24.21.0-cua.1,计算机使用运行时)
webview(内嵌前端,20,117 个文件,chatgpt.com 同源代码)+ 8 个官方插件(RSA 签名校验)
```

Electron 安全面(REA 重建证据):62 个 IPC 操作 / 24 个主进程 handler / **15 处发送方校验** / 6 个 BrowserWindow,全部窗口 `sandbox:true + contextIsolation:true + nodeIntegration:false`;权限处理器默认全拒绝(`setPermissionCheckHandler(()=>!1)`),仅 MCP 应用沙箱白名单与同源 `clipboard-sanitized-write` 例外。

---

# 第三部分 遥测报告(Telemetry)

## 3.1 Sentry —— 崩溃与异常

**端点**(bootstrap-DOZRFVdo.js):
```
DSN: https://6719eaa18601933a26ac21499dcaba2f@o33249.ingest.us.sentry.io/4510999349821440
```

**初始化参数**(反混淆后):
```js
Sentry.init({
  ipcMode: Classic,
  dsn,
  environment: buildFlavor,                    // prod/beta/nightly/internal-alpha
  release: `codex@${version}`,                 // 如 codex@26.1007.71326
  dist: buildNumber,                           // 20398
  beforeBreadcrumb: 裁剪函数,                  // 每条 ≤2KiB、≤20 键,超出标记 codex_truncated_breadcrumb_data
  beforeSend: Tw({app, maxMinidumpAttachments}),
  tracesSampleRate: 0,                         // ★ 性能追踪完全关闭
  integrations: 移除 ChildProcess/Console/Minidump/RendererEventLoopBlock,
    + 会话事件监听: ['clean-exit','abnormal-exit','killed','crashed','oom','integrity-failure']
    + 日志面包屑: 仅 warn/error 级别
    + 路径改写: 项目根 → "app:///" 前缀(去除本地路径)
})
```

**采样策略**:`Pw = 0.001` —— **Prod 构建仅 0.1% 概率启用 minidump 附件上传**;非 Prod 全量。
**标签**:`sessionId=codexAppSessionId`、`preRelease`、`minidumpAttachmentUploadEnabled`、`buildFlavor`、`bundle=electron`、`host=app`。
**preload 侧** 暴露 `__SENTRY_IPC__`(渲染进程错误经 IPC 转发到主进程上报)。
**特别事件**:`integrity-failure`(应用完整性检测失败)作为会话事件上报 —— 与风控联动(见第六部分)。

## 3.2 CES —— 产品分析事件流

**端点**:`https://chat.openai.com/ces/v1/telemetry/intake`(主进程与 git worker 各一个客户端实例)

**协议特征**(src-DCNiwad_.js / worker.js):
```js
const INTAKE = 'https://chat.openai.com/ces/v1/telemetry/intake';
const AUTH = 'dummy-token';          // 占位令牌(事件靠会话上下文关联)
maxBatchSize = 2000;                 // 每批最多 2000 条
flushIntervalMs = 30_000;            // 30 秒定时冲刷
maxBatchBytes = 4 MiB;
retryAttempts = 5;                   // 失败重试 5 次
analyticsEnabled 默认 = true;
```
批量缓冲、超时冲刷、指数重试;事件体为 JSON 数组批量提交。

## 3.3 Statsig —— 实验配置与功能开关

**端点**:
```
https://ab.chatgpt.com/v1                     ← 主端点(自建代理)
https://ab.chatgpt.com/v1/sdk_exception       ← SDK 异常上报
https://api.oaistatsig.com/v1                 ← 直连 API
https://statsigcdn.openai.com/v1              ← 配置 CDN
```

**每个请求体都携带**(Statsig SDK 标准):
```json
{
  "statsigMetadata": {
    "stableID": "<持久UUID>",     // 见 6.3,设备级身份
    "sessionID": "<会话UUID>",
    "sdkType": "...",
    "fallbackUrl": "...",
    ...StatsigMetadataProvider.get()   // 标准浏览器/系统元数据
  }
}
```
**用途面**:模型路由(`modelRouting`)、自动化任务调度、桌面功能可用性(`getDesktopFeatureAvailability`)、灰度放量(`codex-app-background-paginated-rollout-migration`)等 —— 产品核心开关全部经 Statsig 下发,服务端可按设备(stableID)精确控制功能与模型配额。

## 3.4 OpenTelemetry —— 链路追踪

**端点**:OpenAI 域 + **`/o11y/v1/traces`**(可观测性路径)
**请求头**:`Content-Type: application/json` + 桌面表面头(desktopOriginator 等)

**开启条件**(默认关):
```js
traceExportEnabled = 用户配置 otel.trace_exporter !== 'none'   // 用户可关
desktopTraceSampleRate = process.env.CODEX_OTEL_TRACES_ENDPOINT == null ? 0 : 1
```
**强制 loopback 校验**(防调试数据外泄):
```js
if (t != null && !['127.0.0.1','[::1]','localhost'].includes(new URL(t).hostname))
  throw Error('Desktop trace endpoint override must be loopback');
```

## 3.5 其他常驻网络行为

| 行为 | 目标 | 频率/触发 |
|---|---|---|
| Sparkle 更新检查 | `updates.oaistatic.com/codex/app/appcast` | Sparkle 默认周期 |
| Windows 更新清单 | `persistent.oaistatic.com/codex-app-{prod,beta}/windows-*.json` | 按渠道 |
| 网络启动自检 | DoH(`cloudflare-dns.com`)等 | 每次启动,失败拒绝启动 |
| crashpad 崩溃收集 | `browser_crashpad_handler` 本地进程 | 崩溃时(与 3.1 联动) |
| codex CLI 分析开关 | `analytics_enabled` 标志 + OTEL | agent 会话 |

**遥测小结**:崩溃 0.1% 采样、Sentry traces 为 0、OTel 默认关且出口锁 loopback、面包屑 2KiB 裁剪、本地路径 `app:///` 匿名化 —— 崩溃遥测姿态克制;**持续开启的是 CES(产品事件)与 Statsig(实验/开关)**,二者均携带设备级持久身份。

---

# 第四部分 OpenAI 网络调用面（静态目录；逐请求还原见附录 D）

## 4.0 统一请求网关与头部治理

所有桌面端请求经 `performDesktopFetch()` 统一网关,采用 **fail-closed 附加规则** —— 以下信息只允许附加到白名单域,违反即抛错拒绝请求:

| 附加信息 | 允许范围 | 违规报错文本(代码内原文) |
|---|---|---|
| `Authorization` 认证 | OpenAI 域 | `Refusing to attach authentication to non-OpenAI URL` |
| 桌面表面头(desktop surface) | OpenAI 域 | `Refusing to attach desktop surface to non-OpenAI URL` |
| DeviceCheck token | **仅** `/ios/attestation_challenge`、POST `/(backend-api\|api)/messaging/rooms/{id}/messages` | `Refusing to attach DeviceCheck token to unexpected URL` |
| 完整性状态 `X-OAI-IS` | OpenAI 域 | `Refusing to attach integrity state to non-OpenAI URL` |
| `X-OpenAI-Account-Routing-Override` | —— | **用户传入一律删除**(deleteHeader) |

**认证/表面附加白名单**(`isDesktopAuthAllowedUrl`):
```
允许: openai.com 及全部子域、chatgpt.com 及全部子域(但 ab.* 除外——遥测域不带账号认证)、
      localhost / localhost:8000(开发)、cloud environment status 端点、
      (按配置的额外桌面端点 g.nt)
明确拒绝: status.openai.com(状态页不带任何凭证)
协议限制: 仅 http/https
```

**账号一致性防混用**：`X-OpenAI-Expected-Account-Id` 本地读取后删除并用于 expectedAccountId 比较，矛盾返回 `account_changed`（409）；发送前还重查 accountId/userId。它不是上游需要透传的认证头（附录 D）。

## 4.1 chatgpt.com —— 主 API

| 请求 | 详情 |
|---|---|
| `POST /backend-api/codex` | **codex agent 主调用端点**(CLI/桌面共用)。请求头见 4.1.1;体为 Responses 风格流式请求,含完整会话上下文 |
| `POST /(backend-api\|api)/messaging/rooms/{roomId}/messages` | **消息发送。此端点强制附加 DeviceCheck token**(macOS)。体为消息 JSON(roomId、正文、附件) |
| `GET /backend-api/…` 系 | 会话/配置/通知订阅等;含 `config/read`、`config/batchWrite`(经 app-server 转发) |
| `POST /notifications/subscription/deregister` 等 | 通知订阅管理(带 `Content-Type: application/json`、桌面表面头) |
| `GET /codex`、`/codex/install.sh`、`/codex/install.ps1` | 落地页与 CLI 安装脚本 |
| `GET /codex/remote/control/client/refresh/finish` | 远程控制客户端刷新(`requireDeviceIdentityHash: true` —— **需设备身份哈希**)。响应给 `client_id`、`remote_control_token`,后续用 `x-codex-client-session-token: Bearer <token>` 头,且 `requiresDeviceKeyProof: true`(**设备密钥证明**) |
| `GET /user-verification/{type}`(如 `credentials`) | **用户验证注册**。请求头:`Content-Type: application/json`、**`x-openai-use-sheep: 1`**、**`OAI-Device-Id: <deviceId>`**(POST 时)。GET 响应经 zod 解析出 `credentials` 列表;15 秒超时 |

**4.1.1 Codex 相关头部/metadata 符号目录**（CLI strings；不是单次请求头全集，方向与条件见附录 D）:

```
Authorization: Bearer <chatgpt OAuth token>
chatgpt-account-id: <accountId>                 ← 账号路由
originator: codex_rmcp_cli | codex_http_cli | codex_app_server_cli | Codex Browser
session_id / thread_id / turn_id / window_id / context_window_id / agent_name
x-codex-window-id / x-codex-turn-metadata / x-codex-parent-thread-id
x-codex-turn-state
x-openai-subagent-request                        ← 子代理请求标记
x-codex-installation-id                          ← 安装持久 UUID(installation_id.rs)
x-codex-routing-hint                             ← 路由提示
x-oai-attestation                                ← CLI 侧证明信封(attestation.rs)
x-codex-client-id / x-codex-client-session-token ← 远程控制会话
x-codex-git-source                               ← git 操作来源标记
x-codex-protocol-version / x-codex-subscribe-cursor
x-codex-websocket-max-unfragmented-message-bytes
x-oai-request-id / x-openai-request-id / x-client-request-id
x-codex-ws-stream-request-start-ms               ← WS 流计时
x-openai-internal-codex-responses-lite           ← 内部轻量响应标记
OAI-Device-Id                                    ← 设备 ID(浏览器/验证场景)
OAI-Language / OAI-Package-Name / OAI-Product-Sku / X-OpenAI-Product-Sku
X-OpenAI-Expected-Account-Id
X-OpenAI-Aeon-Messaging-Trace                    ← 消息链路追踪
X-OpenAI-Fedramp: true                           ← FedRAMP 部署
x-openai-use-sheep: 1                            ← 内部路由(用户验证请求)
X-OpenAI-Allow-Environmentless-Fallback
```

**turn 元数据随每轮发送**(服务端可见完整拓扑):
```
session_id, thread_id, turn_id, window_id, window_number, context_window_id,
agent_name, turn_started_at_unix_ms, request_kind, compaction, code_mode_tool_names,
tool_namespaces_info, forked_from_thread_id, forked_from_ordinal_exclusive,
parent_turn_id, root_turn_id, subagent_kind, thread_source, turn_trigger,
sandbox, sandbox_mode, auto_review_enabled, node_repl_auto_review_required,
node_repl_disabled, workspaces, history_ingest_requested, analytics_enabled,
responses_api_metadata(≤16 键、键≤短 ASCII、值≤128 字节,客户端预校验)
```

**浏览器(Codex Browser)请求头**(内置浏览器场景):
```
oai-did: <deviceId>                              ← 设备 ID
ChatGPT-Account-ID: <accountId>
originator: Codex Browser
User-Agent: <桌面 UA,含 CodexBrowser 前缀,见 4.9>
sec-ch-ua / sec-ch-ua-mobile: ?0 / sec-ch-ua-platform   ← Chromium client hints
```
scoped UA 注入(打包版):`owl-scoped-user-agent-additional-hosts: openai.com,chatgpt.com,chatgpt.site,chatgpt-team.site` + 前缀 `CodexBrowser`。

## 4.2 auth.openai.com —— 认证

| 请求 | 详情 |
|---|---|
| OAuth 授权流程 | 标准 PKCE;CLI 无头登录走 `auth/headless_chatgpt_login` |
| `GET /api/accounts` | 账号列表(响应解析 token/账号信息) |
| 设备授权流 | app-server RPC:`deviceauth/usercode` → `deviceauth/callback` → `deviceauth/token`(`codex login --device-auth` 用于远程/无头机器) |
| 令牌刷新 | `ChatgptAuthTokensRefresh*`;刷新失败有专用错误通道 |

**站点自动认证**(`OAI-Sites-ID-Token`,autoAuthForSites):对 HTTPS OpenAI 站点自动签发按站点 ID token(zod:`{status:'issued', id_token, expires_in}`),仅限 https、无端口/用户名/密码的 URL。

## 4.3 api.openai.com

```
/auth     认证相关
/mfa      多因素认证
/profile  用户资料
```

## 4.4 chat.openai.com —— 遥测
见 3.2(`POST /ces/v1/telemetry/intake`,批量 JSON,`dummy-token`)。

## 4.5 Statsig 域(见 3.3)

## 4.6 WebSocket 通道

| 通道 | 用途 |
|---|---|
| `ws://codex-app-server`(本地) | 桌面 UI ↔ 本地 app-server 的 JSON-RPC 中枢,全部业务方法经此转发(方法表见第五部分) |
| `wss://codex-cloud-backend.chatgpt.com` | 云端 Codex 后端(云会话/远程线程) |

## 4.7 MASQUE 代理

```
masque-proxy-origins.txt: https://codex-portals.api.openai.com
```
HTTP/3 CONNECT-UDP 隧道(MASQUE),用于浏览器隔离/远程串流流量;配合 `IwaKeyDistribution`(Isolated Web App 密钥分发)与 `PrivacySandboxAttestationsPreloaded` 组件。

## 4.8 更新与静态资源

```
updates.oaistatic.com/codex/app/appcast                          ← macOS Sparkle
persistent.oaistatic.com/codex-app-prod/windows-store-update.json
persistent.oaistatic.com/codex-app-prod/windows-release.json
persistent.oaistatic.com/codex-app-beta/windows-store-update.json
oaisidekickupdates.blob.core.windows.net                         ← Azure 兜底
```

## 4.9 User-Agent 与来源标识

- Electron surface UA：`Codex Desktop/<appVersion> (<platformLabel>; <arch>)`，默认 originator `Codex Desktop`；由实际 Xt/sn/on 工厂确认。`CodexBrowser` 是 browser scoped prefix，完整 runtime UA 待 capture，不能与 surface/engine 混同（附录 D）。
- 多调用面 originator 符号目录（不等于本构建 surface 默认值，默认 Codex Desktop 见附录 D）:`codex-desktop`(桌面)、`codex_rmcp_cli`、`codex_http_cli`、`codex_app_server_cli`(CLI)、`Codex Browser`(内置浏览器)、`chatgpt-dictation`(语音)
- WebRTC:强制 DTLS 1.3(`WebRTC-ForceDtls13`)、ICE handshake 加密(实时语音 `thread/realtime/sdp`)

## 4.10 非 OpenAI 外联(完整性)

| 域名 | 用途 |
|---|---|
| `o33249.ingest.us.sentry.io` | Sentry |
| `registry.npmjs.org` | agent 自动安装 npm 工具 |
| `download-cdn.jetbrains.com` | JetBrains IDE 下载(编辑器集成) |
| `api.stripe.com` / `pm-redirects.stripe.com` / `paypal.com` | 支付 |
| `cloudflare-dns.com` | DoH 网络自检 |
| `statsigapi.net` / `docs.statsig.com` | Statsig 公共域(次要) |
| `github.com` | 文档/链接(26 处引用) |

---

# 第五部分 本地 app-server JSON-RPC 方法面(桌面↔agent 中枢)

**账号/认证类**
```
account/login/start         account/login/completed(通知)
account/logout              account/read
account/usage/read          account/rateLimitResetCredit/consume   ← 限流重置额度消耗
account/rateLimits/updated(通知)
accounts/check              accounts/verified_access               ← 账号已验证访问
auth/agent_identity         auth/callback   auth/permission
auth/headless_chatgpt_login
deviceauth/usercode | deviceauth/callback | deviceauth/token      ← 设备授权登录
authorize/token/register
```

**会话/轮次类**
```
turn/start                  turn/completed(通知)
turn/moderationMetadata(通知)          ← ★ 审核元数据随轮次下发
model/verification(通知)               ← ★ 模型验证
model/rerouted(通知)                   ← 模型重路由(服务端换模型)
model/safetyBuffering/updated(通知)    ← ★ 安全缓冲(输出审核)状态
guardianWarning(通知)                  ← ★ 守护警告
conversation/normalize
thread/realtime/started | itemAdded | item/started | item/transcript/delta |
  item/completed | transcript/done | outputAudio/delta | sdp | error | closed   ← 实时语音(WebRTC)
```

**配置类**
```
config/read | batchWrite | edit | cache/init | managed_features | mod | bundle |
requirements | runtime_refresh | permissions | otel | network_proxy_spec
config/requirements / managed_requirements(服务端下发配置要求)
```

**模型/MCP/工作区**
```
model/meta   model/extension   models/pricing   models/executed_tool_calls   models/time
modelcontextprotocol/*(完整 MCP 协议:Initialize、tools、prompts、resources、elicitation、tasks)
workspace/*(沙箱策略、可写根、worldWritableWarning)
windows/sandbox/setupCompleted   windows/worldWritableWarning
fs/changed(通知)   fuzzyFileSearch/sessionUpdated
```

**通知/其他**
```
notifications/initialized | cancelled | message | progress | tasks |
  subscriptions/acknowledged | subscriptions/listen
remoteControl/status/changed(通知)                       ← 远程控制状态
externalAgentConfig/import/progress | completed           ← 外部 agent 迁移
account/gatewayOAuth/changed
AttestationGenerate(含 AttestationGenerateParams)         ← ★ CLI 证明生成
```

**app-server/Remote Control 协议符号（方向逐项见附录 D，不是所有模型请求必发头）**:`x-codex-protocol-version`(协议版本协商)、`x-codex-subscribe-cursor`(订阅游标)、`x-codex-websocket-max-unfragmented-message-bytes`。

---

# 第六部分 账号风控体系(重点)

## 6.1 Apple DeviceCheck 设备证明(最强硬件级信号)

**触发条件**:仅 `darwin` + 受支持架构(arm64/x64 按构建);`devicecheck.node` 原生模块;不支持时返回错误码(1 平台不支持 / 2 架构不支持 / 3 设备不支持 / 4 生成失败),**错误码也打包上报**。

**令牌封装格式**(反混淆 `ui()`,二进制 TLV):
```
"v1" . base64url( 0xA0 || TLV[] )
TLV 字段:
  token      → Apple DeviceCheck token(base64)
  bundle_id  → com.openai.codex
  f(信号集)  → 嵌套 TLV:
      [0] schemaVersion
      [1] preferredLanguages[]     ← 系统语言列表
      [2] locale                   ← 区域
      [3] timezone                 ← 时区
      [4] screenSizeSum            ← 屏幕尺寸之和(多屏求和)
      [5] screenScale              ← 缩放系数(HiDPI 特征)
      [6] appSessionId             ← 会话 ID
  t          → 生成耗时 ms
```

**发送位置(硬编码白名单,其余一律拒绝)**:
```
1) 路径以 /ios/attestation_challenge 结尾的请求
2) POST /(backend-api|api)/messaging/rooms/{roomId}/messages   ← 发消息即带设备证明
```

**配套 cookie 管理**(`deviceCheckCookieManager`):桌面表面请求统一 `ensureCookie`(设备 cookie)+ `attachToken`。

**服务端能力**:令牌可向 Apple 验证设备真实性/临时性,配合上面的设备指纹信号做交叉校验(模拟器/改机/重放检测在服务端完成)。

## 6.2 X-OAI-IS 完整性状态协议(服务端锚定会话状态机)

```
请求头:  X-OAI-IS: <state>            ← 客户端回传当前状态
响应头:  X-OAI-IS-Update: <state>     ← 服务端下发状态更新
```
- 客户端 `integrityStateStore`(globalState 持久)保存状态;`storeResponseUpdate()` 解析响应更新,`getRequestState()` 取出附头。
- 状态转移校验:`gb({currentState, expectedState, nextState})` —— 期望状态不匹配则拒绝更新(**防重放/乱序**)。
- 每次请求前 `deleteHeader(X-OAI-IS)` 再按需附加 —— 头不可被调用方伪造注入。
- 性质:**服务端可为每个客户端会话维护加密状态,实现风控挑战、客户端标记、异常检测**;客户端代码只搬运,不解读。

## 6.3 三层持久身份体系

| 层 | 标识 | 存储位置 | 生成方式 | 出现于 |
|---|---|---|---|---|
| 设备级 | Statsig `stableID` | `statsig.stable_id.<storageKey>`(localStorage/storage) | UUID v4,首用生成后持久 | 全部 Statsig 请求 |
| 设备级(浏览器) | `oai-did` | 设备管理器 | deviceId | Codex Browser 请求、user-verification |
| 安装级 | `x-codex-installation-id` | CODEX_HOME(`~/.codex`,installation_id.rs) | 安装时生成 | agent 全部会话请求 |
| 会话级 | `codexAppSessionId` | 会话生命周期 | 每次会话 | Sentry 标签、DeviceCheck 信号[6]、turn 元数据 |
| 账号级 | `ChatGPT-Account-ID` / `chatgpt-account-id` | — | 登录态 | 路由与账号一致性校验 |

四层可被服务端交叉关联,构成"设备↔安装↔会话↔账号"的完整关联图。

## 6.4 CLI 侧证明(x-oai-attestation)

codex 二进制含专用模块 `app-server/src/attestation.rs`:
- JSON-RPC 方法 `AttestationGenerate`(schema:`AttestationGenerateParams`)
- 产出"attestation envelope"(序列化失败有专用错误串 `failed to serialize app-server attestation envelope`)
- 以 **`x-oai-attestation`** 头随 agent 请求发送;另有 `x-codex-routing-hint` 路由提示
- (信封内部字段在 Rust 侧,静态字符串可确认机制存在;生成逻辑依赖运行时输入)

## 6.5 用户验证与已验证访问

```
GET/POST /user-verification/credentials
  头: x-openai-use-sheep: 1, OAI-Device-Id: <deviceId>, Content-Type: application/json
  → 响应 zod 解析 {credentials: [...]}
app-server: accounts/verified_access
```
这是账号级挑战/验证注册通道,带设备 ID。

## 6.6 远程控制身份(设备密钥)

```
/codex/remote/control/client/refresh/finish    requireDeviceIdentityHash: true
响应:{client_id, remote_control_token, …} → 头 x-codex-client-session-token: Bearer …
requiresDeviceKeyProof: true                    ← 设备密钥证明(硬件相关密钥)
```
设备侧配套原生模块:`remote-control-device-key.node`、`hid-topology-watcher.node`、`serial_control.node`(@worklouder 设备套件)—— 远程控制/串行控制需设备密钥与 HID 拓扑感知。

## 6.7 审核与安全通知(服务端→客户端)

`turn/moderationMetadata`(轮次审核元数据)、`model/verification`(模型验证)、`model/safetyBuffering/updated`(安全缓冲)、`guardianWarning`(守护警告)、`model/rerouted`(模型重路由)—— **内容审核与限流决策全部在服务端**,客户端仅消费通知并渲染(如安全缓冲的"生成中暂停"状态)。

## 6.8 网络行为管控(客户端侧)

- `applicationNetwork.assertAllowed(url)`:所有请求过统一策略断言(对应启动需求检查;Linux 上 `codex-node-network-restricted` 开关可收紧网络)
- 启动网络自检失败 → `Desktop network requirements prevented startup` → `app.exit(1)`(拒绝启动)
- trace 出口强制 loopback(见 3.4)
- TLS:代码中大量证书 fingerprint 逻辑(证书校验/固定特征);`RegisterCertificate`/`BuildClientWithCustomCa`/`BuildClientWithSystemRoots`(Rust 侧自定义 CA 支持,企业代理场景)
- `X-OpenAI-Account-Routing-Override` 用户侧删除(防账号混淆攻击)

## 6.9 已排除的"疑似风控"项(逐一核实)

| 疑似项 | 实际性质 |
|---|---|
| `fraud`/`abuse` 字符串(src-DCNiwad_.js) | **内容政策分类表**(agent 安全提示词,含 A/S 分级),非风控 |
| `Sentinel` ×10 | `projectlessPathSentinel = '~'` 路径哨兵值 |
| `suspended` ×53 | 渲染进程挂起/采样器状态;无账号封禁客户端逻辑 |
| `/var/lib/dbus/machine-id`、`/etc/machine-id` | **zbus(D-Bus 库)**读取机器 ID(桌面通知依赖),非硬件指纹上报 |
| `trust_level` | 本地**文件夹信任**(VSCode 式),持久化 `projects."<path>".trust_level`,与账号无关 |
| `secondary rate limit`/`abuse detection mechanism` | 识别 **GitHub** 限流错误(git push 失败处理) |
| `keylog`/监控类 | 未发现;屏幕共享走 WebRTC(语音/远程 PIP),截图仅 CUA 沙箱 API |

**结论**:客户端不本地判定风控;身份采集与证明在客户端(DeviceCheck/attestation/stableID/installation-id/oai-did),**判定全部服务端**(X-OAI-IS 状态机 + 审核通知 + Statsig 开关下发)。macOS 有 Apple 硬件级证明;Linux 无 DeviceCheck(仅 darwin 路径加载 `devicecheck.node`),信号面明显更弱。

---

# 第七部分 安全机制补充(与网络/风控相关)

- **权限模型**:默认全拒绝(`setPermissionCheckHandler(()=>!1)`);MCP 应用沙箱白名单(`mcp_app_sandbox.permission_blocked` 日志);同源 `clipboard-sanitized-write`
- **MCP 应用沙箱**:guest 仅 `web-sandbox.oaiusercontent.com`(及子域)+ `codex-sandbox://` 协议;无端口/用户名/密码;initId 格式 `^[A-Za-z0-9_-]{1,128}$`;API:navigate/setTheme/setWidgetData/**runWidgetCode**/**runUserCode**/screenshot/applyAutofillCandidate/prepareEnvironment/stop
- **消息边界**:chunked 协议标记 `codex-host-chunked-message-v1` + transferId 校验;MessagePort 握手 `connect-app-host`
- **插件完整性**:`plugin-signatures/openai-bundled/*` —— RSA_PKCS1_SHA256 签名 + `public-key.der`,`plugin.signature.json` + `plugin.tar.gz`
- **远程 PIP 串流**:主进程推编码视频块 → preload WebCodecs `VideoDecoder`(prefer-hardware, optimizeForLatency)→ 页面;原始帧不落盘
- **FedRAMP**:`X-OpenAI-Fedramp: true` 政企部署支持

---

# 附录 A 版本与构建指纹

```
CFBundleShortVersionString 26.1007.71326        deb Version: 26.1007.71326
CFBundleVersion 20398                          buildFlavor: prod
SCMRevision cfaadc5a132d78e1828635aa8405a499f3e14864-refs/branch-heads/8059@{#936}
Electron Chromium branch-heads/8059            runtimeName: owl
packagedFrom: /home/runner/work/minirepo/minirepo/codex/codex-apps/electron/out/ChatGPT-linux-arm64
runtimeArchiveSha: 6c92e64f… (owl runtime)
cua_node: v24.21.0-cua.1 / cua-node-0.0.30-20261008210730-dff8b428775e
Sparkle 公钥(SUPublicEDKey): mNfr1v9t63BfgDtlw4C8lRvSY6uMggIXABDOCi3tS6k=
Windows 商店: 9N8CJ4W95TBZ(beta) / 9PLM9XGG6VKS(prod)
主二进制 Mach-O:arm64,SDK 26.0,Min macOS 13.0,hardened runtime
codex CLI:Rust 静态链接 aarch64(GitHub Actions /home/runner/work/codex 构建,stripped)
codex-code-mode-host:Rust 1.95.0,tokio,自定义 CA HTTP 客户端,内含 V8 引用
```

# 附录 B 分析过程与 REA 工具修复记录

1. dmg 直接分析被拒(archive target + 无 Ghidra/Hopper)→ 挂载后走 JS 路径
2. macOS asar 因 permissions.node 完整性不匹配被拒(fail-closed)→ 定性为构建后处理(见 1.1)
3. Linux 全量 asar:默认 4GB 堆 OOM → 12GB 后完成解析,但结果校验崩溃
4. **修复 REA bug #1**:空字符串 endpoint 触发 Zod 崩溃 → `JavaScriptArtifactGraphFindings.js` addEndpoint 增加 `if (!input.value.value) return;`
5. **修复 REA bug #2**:巨型语义图 canonical JSON 超 V8 字符串上限(`RangeError: Invalid string length`)→ `javascriptSemanticGraph.js`/`javascriptApplicationGraph.js` 的 `digestCanonical` 改为流式 sha256(字节流与 canonicalize 输出一致,承诺哈希不变)
6. 主进程子集(30MB)图构建需 >8GB;`.vite/build` 子集(9.9MB,35 文件)16GB 堆下 17.7 分钟完成:62 IPC 通道、24 handler、6 窗口、3 bridge、170 万语义节点/174 万关系
7. 官方 CLI 最终输出对巨图 JSON.stringify 仍会失败 → 直接调用 `reconstructJavaScriptArtifact` 选择性提取(脚本 `/tmp/rea_extract.mjs`)
8. 原文件备份:`.bak`/`.bak2`;补丁使 REA 更鲁棒,不影响其他目标分析

# 附录 C 证据文件位置

```
/Volumes/移动/一些资料文档/codexapprea/deb_extracted/     ← deb 完整解包(外置盘)
/tmp/codex_mainproc/           ← asar 主进程子集(package.json+.vite+node_modules)
/tmp/codex_vite/               ← .vite/build 子集(REA 成功分析集)
/tmp/codex_asar/               ← main.js / startup-requirements.js / build-flavor.js 提取件
/tmp/codex_vite_rea_*.json     ← REA 重建证据(core/ipc-channel/ipc-handler/browser-window/
                                 context-bridge-api/storage/worker/graphoverview/semanticoverview)
/tmp/sig.json /tmp/plist.json /tmp/macho.json  ← 签名/plist/Mach-O 证据
/tmp/rea_extract.mjs             ← 选择性提取脚本
本报告:ChatGPT_Codex_桌面端_深度分析报告.md
```

*原主体结束。附录 D 为本次源码复查与请求还原补充；仍不宣称实时上游验证。*


---

# 附录 D 请求头还原、值来源与项目完整性对照（2026-10-14 补充）

## D.1 上一轮是否发送了“固定真实值”

没有。上一轮使用固定 fixture 和本地 mock upstream，没有使用真实 OAuth 凭据、DeviceCheck token、
attestation envelope 或真实 X-OAI-IS 请求 OpenAI。构造检查、mock HTTP 200、代码字符串命中，都不证明
真实服务器已接受这些值。此次仍是静态源码还原与项目缺口核对。

固定公开字面值（如 `OAI-Product-Sku: CODEX`、`x-openai-use-sheep: 1`）可直接从源码确认。
真实 installation UUID/device ID 首次生成后持久；session/thread/turn/timestamp 绑定当前上下文；
OAuth/DeviceCheck/attestation/integrity/remote-control token 有签发、更新、有效期和身份归属。
后两类不能换成固定占位字符串后声称“还原完成”。真实请求应使用自己获授权的账号/安装上下文。
报告与日志对 token/cookie 只保留脱敏形状、长度/摘要和结果，不保存原始值。

## D.2 补查确认的更正

1. Electron surface 默认 originator 是 `Codex Desktop`，不是推测的 `codex-desktop`。
   UA 工厂是 `Codex Desktop/<appVersion> (<platformLabel>; <arch>)`。
   此构建 macOS arm64 为 `Codex Desktop/26.1007.71326 (Mac OS; arm64)`，
   Linux arm64 为 `Codex Desktop/26.1007.71326 (X11; Linux; arm64)`。
   `CodexBrowser` 是 browser scoped prefix；完整 browser runtime UA 尚未恢复。surface/browser/engine 分开。
2. `X-OpenAI-Expected-Account-Id` 与四个 `X-OpenAI-Attach-*` 是本地控制字段，源码读取后删除。
   expected account 用于发送前校验；它们不是应直接补到上游的请求头。
3. DeviceCheck 实际附加 `x-sentinel-dc: {"token":"<DEVICECHECK_ENVELOPE>"}`，同时重建
   `OAI-Package-Name: <BUNDLE_ID>`。cookie 注册 body 是 `bundle_id/device_token`；不能遗漏这条生成链。
4. `X-OAI-IS` 格式检查为 `^ois1\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`，
   非空、无首尾空白、长度最多 2048。`X-OAI-IS-Update` 是响应头；当前状态等于发出时的 expectedState
   才接受更新。语法通过不证明服务器接受或密码学有效性；它不是 x-codex-turn-state 的别名。
5. desktop GET helper `Wr` 已确认固定 `OAI-Product-Sku: CODEX`，不自动等同
   `X-OpenAI-Product-Sku`。后者仍需自己的设置点证据。
6. user-verification 的 device 参数存在时 GET/POST 都可附加 `OAI-Device-Id`，并非只能 POST。
7. 本地/SSH app-server 连接读取握手响应的 `x-codex-websocket-max-unfragmented-message-bytes`，
   回退上限 16777216。不能仅凭字符串目录把它当作每次模型请求必发头。
8. `x-openai-subagent-request` 与项目 `x-openai-subagent` 拼写差异保留。metadata 符号存在不证明
   `session_id/thread_id/turn_id/window_id/context_window_id/agent_name` 全是独立 HTTP header。

## D.3 逐项头部清单

原报告显式 41 种头部拼写全部收录，额外补入源码确认的控制头、DeviceCheck 实际头和项目现有协议头。
HTTP 名称大小写不敏感，后缀不同的名字不自动视为 alias。
JSON 中 `project_symbol_occurrence_refs` 只是源码出现点；发送结论以实际构造函数和表中“项目现状”为准。

| ID | 原样名称 | 方向 | 端点/调用面 | 值来源与模板 | 项目现状 |
| --- | --- | --- | --- | --- | --- |
| H01 | `Authorization` | request | 模型/desktop/remote-control | `Bearer <ACCESS_TOKEN>；remote-control 用自己的 <REMOTE_CONTROL_TOKEN>` | 认证已构造；模型 token 与 remote-control token 分开 |
| H02 | `chatgpt-account-id` | request | 模型/账号 API | `<ACCOUNT_ID>；token account claim` | 凭据账号构造；shadow 使用母账号；已知矛盾拒绝 |
| H03 | `Content-Type` | request | JSON/OTel/OAuth | `JSON 为 application/json；token exchange 为 form-urlencoded` | 模型/透传 JSON 已归一；OAuth form 独立 |
| H04 | `User-Agent` | request | surface/engine/browser | `surface 为 Codex Desktop/26.1007.71326 (Mac OS; arm64)；engine=<ENGINE_UA>；browser=<BROWSER_RUNTIME_UA>` | 模型用配对 engine profile；surface UA 不能当模型实测 UA |
| H05 | `originator` | request | surface/engine/browser | `surface: Codex Desktop；engine=<ENGINE_ORIGINATOR>；browser: Codex Browser` | 模型配对 UA；原报告 codex-desktop 不能覆盖已证实 surface 默认值 |
| H06 | `OAI-Device-Id` | request_conditional | user-verification | `<DEVICE_ID>；helper device 参数存在即附加，GET/POST 均可` | 模型不发送；尚无 user-verification 注册链 |
| H07 | `oai-did` | request | browser/device cookie | `<DEVICE_ID>；设备管理器持久身份` | 不是 installation UUID 的已证实 wire 别名；模型不发送 |
| H08 | `OAI-Language` | unresolved_or_quota_request | CLI 符号/项目 quota | `<LOCALE>；真实 locale；项目 quota 为 zh-CN` | quota 已构造；模型未见构造；quota 的值不能算模型覆盖 |
| H09 | `OAI-Package-Name` | request_conditional | DeviceCheck attachToken | `<BUNDLE_ID>；报告 com.openai.codex` | 与 x-sentinel-dc 同步附加；模型不发送 |
| H10 | `OAI-Product-Sku` | request | desktop GET helper Wr | `CODEX；已确认源码固定公开字面值` | 当前模型/quota 未见构造；已登记缺口 |
| H11 | `X-OpenAI-Product-Sku` | unresolved | CLI strings | `<PRODUCT_SKU>；具体来源待确认` | 不可默认为 OAI-Product-Sku 的同值别名；未见模型构造 |
| H12 | `OAI-Sites-ID-Token` | request_conditional | 支持的 HTTPS Site | `<SITE_ID_TOKEN>；session_bootstrap 签发` | 不是模型 OAuth 头；Site flow 独立 |
| H13 | `sec-ch-ua` | request | Codex Browser | `<CHROMIUM_CLIENT_HINTS>；browser runtime` | 完整真实值尚未抓包；Go engine 不伪造 browser hints |
| H14 | `sec-ch-ua-mobile` | request | Codex Browser | `?0；桌面 browser 场景` | 普通模型不发送 |
| H15 | `sec-ch-ua-platform` | request | Codex Browser | `<BROWSER_PLATFORM>；实际 Chromium 平台` | 普通模型不发送；不得与 UA 平台冲突 |
| H16 | `x-client-request-id` | request | 模型 HTTP/WS | `<CLIENT_REQUEST_ID>；同 request plan` | 已构造；full 为 thread ID，旧 session 模式为 turn ID |
| H17 | `x-codex-client-id` | unresolved | remote-control client | `<CLIENT_ID>；client enrollment/refresh` | 项目已有 server 角色协议，不能算 desktop client 已覆盖 |
| H18 | `x-codex-client-session-token` | request | remote-control client 后续请求 | `Bearer <REMOTE_CONTROL_TOKEN>；refresh/finish 返回` | 模型不发送；server 角色的 Authorization 不是它的替代 |
| H19 | `x-codex-git-source` | unresolved | desktop git 来源 | `<GIT_SOURCE>；真实 git 操作分类` | 模型白名单未发送；需逐调用点确认 |
| H20 | `x-codex-installation-id` | request_or_metadata | 模型/Compact/remote-control | `<INSTALLATION_ID>；真实持久安装 UUID；项目账号 device 值或稳定派生` | 普通/device/session 可直接头；full 普通 Responses/WS 只有 body，Compact 可直接头；明确 wire 差异 |
| H21 | `x-codex-parent-thread-id` | request_conditional | 子线程模型 | `<PARENT_THREAD_ID>；同 principal lineage` | 有来源时重建，无父线程省略 |
| H22 | `x-codex-protocol-version` | request | remote-control/app-server 协议 | `<PROTOCOL_VERSION>；独立协议协商` | server WS 已发送 3；不得外推其他角色/模型 |
| H23 | `x-codex-routing-hint` | request | 模型 HTTP/WS | `model=<MODEL>[;tier=<TIER>]；最终模型和有效 tier` | 已生成 model 与 priority/flex；未扩展任意 tier |
| H24 | `x-codex-subscribe-cursor` | request_conditional | remote-control subscription | `<SUBSCRIBE_CURSOR>；真实订阅进度` | server WS 可选生成；模型不发送 |
| H25 | `x-codex-turn-metadata` | request_and_metadata | 模型 HTTP/WS | `<TURN_METADATA_JSON>；当前轮次的一份投影` | 已有归一/投影；28 项实际 provenance 另表，不能算真实桌面采集 |
| H26 | `x-codex-turn-state` | request_and_metadata | 模型 HTTP/WS | `<TURN_STATE>；opaque 上游状态` | 有观察/回带；配置随机池是有意差异，不等于同客户端真实状态连续性 |
| H27 | `x-codex-websocket-max-unfragmented-message-bytes` | handshake_response | 本地/SSH app-server RPC | `<MAX_UNFRAGMENTED_BYTES>；握手响应；回退 16777216` | bundle 已证实读响应；不能补成每次模型必发头 |
| H28 | `x-codex-window-id` | request_and_metadata | 模型窗口 | `<WINDOW_ID>；同窗口来源` | session/full 可生成；普通 OAuth 按既有可信投影边界 |
| H29 | `x-codex-ws-stream-request-start-ms` | metadata | 模型 WS 每轮流 | `<WS_REQUEST_START_MS>；真实帧发送时刻` | 仅见兼容键定义，未见时间生成；明确未完成项 |
| H30 | `x-oai-attestation` | request_conditional | Live / CLI attestation | `<ATTESTATION_ENVELOPE>；Live macOS provider 或未来 AttestationGenerate 输出` | Live 路径已有 macOS provider、加密持久化和 header 发送；普通 Codex CLI 的 AttestationGenerate 仍未接入，不能把 Live envelope 冒充 CLI proof |
| H31 | `X-OAI-IS` | request_conditional | attachIntegrityState desktop HTTP、OAuth HTTP/透传/直接 WS handshake | `<INTEGRITY_STATE>；服务端签发；ois1.A.B.C；最多 2048 字符` | 已加入账号隔离回带；入站伪造值删除，只有当前账号保存状态才发送；不是 x-codex-turn-state 别名 |
| H32 | `X-OAI-IS-Update` | response | 上述 desktop HTTP、OAuth HTTP/透传/直接 WS handshake 响应 | `<NEXT_INTEGRITY_STATE>；响应签发；expectedState CAS` | 已加入合法格式检查与 expected-state CAS；无真实服务端状态时不会生成占位值 |
| H33 | `x-oai-request-id` | unresolved | CLI strings | `<REQUEST_ID>；方向/调用点待确认` | 模型未见构造；不默认等同 x-client-request-id |
| H34 | `X-OpenAI-Account-Routing-Override` | trusted_request | workspace routing | `<ROUTING_OVERRIDE>；可信 backend auth/discovery` | 客户端值删除；服务端结果可重建，不是一律不发 |
| H35 | `X-OpenAI-Aeon-Messaging-Trace` | local_control | desktop messaging tracing | `<TRACE_FLAG>；主进程读取后删除` | 触发 trace 处理；不是每次模型必须发送的线缆头 |
| H36 | `X-OpenAI-Allow-Environmentless-Fallback` | unresolved | CLI strings | `<ENVIRONMENTLESS_FALLBACK>；调用条件/值待确认` | 当前模型未见构造，保留差异 |
| H37 | `X-OpenAI-Expected-Account-Id` | local_control | performDesktopFetch 入站 | `<EXPECTED_ACCOUNT_ID>；消费后本地比较` | 不是上游必发头；完整 principal 还涉及 userId |
| H38 | `X-OpenAI-Fedramp` | request_conditional | FedRAMP account API | `true；真实 token/account claim 为 true` | 按账号发送，否则删除；不可固定 true |
| H39 | `x-openai-internal-codex-responses-lite` | request_and_metadata | Responses Lite | `<RESPONSES_LITE_FLAG>；实际协议选择` | 项目按 handshake/body 重建；普通请求不强开 |
| H40 | `x-openai-request-id` | response_or_unresolved | 响应 tracing/CLI strings | `<UPSTREAM_REQUEST_ID>；bundle 证实读响应` | 不凭字符串要求模型请求必须发送 |
| H41 | `x-openai-subagent-request` | unresolved | CLI strings | `<SUBAGENT_REQUEST_MARKER>；类型/值未证实` | 项目是 x-openai-subagent；后缀不同，未证实 alias |
| H42 | `x-openai-use-sheep` | request | user-verification | `1；固定公开字面值` | 普通模型不发送；user-verification flow 未建立 |
| H43 | `Accept` | request | Responses/Compact/manifest | `Responses=text/event-stream；Compact/manifest=application/json` | 模型已归一；其他端点依其协议 |
| H44 | `session-id` | request | engine HTTP/WS | `<SESSION_ID>；真实 session，项目 principal 派生` | 已构造；full 与 thread/prompt_cache_key 关联 |
| H45 | `thread-id` | request | engine HTTP/WS | `<THREAD_ID>；真实 thread` | 已构造；不得给无关系的线程共用无来源固定值 |
| H46 | `session_id` | legacy_request_and_metadata | 报告符号/项目兼容 | `<SESSION_ID>；client_metadata/turn JSON` | 普通兼容可发 underscore；full 不发；不能忽略 hyphen 差异 |
| H47 | `conversation_id` | legacy_request | 项目兼容 | `<CONVERSATION_ID>；账号/用户隔离` | 项目兼容项；报告没证明 agent 必需 |
| H48 | `version` | request | 项目 engine profile | `<ENGINE_VERSION>；UA 首版本` | 项目已配对；不可拿 surface appVersion 当通用 engine version |
| H49 | `OpenAI-Beta` | request_conditional | WS/quota | `v2=responses_websockets=2026-02-06；quota=codex-1` | 项目现状；普通 HTTP 去除旧 responses=experimental；非真实新客户端 capture |
| H50 | `x-codex-beta-features` | request_conditional | native compact/模型 | `<BETA_FEATURES>；实际 feature set` | 项目已有 remote compaction v2 策略，新增协议不能漏记 |
| H51 | `x-openai-subagent` | request_and_metadata | 项目 subagent | `<SUBAGENT_KIND>；真实当轮语义` | 按语义重建，不能宣称等同 report 的 -request |
| H52 | `x-codex-guardian` | request_conditional | guardian | `reviewer/classifier；匹配 subagent/source/trigger` | 项目按条件重建 |
| H53 | `x-openai-memgen-request` | request_conditional | memory consolidation | `true；一致的 kind/source/trigger` | 项目按完整语义条件附加 |
| H54 | `x-responsesapi-include-timing-metrics` | request_conditional | 项目 timing | `<TIMING_METRICS_FLAG>；调用参数` | 项目白名单透传；不是 desktop 硬件证明 |
| H55 | `x-sentinel-dc` | request_conditional | challenge 或 POST messaging | `{"token":"<DEVICECHECK_ENVELOPE>"}；真实 DeviceCheck output` | 新补查的实际 wire 头，原报告漏列；模型不发送 |
| H56 | `Cookie` | request_conditional | surface/device cookie/Site | `<COOKIE_JAR_VALUES>；Electron session 管理` | desktop credentials include 行为；Go engine 不等同 browser cookie jar |
| H57 | `traceparent` | request_conditional | messaging tracing/OTel | `<TRACEPARENT>；同一 W3C trace context` | bundle 条件附加；metadata 兼容键不等于完整实际 trace 传播 |
| H58 | `tracestate` | request_conditional | 同上 | `<TRACESTATE>；同一 trace context` | 有 context 值才发送；project wire 传播尚需核对 |
| H59 | `X-Codex-Surface` | unresolved | renderer/desktop 符号 | `<SURFACE>；读写条件未完整确定` | 已定位常量，不构成所有模型必发头的证据 |
| H60 | `X-OpenAI-Attach-Auth` | local_control | performDesktopFetch 入站 | `1/true 开启，0/false 关闭；读取删除` | 本地控制，不发送上游 |
| H61 | `X-OpenAI-Attach-Desktop-Surface` | local_control | 同上 | `1/true；读取删除` | 触发按需 originator/User-Agent 补齐 |
| H62 | `X-OpenAI-Attach-DeviceCheck-Token` | local_control | 同上 | `1/true；读取删除` | 通过 surface/允许端点检查后附加真实 x-sentinel-dc |
| H63 | `X-OpenAI-Attach-Integrity-State` | local_control | 同上 | `1/true；读取删除` | 触发当前有效 X-OAI-IS 回带 |
| H64 | `x-codex-server-id` | request | 项目 remote-control server | `<SERVER_ID>；server enrollment` | server 已构造；不是 client_id |
| H65 | `x-codex-name` | request | 同上 | `base64(<SERVER_NAME>)` | server 已构造 |
| H66 | `x-codex-host-device-kind` | request_conditional | remote-control server | `<HOST_DEVICE_KIND>；真实宿主` | server 可选生成；不能当硬件证明 |
| H67 | `x-openai-internal-codex-residency` | unresolved | 项目保留过滤项 | `<RESIDENCY>；真实 server selection 未知` | full 清理，未确认应如何可信重建 |

## D.4 每轮 metadata 的 28 项还原

这是 body/JSON 元数据，不当成 28 个独立 HTTP header。每一项保留真实来源 slot 与项目差异，
后续证据精确填入；“清理过”或“字段名称存在”都不算完整还原。

| 字段 | 位置/真实来源 | 项目现状 |
| --- | --- | --- |
| `session_id` | body/turn JSON；`<SESSION_ID>` | 有 request/identity plan 投影；逐 mode 说明来源，不等于真实 desktop 采集 |
| `thread_id` | body/turn JSON；`<THREAD_ID>` | 有 request/identity plan 投影；逐 mode 说明来源，不等于真实 desktop 采集 |
| `turn_id` | body/turn JSON；`<TURN_ID>` | 有 request/identity plan 投影；逐 mode 说明来源，不等于真实 desktop 采集 |
| `window_id` | body/turn JSON；`<WINDOW_ID>` | 有 request/identity plan 投影；逐 mode 说明来源，不等于真实 desktop 采集 |
| `window_number` | body/turn JSON；`<WINDOW_NUMBER>` | 有 request/identity plan 投影；逐 mode 说明来源，不等于真实 desktop 采集 |
| `context_window_id` | body/turn JSON；`<CONTEXT_WINDOW_ID>` | 有 request/identity plan 投影；逐 mode 说明来源，不等于真实 desktop 采集 |
| `agent_name` | body/turn JSON；`<AGENT_NAME>` | 在保留/额外字段规则中逐 mode 核对；尚无真实 desktop 运行态采集证据，不计为完整实现 |
| `turn_started_at_unix_ms` | body/turn JSON；`<TURN_STARTED_AT_UNIX_MS>` | 有 request/identity plan 投影；逐 mode 说明来源，不等于真实 desktop 采集 |
| `request_kind` | body/turn JSON；`<REQUEST_KIND>` | 有 request/identity plan 投影；逐 mode 说明来源，不等于真实 desktop 采集 |
| `compaction` | body/turn JSON；`<COMPACTION>` | 在保留/额外字段规则中逐 mode 核对；尚无真实 desktop 运行态采集证据，不计为完整实现 |
| `code_mode_tool_names` | body/turn JSON；`<CODE_MODE_TOOL_NAMES>` | 当前 forbidden turn metadata 项；明确差异，待新 runtime 证据决定位置 |
| `tool_namespaces_info` | body/turn JSON；`<TOOL_NAMESPACES_INFO>` | 在保留/额外字段规则中逐 mode 核对；尚无真实 desktop 运行态采集证据，不计为完整实现 |
| `forked_from_thread_id` | body/turn JSON；`<FORKED_FROM_THREAD_ID>` | 按存在的 lineage/subagent 语义保留/重建；无来源不能制造拓扑 |
| `forked_from_ordinal_exclusive` | body/turn JSON；`<FORKED_FROM_ORDINAL_EXCLUSIVE>` | 按存在的 lineage/subagent 语义保留/重建；无来源不能制造拓扑 |
| `parent_turn_id` | body/turn JSON；`<PARENT_TURN_ID>` | 按存在的 lineage/subagent 语义保留/重建；无来源不能制造拓扑 |
| `root_turn_id` | body/turn JSON；`<ROOT_TURN_ID>` | 按存在的 lineage/subagent 语义保留/重建；无来源不能制造拓扑 |
| `subagent_kind` | body/turn JSON；`<SUBAGENT_KIND>` | 按存在的 lineage/subagent 语义保留/重建；无来源不能制造拓扑 |
| `thread_source` | body/turn JSON；`<THREAD_SOURCE>` | 在保留/额外字段规则中逐 mode 核对；尚无真实 desktop 运行态采集证据，不计为完整实现 |
| `turn_trigger` | body/turn JSON；`<TURN_TRIGGER>` | 在保留/额外字段规则中逐 mode 核对；尚无真实 desktop 运行态采集证据，不计为完整实现 |
| `sandbox` | body/turn JSON；`<SANDBOX>` | 在保留/额外字段规则中逐 mode 核对；尚无真实 desktop 运行态采集证据，不计为完整实现 |
| `sandbox_mode` | body/turn JSON；`<SANDBOX_MODE>` | 在保留/额外字段规则中逐 mode 核对；尚无真实 desktop 运行态采集证据，不计为完整实现 |
| `auto_review_enabled` | body/turn JSON；`<AUTO_REVIEW_ENABLED>` | 在保留/额外字段规则中逐 mode 核对；尚无真实 desktop 运行态采集证据，不计为完整实现 |
| `node_repl_auto_review_required` | body/turn JSON；`<NODE_REPL_AUTO_REVIEW_REQUIRED>` | 在保留/额外字段规则中逐 mode 核对；尚无真实 desktop 运行态采集证据，不计为完整实现 |
| `node_repl_disabled` | body/turn JSON；`<NODE_REPL_DISABLED>` | 在保留/额外字段规则中逐 mode 核对；尚无真实 desktop 运行态采集证据，不计为完整实现 |
| `workspaces` | body/turn JSON；`<WORKSPACES>` | 项目结构脱敏路径/remote 凭据；与 desktop 原始 workspace 明确有差异 |
| `history_ingest_requested` | body/turn JSON；`<HISTORY_INGEST_REQUESTED>` | 在保留/额外字段规则中逐 mode 核对；尚无真实 desktop 运行态采集证据，不计为完整实现 |
| `analytics_enabled` | body/turn JSON；`<ANALYTICS_ENABLED>` | 在保留/额外字段规则中逐 mode 核对；尚无真实 desktop 运行态采集证据，不计为完整实现 |
| `responses_api_metadata` | body/turn JSON；`<RESPONSES_API_METADATA>` | 报告限制 16 键/值 128 字节；project bounded extras 不等于同一字段完整实现 |

## D.5 逐端点请求模板

以下是还原模板，不是一次真实 capture。尖括号是 typed slot；条件头只在实际场景加入。
未知 HOST/METHOD/PATH 保留槽位。Host/Content-Length、HTTP2 伪头/顺序、TLS/ALPN/h2 SETTINGS
由具体 transport 决定，不能从应用字符串清单证明全部与官方一致。

### 1. Rust engine Responses HTTP

```http
POST /backend-api/codex/responses HTTP/1.1
Host: chatgpt.com
Authorization: Bearer <ACCESS_TOKEN>
ChatGPT-Account-Id: <ACCOUNT_ID>
User-Agent: <ENGINE_UA>
originator: <ENGINE_ORIGINATOR>
Content-Type: application/json
Accept: text/event-stream
session-id: <SESSION_ID>
thread-id: <THREAD_ID>
x-client-request-id: <CLIENT_REQUEST_ID>
x-codex-turn-metadata: <TURN_METADATA_JSON>
x-codex-routing-hint: model=<MODEL>[;tier=<TIER>]
```

installation/window/parent-thread/subagent/turn-state/attestation/Lite/beta/FedRAMP 逐项按实际 client 与当轮条件决定。
full 模式的直接 installation 目前只有 body 投影；不得称这张模板在所有模式都已完整还原。
body 绑定真实 `model/input/instructions/stream/store/client_metadata`；Lite、compact、私有 Codex 与公开 API
需要同一次请求证据。报告的 `/backend-api/codex` 是调用面/base，不是所有请求的完整 leaf path。

### 2. Compact 与模型列表

```http
POST /backend-api/codex/responses/compact HTTP/1.1
Host: chatgpt.com
Authorization: Bearer <ACCESS_TOKEN>
ChatGPT-Account-Id: <ACCOUNT_ID>
User-Agent: <ENGINE_UA>
originator: <ENGINE_ORIGINATOR>
Content-Type: application/json
Accept: application/json
session-id: <SESSION_ID>
thread-id: <THREAD_ID>
```

manifest 是独立 `GET /backend-api/codex/models?<MANIFEST_QUERY>`，参数按实际客户端版本请求生成，使用 JSON Accept。
不复制全部 Responses SSE 头。native compaction v2 与 unary /responses/compact 也分别记录。

### 3. Responses WebSocket

```http
GET /backend-api/codex/responses HTTP/1.1
Host: chatgpt.com
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Key: <WEBSOCKET_HANDSHAKE_KEY>
Sec-WebSocket-Version: 13
Authorization: Bearer <ACCESS_TOKEN>
ChatGPT-Account-Id: <ACCOUNT_ID>
User-Agent: <ENGINE_UA>
originator: <ENGINE_ORIGINATOR>
session-id: <SESSION_ID>
thread-id: <THREAD_ID>
OpenAI-Beta: <WS_PROTOCOL_NEGOTIATION>
x-codex-turn-metadata: <TURN_METADATA_JSON>
```

标准 WS 头由 dialer 生成。后续轮次是 `response.create` JSON，不能假定每帧重发 HTTP 头。
`previous_response_id` 回到正确账号/原连接；prewarm `generate:false` 不等于生成模型输出。
项目 v2 negotiation 当前为 `responses_websockets=2026-02-06`，这是项目现状，非本构建实时 capture。
WS stream start-ms 必须是真正发帧时刻，当前缺生成链。固定 timestamp 不是还原。

### 4. Electron desktop surface 账号 API

```http
<METHOD> /backend-api/<DESKTOP_API_PATH> HTTP/1.1
Host: chatgpt.com
Authorization: Bearer <ACCESS_TOKEN>
ChatGPT-Account-Id: <ACCOUNT_ID>
originator: Codex Desktop
User-Agent: Codex Desktop/26.1007.71326 (Mac OS; arm64)
```

surface helper 只在调用方未设置 originator/UA 时补齐；JSON body 按协议加 Content-Type。
启用 integrity 且当前有效才附加 `X-OAI-IS: <INTEGRITY_STATE>`；Electron 使用 credentials include/cookie manager。
发送前还重查 accountId/userId。platform/arch 必须与实际构建一致；不能把 macOS surface UA 固定复制到 Linux engine。

### 5. macOS messaging 与 DeviceCheck 注册

```http
POST /backend-api/messaging/rooms/<ROOM_ID>/messages HTTP/1.1
Host: chatgpt.com
Authorization: Bearer <ACCESS_TOKEN>
ChatGPT-Account-Id: <ACCOUNT_ID>
originator: Codex Desktop
User-Agent: <DESKTOP_SURFACE_UA>
Content-Type: application/json
x-sentinel-dc: {"token":"<DEVICECHECK_ENVELOPE>"}
OAI-Package-Name: com.openai.codex
Cookie: <DEVICE_COOKIE_JAR>
```

`/api/messaging/...` 是允许的同类路径变体。DeviceCheck 来自真实设备与相同 bundle context；
Linux 没有 darwin native 生成链，不能用固定字符串补成“真证明”。

```http
POST <PATH_ENDING_IN_/ios/attestation_challenge> HTTP/1.1
Host: <ALLOWED_OPENAI_HOST>
<REGISTRATION_HEADERS_FROM_DESKTOP_CONTEXT>
Content-Type: application/json

{"bundle_id":"com.openai.codex","device_token":"<DEVICECHECK_ENVELOPE>"}
```

注册使用 credentials include；取得真实 cookie 后由 cookie jar 发送。不要猜 cookie 名，
也不要把消息调用的全部头当成注册必需头。trace 开启时从同一次上下文加入 traceparent/tracestate。

### 6. 用户验证

```http
GET /user-verification/credentials HTTP/1.1
Host: <ACTUAL_BACKEND_HOST>
Authorization: Bearer <ACCESS_TOKEN>
ChatGPT-Account-Id: <ACCOUNT_ID>
Content-Type: application/json
x-openai-use-sheep: 1
originator: Codex Desktop
User-Agent: <DESKTOP_SURFACE_UA>
OAI-Device-Id: <DEVICE_ID>
```

device 参数不存在则省略 OAI-Device-Id；存在时 GET/POST 都可以附加。POST body 用真实 enrollment/verifier 结果。
helper timeout 15 秒；不以一个默认字符串替代 user-verification 能力。

### 7. OAuth code exchange 与 refresh

```http
POST /oauth/token HTTP/1.1
Host: auth.openai.com
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code&client_id=<CLIENT_ID>&code=<CODE>&redirect_uri=<REDIRECT_URI>&code_verifier=<PKCE_VERIFIER>
```

refresh 使用 grant_type=refresh_token、refresh_token、client_id；scope 按该客户端实际证据。
项目 scope 是 `openid profile email`，不能直接宣称是本构建所有 auth 请求的唯一格式。
PKCE S256、state、redirect/client_id 来自同次授权，不是统一固定 token。
原报告没有逐请求证明所有官方 auth UA/originator/body。deviceauth usercode/callback/token 也逐实际 leaf path 填槽。

### 8. Remote Control 的 client/server 两个角色

```http
GET /codex/remote/control/client/refresh/finish HTTP/1.1
Host: <ACTUAL_BACKEND_HOST>
Authorization: Bearer <ACCESS_TOKEN>
ChatGPT-Account-Id: <ACCOUNT_ID>
originator: Codex Desktop
User-Agent: <DESKTOP_SURFACE_UA>
<DEVICE_KEY_PROOF_HEADER>: <DEVICE_KEY_PROOF>
```

device identity hash/challenge proof 必须由实际 device key 产生；client_id、remote_control_token、expires_at、
scopes 与 enrollment 校验。已证实后续 client 头为
`x-codex-client-session-token: Bearer <REMOTE_CONTROL_TOKEN>`，并要求 device-key proof。
完整 proof 算法/头名仍是槽位，不用 fixture 冒充。

项目已有的 remote-control server WS 属于另一角色：

```http
GET <REMOTE_CONTROL_SERVER_WS_PATH> HTTP/1.1
Host: <ALLOWED_HOST>
Authorization: Bearer <SERVER_REMOTE_CONTROL_TOKEN>
x-codex-server-id: <SERVER_ID>
x-codex-name: <BASE64_SERVER_NAME>
x-codex-protocol-version: 3
x-codex-installation-id: <INSTALLATION_ID>
x-codex-host-device-kind: <HOST_DEVICE_KIND>
x-codex-subscribe-cursor: <SUBSCRIBE_CURSOR>
```

可选字段为空时省略；server Authorization 与 client-session-token 不互换。
本地 `ws://codex-app-server/rpc`、云 `wss://codex-cloud-backend.chatgpt.com` 与 Responses WS 不合并为一种协议。

### 9. 通知、quota、Site 与浏览器

通知 deregister/token 是 POST JSON，body 放 deregistration token；源码有只带 desktop surface 的调用，
不能默认附加账户 bearer。workspace route/FedRAMP 依据真实 token/route。`GET /api/accounts` 与
未知 /backend-api/... 逐实际 path/method 还原。

项目 `GET /backend-api/wham/usage` 的 quota 头为 OAI-Language=zh-CN、OpenAI-Beta=codex-1、
originator=Codex Desktop、Accept=application/json，另有 sec-fetch/priority；不自动视为模型必需头。

Site bootstrap 签发 OAI-Sites-ID-Token；源码限定无 port/userinfo 的 HTTPS，以及
`.chatgpt.site` 或 `.appgardenapps.dev` 主机后缀，不是对所有 OpenAI 域无条件认证。

Browser 的 oai-did、ChatGPT-Account-ID、originator=Codex Browser、sec-ch-* 来自 runtime。
完整 UA/Cookie/Origin/Referer/Sec-Fetch-* 尚需同一真实请求证据，当前保留槽位。

### 10. 遥测、公开账号接口、静态更新与代理

CES、Statsig、Sentry、OTel 各使用自己的 SDK/trace pipeline。CES dummy-token 是 SDK 占位值，不是 OAuth 真 token。
Statsig stableID/sessionID 是 body 字段，不是模型头。OTel 按配置开启，Content-Type JSON 与 trace context 按真实请求生成。
api.openai.com/auth、/mfa、/profile、MASQUE 和更新/静态下载尚缺逐请求 method/header 证据，
登记 `<METHOD>/<SDK_HEADERS>/<UPDATE_HEADERS>/<MASQUE_HEADERS>`，不凭 endpoint 名猜测通用认证头。
遥测/更新/支付等保持各自作用域，不称为模型请求完整性的已验证依赖。

## D.6 项目仍需补齐的明确差异

| 缺口 | 下一份证据 | 完成条件 |
| --- | --- | --- |
| full 的直接 installation 头 | 同一 engine 的 HTTP/Compact/WS/body capture | 明确 placement 后修复对应 mode；body 中有字段不算 header 已发送 |
| 完整 principal 与发送前重查 | token/cache 的 accountId 与 userId、刷新/切换时序 | 不仅比较账号 claim；发送前再校验当前 principal，区分 opaque token 与 known claim |
| X-OAI-IS store/CAS/回带 | 初始请求、update、下一请求、并发/乱序、账号切换、跨副本共享缓存 | 本地 HTTP/透传/直接 WS 已具备真值保存、expectedState 检查和账号隔离；仍需真实上游证明服务端接受后继状态 |
| attestation | AttestationGenerate params/result、请求位置、生命周期 | 有真实来源和安装归属；清理后可信重建，不只是删除 |
| subagent marker | -request 与无后缀字段的 setter/同次 capture | 判明独立/等价/废弃；保持 body/header 同轮一致 |
| WS stream start-ms | 发帧代码点、metadata、事件 | 真发送时间关联当轮，不是固定 timestamp |
| SKU/fallback/request-id | 各拼写的设置点、方向、endpoint | 每行都有 send/consume/response/conditional 结论 |
| DeviceCheck/client remote-control | 真实允许端点、设备生成/签名输入、响应 | 已加入 raw DeviceCheck provider 接口、`x-sentinel-dc`/`OAI-Package-Name` builder 和两个端点 allow-list；仍需真实 macOS native/上游响应与 remote-control client proof |
| surface/browser/engine 与 transport | 三种完整 UA/headers、TLS/ALPN/h2/cookies | 同次请求的整组一致，不能只贴版本字符串 |
| 28 项 metadata provenance | 当前 thread/turn/tools/workspace/approval/analytics | 位置/类型/语义完整且来源可核对，不能用固定字面值填动态状态 |

这些是实在的未完成项，不记为“已有等价替代”。本轮先完善报告与清单，等待后续补充的实际协议证据。

## D.7 后续补充的记录格式

每条记录带 `capture_id / client_version / platform / surface_or_engine / endpoint / method /
HTTP_or_WS / request_headers / request_body / response_headers / response_status / relevant_events /
value_sources / redacted_secret_shapes / evidence_file_sha256`。
保留同账号/安装/会话的先后关系；header 名称原样记录，明确方向，不能合并不同端点。
新证据更新同一 Hxx 与 metadata 行，新增项追加 ID，复用本任务同一四角色交付文件。

机器清单：[desktop-request-inventory.json](desktop-request-inventory.json)。
每份 bundle SHA256、字符 offset、UTF-8 byte offset 和源代码摘录保存在清单内；它是静态证据，不是网络 capture。

## D.8 静态证据索引

| Bundle | SHA256 | 确认内容 |
| --- | --- | --- |
| `bootstrap-DOZRFVdo.js` | `8602f1022fc835148268e8cdfcfb939d8ab5a969e4fd0b4141e264ce8ec4be4d` | `Gt=`Codex Desktop``、`function Xt({appVersion:`、`function on(e,t,{desktopOriginator:`、`function sn(e,{desktopOriginator:` |
| `main-CwxgTOle.js` | `87d52265eb13608a380fabcac47ecf13a39b981115c0960c18af563ea6d32d51` | `var db=`X-OAI-IS``、`function gb({currentState:`、`async attachToken(e)`、`async registerCookie(e)`、`storeResponseUpdate(y.headers,d)`、`headers:{"x-codex-client-session-token"`、`headers[bGe]`、`var gAe=3e3,_Ae=`OAI-Sites-ID-Token``、`function xb({request:e,headers:t` |
| `src-CP7XXlpZ.js` | `79623d50989f9efbcb8e5752c56b3cc608a0f5993c05112e03e11b718ac7a586` | `tc=`X-Codex-Surface``、`function fc(e){let t=hc(e,oc)`、`function pc(e,t){let n=hc(e,t)` |
| `src-DCNiwad_.js` | `86fb2066bbc2f7428864b76b46ef570b09eb1e92e81e9d6f089f5b048a66df25` | `ar=`CODEX`,or=`OAI-Product-Sku``、`function Wr(e){let t={Authorization:` |
| `early-bootstrap.js` | `036bde993c50a1a34d0f7a0ee4f3d57b5592b4cfe3b2a87b4022b263bf0367aa` | `value:`CodexBrowser`` |


# 附录 E 代码与真实账号验证（后续补充）

此附录更新附录 D 的项目状态；D 中“未接入”的历史文字不再代表当前候选。原始报告版本仍是 26.1007.71326；当前本机安装的是 26.1007.30912，CLI 0.162.0-alpha.17.2。它们的证据分别保存，未混同版本。

OAuth：用户提供的账号仅在内存中读取，没有导入数据库、修改原文件或主动轮换 refresh token。真实 wham/usage 返回 HTTP 200；候选通过实际 gateway/HTTPUpstream 完成非流式、SSE 请求，均 HTTP 200；真实 WS 请求也完成且 OpenAIWSMode=true。导出代理未取得响应，成功路径为直连。

attestation：已接入普通 HTTP、透传 HTTP、托管 WS、预热与直接 WS 的实际发送点，采用上游 attestation/generate 的 {v,s,t} 封装。Linux 回调携带 CBOR error_code=1；当前 macOS native 模块实际返回 unsupported device，回调携带 error_code=3。外层 s=0 表示回调成功返回 token，不能解释为 Apple 硬件证明通过。两次真实 HTTP 请求携带这一明确失败状态并获 HTTP 200；这证明请求可完成，不证明可信设备注册成功。超时、取消、回调失败、格式错误分别使用 s=1/3/2/4 且无伪造成功 t。

DeviceCheck：源码确认注册路径是 /backend-api/devicecheck（或对应 /api/devicecheck），不是 /ios/attestation_challenge。注册 body 使用 raw Apple token 的 bundle_id/device_token，成功响应应提供 _devicecheck cookie；messages/challenge 使用 x-sentinel-dc JSON 与 OAI-Package-Name。注册链已接入代码并通过本地 round-trip；同 principal/安装/origin 的 cookie 才可复用。当前 Mac 与 Linux 均未提供 raw Apple token，因此没有发送伪造真实注册请求。Linux lte 分支本身也返回 unsupported_platform；错误 CBOR attestation 不能作为 raw token 拿去注册。

X-OAI-IS：已将状态绑定到实际 send receipt、principal/安装/第一方 origin；缓存使用 Redis 原子 CAS，32 个并发副本争用只有 1 个更新获准。普通 HTTP、托管 WS/prewarm、直接 WS 的返回路径均消费 update。缓存故障不恢复 stale 本地 shadow 值，未绑定请求不接受更新；本地容量有限且有过期。真实模型请求本次没有提供 X-OAI-IS-Update，所以真实服务端后继状态闭环仍没有证据。

两个 proof 开关均默认关闭，配置位于 deploy/config.example.yaml：gateway.openai_oauth_attestation_enabled 与 gateway.openai_desktop_devicecheck_enabled。不开启时维持原协议；开启后生成值来自 provider 或真实平台不可用状态。模型 Responses 不会附加 DeviceCheck raw token，非第一方 relay 不继承 attestation/integrity 证明；第一方 proof 请求禁止自动跨域重定向。

受限原生执行已修正 headless NSScreen 无显示器时的信号读取、多显示器总尺寸和 latency Float64 编码；未通过伪造屏幕/硬件/签名绕过 native unsupported。支持设备或真实证明提供者是成功 Apple 注册的外部条件，不能靠固定字符串补齐。


# 附录 F Ghidra 原生层报告补充

新报告指向的设备密钥证明属于 controller Remote Control 会话，与模型 x-oai-attestation、Desktop DeviceCheck cookie 注册均为独立协议。当前代码已增加 native TPM/SE/Keychain provider、controller enroll/refresh、fresh step-up 检查、设备身份摘要和 WS device_key_proof。生产代码不使用本地软件 fixture 代替硬件密钥。实现与证据见 [原生证明对照](native-device-key-alignment.md)。

与同版本 bundle 交叉核对后，enroll/refresh start/finish 使用 POST；签名载荷为固定 domain 包装的有序 JSON；attestation 是 CBOR map，f 是 CBOR byte string；Windows error_code=1，Intel Mac error_code=2。报告对应表述以对照文档的源码证据修正。新增 sky.node/CUA 执行层分析没有提供新的网关请求字段。

本地 HTTP/WS 注册、刷新和签名验签通过；Go 的身份 hash、两种载荷与 CBOR 都按原 JS 纯函数生成 golden 逐字节比较。native 模块缺失时返回 error_code=4 与实际系统 signals，旧 Live/raw 路径继续拒绝；同输入基线/修改/回退结果记录在交付 ledger。此前真实 native hardware_only 与 allow_os_protected_nonextractable 两策略均未能创建 key，真实硬件成功和上游 fresh step-up 注册仍未验证。

## 附录 H：账号设备证明作用域更新（2026-10-16）

当前网关不实现多个桌面宿主的 app-server 注册，但会在账号选择后建立服务端拥有的证明作用域：credential owner、账号安装身份、origin 和已投影线程身份共同决定本次 provider、cookie 和 integrity receipt。Responses/Compact、透传与托管 WS、预热、DeviceCheck 注册和 Live 创建均从该作用域生成；换号、影子账号 owner、安装身份变化或 failover 不复用旧账号证明。入站 `x-codex-installation-id`、`thread-id` 和 `oai-did` 只作为待清理输入。

macOS scoped provider 只在进程内派生不同的 app session，并仍调用同一部署机上的 native module；它没有创造不同物理设备，也没有伪造 Apple DeviceCheck。当前真实 Apple proof、硬件 key 签名和上游 `X-OAI-IS-Update` 仍未验证成功。完整代码和验证边界见 [原生证明对照](native-device-key-alignment.md)。


## 附录 G：CLI attestation 反编译更新（2026-10-15）

新版 Ghidra 报告和现存 v1/v2 产物已复核：分别为 20 字符串/7 函数、49 地址/3 panic Location/2 函数。v2 的 `FUN_039d5bc8` 确认 attestation.rs 异步状态机、thread_state 联动与 Tokio timer；此前“0 命中/CLI 尚未完成”的记录已由此替代。

公开 Rust revision `3ee47c10513f1bb57d869ea650aba28638e3d11c` 交叉核对到固定 100ms、按线程选择支持 requestAttestation 的宿主、回调参数 `{}` 与 `{v,s,t?}` 封装；该源码与被分析二进制同 revision 尚未证明。`timeout_seconds` 是日志字段，不能据此添加秒级配置；`0x432edc0` 的相邻字符串也不证明 thread_id 是 callback 参数。未发现可支持新增 HMAC 的字段级证据。

本轮在现有本地 provider 架构中落实等待上限、取消、无 provider 省略头、opaque token 保留及有界并发。HTTP、Compact、WS dial 与预热仍在实际发送时生成证明；s=0 不证明 Apple token 有效。Sub2API 尚无 app-server 多宿主 capability/线程注册链，未据此声称实现完整桌面宿主协议。当前 Apple 证明、真实 integrity update 与硬件密钥验证边界不变。源码位置、哈希和本地验证入口见 [原生证明对照](native-device-key-alignment.md) 与 [证据清单](native-device-key-evidence.json)。
