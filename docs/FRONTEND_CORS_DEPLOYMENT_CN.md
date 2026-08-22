# 二开前端同源与 CORS 部署指南

本文面向把 Sub2API 前端单独构建、改名、放到 CDN，或嵌入其他控制台的维护者。目标是让浏览器只在确实需要时跨源，并且让跨源请求的来源、Cookie、预检、WebSocket 和静态资源行为保持一致。

## 先记住三条规则

1. **优先同源部署。** 让浏览器从一个 HTTPS 域名同时取得 HTML、`assets/*`、`/api/v1/*`、`/v1/*` 和 `/setup/*`。生产构建默认使用相对 API 地址 `/api/v1`，此时不需要为前端额外打开 CORS。
2. **分离部署才配置 CORS。** 前端和 API 使用不同源时，在后端 `cors.allowed_origins` 中填写前端的完整 Origin，并保留凭证配置；不要用 `*` 代替白名单。
3. **不要把 `null` 当成前端域名。** `Origin: null` 通常来自 `file://`、`data:`、没有 `allow-same-origin` 的 sandbox iframe，或浏览器扩展。把 `null` 加入白名单既不安全，也不能恢复 `localStorage`、Cookie 和 OAuth 所需的正常页面上下文。

当前实现的事实源是：`frontend/vite.config.ts`、`frontend/src/core/networks/url.ts`、`frontend/src/core/networks/client.ts`、`backend/internal/transport/http/server/middleware/cors.go` 和 `deploy/config.example.yaml`。文档中的路径和头部以这些源码为准，路由变更后应重新检查源码。

## 截图现象如何判断

截图中页面地址是正常的 HTTPS 管理页，但控制台同时出现 `origin 'null'`、静态 JS/CSS 被拦截、`localStorage` sandbox 错误，以及 `/cdn-cgi/rum`、`/cdn-cgi/speculation` 报错。它们不是一个单一的“后端少了一个响应头”问题：

| 现象 | 更可能的含义 | 正确处理 |
| --- | --- | --- |
| `from origin 'null'` | 页面处于 opaque origin，常见于 sandbox、`file://` 或扩展注入页 | 用顶层 `https://` 页面复现；检查 iframe 的 sandbox 和扩展，不要放行 `null` |
| 资源返回 `200` 但 `ERR_FAILED` / “blocked by CORS” | 服务器返回了内容，但浏览器拒绝把跨源响应交给页面 | 先修正页面来源或同源代理，再检查 API 的预检响应；`200` 本身不是浏览器可用的证明 |
| `SecurityError: localStorage ... sandboxed` | sandbox 没有 `allow-same-origin`，前端无法使用正常存储上下文 | 不要在 opaque iframe 中运行管理端；必要时改为可信同源 iframe 或独立顶层页面 |
| `/cdn-cgi/rum`、`/cdn-cgi/speculation` | Cloudflare 生成的遥测/预取请求，不是 Sub2API 登录前置接口 | 单独检查 Cloudflare 规则和边缘响应；不要把这些路径代理成业务 API |
| `preloaded ... but not used` | 预加载提示，通常是前面的脚本加载失败后的次生现象 | 先修复资源来源和加载链路，最后再处理 preload |

`/cdn-cgi/rum` 出错时，仍应直接探测 `/api/v1/settings/public`、`/api/v1/auth/credential-key` 和登录接口。Cloudflare 遥测失败不能单独证明 Sub2API 的认证链路失败。

## 推荐拓扑：单域名反向代理

### 请求关系

```text
浏览器 https://app.example.com
  ├── /                 -> 嵌入式 Vue 前端
  ├── /assets/*         -> 后端嵌入式静态资源
  ├── /api/v1/*         -> 管理端/用户端 API
  ├── /v1/*             -> API Key 网关
  └── /setup/*          -> 首次安装接口
反向代理 -> Sub2API :8080
```

这种拓扑下：

- 不设置 `VITE_API_BASE_URL`，让它保持 `/api/v1`；
- `cors.allowed_origins` 留空即可，跨源请求会被拒绝，但同源浏览器请求不受影响；
- 不要把前端源文件直接以 `file://` 打开，也不要把 `frontend/src` 当成生产静态目录；
- 反向代理必须把未知的 SPA 路由回退到后端返回的 `index.html`，并把 `/api`、`/v1`、`/setup` 原样转发。

现有 `deploy/Caddyfile` 已采用“一个站点反向代理到 `localhost:8080`”的基线。自定义域名时，优先只修改站点地址和证书/边缘设置，不要在 Caddy、Cloudflare 和 Go 服务上重复添加三套 CORS 响应头。

最小 Caddy 形态如下（把域名和上游地址替换成实际值）：

```caddyfile
app.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

SSE/WebSocket 不能被全局压缩或缓冲。继续沿用仓库中的 `deploy/Caddyfile` 和 [边缘安全指南](../deploy/EDGE_SECURITY.md) 的流式响应约束，不要用一个简单的 `text/*` 压缩规则覆盖所有响应。

## 需要分离前端和 API 时

假设：

```text
前端页面: https://ui.example.com
后端 API: https://api.example.com
```

### 前端构建配置

`VITE_API_BASE_URL` 是 **构建时**变量，不是运行时浏览器设置。分离部署时在构建前设置完整 API base URL：

```sh
cd frontend
VITE_API_BASE_URL=https://api.example.com/api/v1 pnpm run build
```

不要把 `/api/v1` 再重复拼接，也不要在构建完成后只修改服务器上的 `index.html` 期待它改变已打包的请求地址。构建产物会写入 `backend/internal/transport/webassets/dist/`；该目录是生成物，不能手改。

管理端 Ops WebSocket 默认从 API base 的 Origin 推导。如果 WebSocket 使用不同主机，可设置 `VITE_WS_BASE_URL`；当前代码把它当作 `host[:port]` 拼接 `ws://` 或 `wss://`，不要填写带 `https://` 的完整 URL。跨域 WebSocket 还必须在代理层转发 `Upgrade`/`Connection`，并在鉴权策略中允许管理端来源。

### 后端运行配置

在后端的 `config.yaml` 中使用精确来源：

```yaml
server:
  # 用于邮件、OAuth 等生成外部链接；填写用户实际访问的前端地址
  frontend_url: "https://ui.example.com"

cors:
  # 只填写 Origin：协议 + 主机 + 可选端口；不带路径、查询串或尾部 `/`
  allowed_origins:
    - "https://ui.example.com"
  # 当前浏览器客户端使用 HttpOnly Cookie 和 withCredentials
  allow_credentials: true
```

Origin 的比较是精确的。以下每一项都被视为不同来源：

```text
https://ui.example.com
http://ui.example.com
https://ui.example.com:8443
https://www.ui.example.com
```

`server.frontend_url`、`cors.allowed_origins`、OAuth 回调地址和 WebAuthn `rp_origins` 各有用途，不能因为域名相似就互相替代。启用 Passkey 时，`webauthn.rp_origins` 也要填写用户真实打开的完整 Origin。

### 反向代理边界

分离部署时，API 站点至少要把这些请求原样送到 Go 服务：

```text
/api/v1/*
/v1/*
/v1beta/*
/setup/*
/responses*、/images/*、/generated/*、/videos/*（若二开页面使用）
```

代理必须：

- 保留客户端 `Origin`、`Access-Control-Request-Method` 和 `Access-Control-Request-Headers`；
- 原样转发后端的 `Set-Cookie`、`Vary: Origin` 和 CORS 响应头；
- 不在 CDN/WAF 再追加第二个 `Access-Control-Allow-Origin`；
- 不缓存带 Cookie 的认证响应、写请求、SSE 或 WebSocket；
- 只对带内容哈希的静态资源做长期缓存，`index.html` 保持重新验证。

HTML 与 `assets/*` 同源时，静态资源不需要 CORS。若确实把不可变资源放到独立 CDN，应该把它当作“无凭证的公共资源”单独配置，不能把静态资源的 `Access-Control-Allow-Origin: *` 规则复制到带 Cookie 的 API。

如果前端和 API 之间可以通过反向代理合并为一个域名，应优先合并，而不是继续扩大跨源白名单。

## CORS 预检的可验证契约

当前后端 CORS 中间件对允许来源返回动态的 `Access-Control-Allow-Origin`，并设置 `Vary: Origin`；允许凭证时还返回 `Access-Control-Allow-Credentials: true`。预检成功返回 `204`，不允许来源返回 `403`。

用下面的命令从部署机或能访问 API 的环境验证（把来源和地址替换成实际值）：

```sh
curl -i -X OPTIONS 'https://api.example.com/api/v1/auth/me' \
  -H 'Origin: https://ui.example.com' \
  -H 'Access-Control-Request-Method: GET' \
  -H 'Access-Control-Request-Headers: authorization,content-type,x-admin-ui-request'
```

成功至少应同时满足：

```text
HTTP/1.1 204 No Content
Access-Control-Allow-Origin: https://ui.example.com
Access-Control-Allow-Credentials: true
Access-Control-Allow-Methods: ... GET ...
Access-Control-Allow-Headers: ... authorization ... content-type ...
Vary: Origin
```

不要只看 `curl` 的 HTTP 状态码。浏览器还会检查 `Access-Control-Allow-Origin` 是否与请求的 `Origin` 完全相同，以及带 Cookie 时是否有 `Access-Control-Allow-Credentials: true`。前端当前的 Axios 客户端默认 `withCredentials: true`，登录、刷新和管理请求必须按这个契约配置。

## 开发环境约定

推荐使用 Vite 代理，不让开发页面直接跨源访问后端：

```sh
cd frontend
VITE_DEV_PROXY_TARGET=http://localhost:8080 pnpm run dev
```

然后只从 `http://localhost:3000` 打开页面。`frontend/vite.config.ts` 已代理 `/api`、`/v1` 和 `/setup`；如果二开新增了其他浏览器请求前缀，应同时补充代理和后端路由测试。

以下方式会制造不必要的 CORS 噪音或直接破坏认证：

- 双击 `index.html` 以 `file://` 打开；
- 让开发前端使用一个绝对 API 地址，却没有把 `http://localhost:3000` 加入后端白名单；
- 在浏览器扩展的 preview/overlay 页面里验证管理后台；
- 用临时端口、IP 地址替换域名，却仍复用旧的 Origin 白名单。

若确实要测试分离开发，后端临时配置应明确写出 `http://localhost:3000`，测试结束后移除，不要把 `*` 留在生产配置。

## 二开时的代码约束

部署问题经常是由一处看似方便的前端改动扩散出来的。新增页面或 SDK 集成时，保持下面的边界：

- 业务 API 统一通过 `frontend/src/core/networks/client.ts`、`buildApiUrl` 或 `buildGatewayUrl` 发出，不在页面里新建 Axios 实例并硬编码 API 域名；
- 新增浏览器请求前缀时，同时更新 Vite 开发代理、生产反向代理和本节的浏览器验收，不要只在某个环境“碰巧能用”；
- 新增自定义请求头或 HTTP 方法时，先确认它会不会触发预检；若确有必要，补充后端 CORS allow-list、代理规则和 `OPTIONS` 负向测试；
- 静态脚本、样式、字体和 `modulepreload` 优先使用当前页面的相对 URL，避免把构建机、旧域名或个人测试域名写进产物；
- OAuth 回调、Passkey、Cookie 和 WebSocket 的 URL 不能从 `server.frontend_url` 互相推导，必须按各自协议和真实 Origin 验证；
- 每次更换域名、端口、CDN 或 iframe 容器，都要清理旧构建产物并重新执行分离模式预检测试。

## iframe、sandbox 和第三方控制台

管理端依赖正常的页面 Origin、内存 token、HttpOnly Cookie、`localStorage` 的部分兼容能力以及 OAuth/Passkey 的顶层浏览器流程。不要把整个管理 SPA 放入没有 `allow-same-origin` 的 sandbox iframe。

如果业务必须嵌入：

1. 只允许明确可信的父页面，并使用 HTTPS；
2. 评估是否真的需要 `allow-same-origin`、弹窗和表单能力，避免把管理面暴露给任意外站；
3. 使用 `postMessage` 时固定精确 `targetOrigin`，接收方校验 `event.origin`；
4. 不要让 iframe 处于 `Origin: null` 后再尝试调用登录、刷新或管理 API；
5. 更稳妥的集成方式是让父站通过同源反向代理打开独立顶层页面，或只嵌入不带会话的只读组件。

扩展、翻译器、验证码 overlay 等注入脚本也可能制造 `origin: null` 和 `localStorage` 错误。排障时先用无扩展的临时浏览器配置文件复现。

## Cloudflare/CDN 注意事项

Cloudflare 生成的 `/cdn-cgi/rum`、`/cdn-cgi/speculation` 等请求不属于 Sub2API 业务 API。对于截图中的错误，按以下顺序判断：

1. 先检查 `/api/v1/settings/public`、`/api/v1/auth/credential-key` 和实际登录请求的状态、响应体和 Cloudflare Ray/拦截信息；
2. 确认 WAF/Challenge 没有拦截 HTML、静态资源、CORS `OPTIONS` 或登录前置接口；
3. 对 API、Webhook、健康检查和网关路径不要套浏览器挑战；
4. 不要把 Cloudflare 的遥测 CORS 失败当成登录协议需要修改的证据；
5. 若 CDN 代加 CORS，必须保证它不会与后端同时产生重复头部，并按 `Origin` 正确缓存/回源。

接口分类和边缘规则详见 [Cloudflare 接口分类与边缘规则建议](CLOUDFLARE_EDGE_RULES.md)。认证仍必须保留 `credential-key`、HttpOnly `sub2api_auth_flow` Cookie 和加密 `credential_envelope` 流程，不要为了“解决 CORS”恢复明文登录。

## 常见错误修复对照

| 错误做法 | 为什么无效或危险 | 应改为 |
| --- | --- | --- |
| `allowed_origins: ["*"]` 同时 `allow_credentials: true` | 浏览器禁止通配符与凭证组合；当前后端会主动关闭凭证 | 精确列出前端 Origin |
| 把 `null` 加入白名单 | 放大任意本地文件/opaque 页面权限，且无法恢复 sandbox 存储 | 修复页面加载方式或 iframe 策略 |
| 只在前端 dev server 加 `Access-Control-Allow-Origin` | 生产请求由 API/CDN 响应，开发服务器头部不会跟着部署 | 在同源代理或后端 CORS 边界统一配置 |
| CDN 和 Go 服务各加一套 CORS 头 | 重复/冲突的头部会让浏览器拒绝响应 | 选择一个权威响应头来源，通常是后端 |
| 只代理 `/api` | `/v1` 网关、`/setup`、WebSocket 或媒体请求仍会跨源失败 | 按实际请求链路完整代理并验收 |
| 只看到 `200` 就认为成功 | CORS 是浏览器读取权限，响应可以到达但仍不可见 | 同时检查 `Origin`、预检和浏览器 Network 面板 |
| 把 `server.frontend_url` 当作 Axios base URL | 前者用于邮件/OAuth 外链，后者由构建时 `VITE_API_BASE_URL` 决定 | 分别配置并验证生成链接 |

## 发布前验收清单

每个二开版本至少完成一次真实浏览器验收：

- [ ] 页面通过 `https://` 顶层 URL 打开，不是 `file://`，控制台没有 `origin: null`。
- [ ] `index.html`、所有 `assets/*.js`、`assets/*.css` 和 favicon 返回正确的 `Content-Type` 与 `200`。
- [ ] 同源模式下 `VITE_API_BASE_URL` 未被旧域名覆盖；分离模式下它指向正确的 `/api/v1`。
- [ ] 分离模式的预检对实际前端来源返回 `204`、精确 `Access-Control-Allow-Origin`、凭证头和 `Vary: Origin`。
- [ ] 登录、刷新、退出、管理员 API、SSE 和 WebSocket 至少各验证一次；Cookie 的 Domain/SameSite/Secure 与部署域名匹配。
- [ ] Cloudflare/WAF 不挑战静态资源、`OPTIONS`、健康检查、网关和支付回调；API 认证接口仍由后端限流和鉴权。
- [ ] 生产构建使用 `pnpm run build`，没有手改 `backend/internal/transport/webassets/dist/`。
- [ ] 从仓库根目录运行 `make check-docs`；涉及前端构建时再运行 `pnpm --dir frontend run typecheck` 和相邻测试。

如果出现“整个页面几百条 CORS”而不是一个明确的 API 失败，先回到本清单的前两项：确认页面实际 Origin 和静态资源是否同源。大量资源同时报错通常是部署拓扑或 sandbox 上下文错误，不是逐个给资源添加响应头的问题。

## 相关事实源

- [前端目录索引](../frontend/README.md)
- [部署索引](../deploy/README.md)
- [边缘安全指南](../deploy/EDGE_SECURITY.md)
- [Cloudflare 接口分类与边缘规则建议](CLOUDFLARE_EDGE_RULES.md)
- `frontend/vite.config.ts`
- `frontend/src/core/networks/url.ts`
- `frontend/src/core/networks/client.ts`
- `backend/internal/transport/http/server/middleware/cors.go`
- `deploy/config.example.yaml`
