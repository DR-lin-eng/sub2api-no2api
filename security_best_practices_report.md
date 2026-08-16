# Sub2API 安全最佳实践审查报告

## 1. 审查信息

- 审查日期：2026-08-16
- 审查提交：`51ecc755ba11198de56afc5023a9e6649d751a81`
- 审查分支：`codex/pr-7-integration`
- 基线状态：审查开始时该提交与 `origin/main` 一致
- 审查范围：Go 后端、Vue/TypeScript 前端、Docker 部署、安装与更新链路、依赖、全部客服 HTTP/WebSocket 接口、客服附件上传与读取、管理端高价值操作
- 测试方式：源码审查、静态与依赖扫描、全量自动化测试、全新 PostgreSQL/Redis/应用卷上的隔离 Docker 黑盒测试

本报告记录的是上述提交在审查时的状态。安全测试能够降低风险，但不构成对未来依赖、部署环境或未知漏洞的绝对保证。

## 2. 结论摘要

共确认 5 项需要跟进的问题：

| 编号 | 严重性 | 结论 |
| --- | --- | --- |
| SEC-001 | 高 | 未初始化且远程可达的实例可被匿名抢先安装，并可被用于探测 PostgreSQL/Redis 连接 |
| SEC-002 | 中 | 安装脚本从与发布包相同的发布信任域下载验签公钥，无法抵御仓库或标签整体失陷 |
| SEC-003 | 低 | 敏感管理操作的 step-up 2FA 默认关闭，管理员会话失窃后的纵深防御不足 |
| SEC-004 | 低 | 自定义首页 HTML 未净化，可造成同源页面覆盖、钓鱼或外部资源跟踪 |
| SEC-005 | 低 | URL allowlist、HTTPS-only 和私网地址限制默认关闭，管理员误配置时会扩大 SSRF 面 |

未发现严重级别问题。客服模块的认证、角色隔离、对象所有权、WebSocket Origin 校验、上传内容归一化和响应头均通过黑盒验证；未发现可利用的客服接口越权、跨用户附件读取或可执行上传内容保留。

## 3. 详细发现

### SEC-001：首次安装服务可被远程抢占

**严重性：高**

**证据**

- 首次运行且未启用自动安装时，程序直接启动 Web 安装服务：[`backend/cmd/server/main.go`](backend/cmd/server/main.go#L77-L90)。
- 安装服务复用默认监听地址，而默认地址为 `0.0.0.0:8080`：[`backend/cmd/server/main.go`](backend/cmd/server/main.go#L97-L130)、[`backend/internal/platform/config/defaults_runtime.go`](backend/internal/platform/config/defaults_runtime.go#L23-L25)。
- `setupGuard` 只检查系统是否尚未安装，没有一次性认领令牌、来源限制或其他认证：[`backend/internal/bootstrap/setup/handler.go`](backend/internal/bootstrap/setup/handler.go#L21-L35)、[`backend/internal/bootstrap/setup/handler.go`](backend/internal/bootstrap/setup/handler.go#L53-L63)。
- 匿名调用者可要求服务端主动连接指定 PostgreSQL/Redis 地址：[`backend/internal/bootstrap/setup/handler.go`](backend/internal/bootstrap/setup/handler.go#L116-L229)。
- 匿名调用者可提交首个管理员及持久化配置：[`backend/internal/bootstrap/setup/handler.go`](backend/internal/bootstrap/setup/handler.go#L231-L366)。

**黑盒复现**

在全新数据目录且 `AUTO_SETUP=false` 的隔离容器中，安装服务监听 `0.0.0.0:8080`。不携带任何认领凭据时：

- `GET /setup/status` 返回 200；
- `POST /setup/test-db` 与 `POST /setup/test-redis` 未返回 401/403，并实际尝试连接请求指定的端口；
- `POST /setup/install` 进入业务参数校验并返回 400，而不是认证失败。

测试没有提交有效安装请求，也没有创建管理员。

**影响**

若裸二进制首次启动、Docker 显式关闭 `AUTO_SETUP`，或其他未初始化实例被暴露到不可信网络，攻击者可在合法运维人员之前创建首个管理员，并可利用连接测试端点探测实例可达的数据库或 Redis 地址。标准 Compose 当前设置 `AUTO_SETUP=true`，可缓解默认 Docker 部署，但不能覆盖裸机部署和错误配置。

**建议**

1. 首次启动生成高熵、一次性的 setup claim token，只允许携带该 token 的 `/setup/test-*` 和 `/setup/install` 请求。
2. 安装服务默认仅监听 loopback；只有明确配置远程安装时才开放外部地址。
3. token 在成功认领或安装后立即失效，并为失败尝试增加限速和审计。
4. 为平滑升级，仅对尚未安装的新实例强制新流程；已安装实例不改变运行时行为。

### SEC-002：安装脚本的发布验签公钥未建立独立信任根

**严重性：中**

**证据**

- 安装脚本从同一 GitHub 仓库和版本标签下载归档、checksum、签名以及公钥，再使用下载到的公钥验签：[`deploy/install.sh`](deploy/install.sh#L638-L666)。
- 应用内更新器不受此问题影响；它使用编译期内置公钥，并在信任 checksum 前校验签名：[`backend/internal/application/service/update_service.go`](backend/internal/application/service/update_service.go#L38-L41)、[`backend/internal/application/service/update_service.go`](backend/internal/application/service/update_service.go#L329-L340)。

**影响**

HTTPS 可以保护传输过程，但若仓库、发布标签或相应发布权限整体失陷，攻击者可同时替换归档、checksum、签名和脚本下载的公钥，安装脚本仍会接受恶意归档。该问题只针对 `deploy/install.sh` 的初始安装信任，不应扩大为应用内更新器漏洞。

**建议**

1. 将发布公钥直接固定在安装脚本中，或固定并校验公钥 SHA-256 指纹后再验签。
2. 公钥轮换采用版本化的双公钥过渡或旧钥匙对新钥匙签名，避免直接从待验证的发布来源信任新钥匙。
3. 在发布 CI 中增加“安装脚本固定的公钥必须能验证发布清单”的回归测试。

### SEC-003：敏感管理操作的 step-up 2FA 默认关闭

**严重性：低**

**证据**

- 设置键明确声明默认关闭：[`backend/internal/application/service/domain_constants.go`](backend/internal/application/service/domain_constants.go#L195-L200)。
- 设置缺失或读取失败时返回关闭：[`backend/internal/application/service/setting_features.go`](backend/internal/application/service/setting_features.go#L249-L257)。
- 开关关闭时，中间件直接放行：[`backend/internal/transport/http/server/middleware/step_up.go`](backend/internal/transport/http/server/middleware/step_up.go#L39-L45)、[`backend/internal/transport/http/server/middleware/step_up.go`](backend/internal/transport/http/server/middleware/step_up.go#L97-L102)。
- 黑盒测试中，未完成 step-up 的普通管理员 JWT 可以访问受影响的账号数据导出端点。

**影响**

这些接口仍受管理员认证保护，因此不是认证绕过；但管理员 JWT 被盗后，导出、备份、S3 配置等高价值操作默认缺少第二道验证。

**建议**

1. 新安装默认启用 step-up，并在首个管理员引导中完成 TOTP 配置，避免锁定管理员。
2. 老版本升级保留既有设置值，确保兼容；对仍关闭的实例显示醒目风险提示和一键启用入口。
3. 设置读取失败时评估改为对敏感接口 fail-closed，并提供清晰的运维恢复路径。

### SEC-004：自定义首页 HTML 未净化

**严重性：低**

**证据**

- 非 URL 的 `home_content` 通过 `v-html` 直接写入主页 DOM：[`frontend/src/common/pages/HomePage.vue`](frontend/src/common/pages/HomePage.vue#L1-L12)、[`frontend/src/common/pages/HomePage.vue`](frontend/src/common/pages/HomePage.vue#L503-L516)。

**影响**

该设置需要管理员权限，且当前 CSP 会阻止多数直接脚本执行，因此不按匿名高危 XSS 处理。但恶意或被盗管理员会话仍可注入同源 UI 覆盖、仿冒登录/支付提示、跳转链接和外部图片跟踪；未来 CSP 放宽也可能提高影响。

**建议**

使用 DOMPurify 等经过维护的净化器并采用严格 allowlist，禁止表单、事件属性、危险 URL scheme 和不必要的外部资源。若必须支持完全自定义 HTML，将内容置于无同源权限的 sandbox iframe 中。

### SEC-005：SSRF 相关限制默认关闭

**严重性：低（安全加固）**

**证据**

- 运行时默认关闭 URL allowlist，并默认允许私网地址与明文 HTTP：[`backend/internal/platform/config/defaults_runtime.go`](backend/internal/platform/config/defaults_runtime.go#L64-L83)。
- Docker Compose 示例沿用相同默认值：[`deploy/docker-compose.yml`](deploy/docker-compose.yml#L155-L165)。

**影响**

相关 URL 主要来自管理员配置，未发现匿名调用者可以直接控制这些目标，因此不定性为匿名 SSRF。风险在于管理员误配置、低权限管理面未来扩展或管理员凭据失窃时，服务端可访问内网和明文上游的范围较大。

**建议**

1. 公网部署模板默认启用 allowlist、HTTPS-only 并拒绝私网地址。
2. 私有上游场景保留显式兼容配置，并在启用时显示风险说明。
3. 对解析后的每个 IP、重定向目标和 DNS 重绑定分别执行策略校验；配置测试接口沿用相同策略。

## 4. 客服与上传专项结论

### 4.1 接口认证与授权

- 用户客服路由均位于用户认证组并受客服功能开关保护：[`backend/internal/transport/http/server/routes/user.go`](backend/internal/transport/http/server/routes/user.go#L146-L157)。
- 管理端客服路由均位于管理员认证组并受同一功能开关保护：[`backend/internal/transport/http/server/routes/admin.go`](backend/internal/transport/http/server/routes/admin.go#L130-L154)。
- 功能开关在设置服务缺失或读取失败时 fail-closed：[`backend/internal/transport/http/server/middleware/support_chat_guard.go`](backend/internal/transport/http/server/middleware/support_chat_guard.go#L10-L21)。
- 匿名访问全部客服 HTTP 接口均返回 401；普通用户访问全部管理员客服接口均返回 403。
- JWT 查询参数被拒绝；管理员 API Key 被明确禁止用于读取客服会话。
- 两个独立用户之间的会话、消息、回复目标和附件对象均完成 IDOR 黑盒验证，未发现跨用户读取或引用。

### 4.2 上传内容隔离

- 请求体和文件体均设置硬上限，解码并发限制为 2，图片最大边长 4096、最大 1600 万像素：[`backend/internal/transport/http/handler/supportchatasset/image.go`](backend/internal/transport/http/handler/supportchatasset/image.go#L21-L29)、[`backend/internal/transport/http/handler/supportchatasset/image.go`](backend/internal/transport/http/handler/supportchatasset/image.go#L31-L80)。
- 服务端校验真实解码格式和探测 MIME 是否一致，然后重新编码为 JPEG/PNG；不会原样回传上传字节：[`backend/internal/transport/http/handler/supportchatasset/image.go`](backend/internal/transport/http/handler/supportchatasset/image.go#L123-L184)。
- 下载响应固定安全文件名，并带 `nosniff`、`private, no-store`、same-origin CORP、sandbox CSP 和 `DENY`：[`backend/internal/transport/http/handler/supportchatasset/image.go`](backend/internal/transport/http/handler/supportchatasset/image.go#L100-L120)。
- 用户附件查询绑定用户、会话和消息所有权；发送消息时再次校验附件归属：[`backend/internal/infrastructure/repository/chat_asset_repo.go`](backend/internal/infrastructure/repository/chat_asset_repo.go#L80-L126)、[`backend/internal/infrastructure/repository/chat_repo.go`](backend/internal/infrastructure/repository/chat_repo.go#L295-L349)。

黑盒结果：

- SVG、伪装 HTML 和超过 5 MiB 的文件均被拒绝。
- 合法 PNG 后追加 `<script>` 尾载荷后上传，服务端返回的是重新编码图片，尾载荷完全消失。
- GIF 被解码后扁平化为 PNG，没有保留原始可执行或附加内容。
- 附件响应的 MIME、文件名和全部安全响应头符合预期。

在本次范围内，上传内容不能作为 SVG/HTML/脚本由同源接口主动执行，也不能被其他用户越权读取。后续新增格式时必须继续坚持“解码、限制、重新编码、再保存”，不能改为依赖扩展名或客户端 MIME 原样存储。

### 4.3 WebSocket、消息与余额操作

- 用户 WebSocket 绑定已认证用户，并设置连接数、消息大小、认证寿命、ping/pong 和写超时限制：[`backend/internal/transport/http/handler/chat_handler.go`](backend/internal/transport/http/handler/chat_handler.go#L29-L40)、[`backend/internal/transport/http/handler/chat_handler.go`](backend/internal/transport/http/handler/chat_handler.go#L165-L212)。
- 用户与管理员 WebSocket 对恶意 Origin 均返回 403；同源且使用正确 JWT 子协议时均成功升级为 101；普通用户连接管理员 WebSocket 返回 403。
- 客服 JSON 请求体超过 64 KiB 时返回 413。
- 余额转账必须提供幂等键；相同请求重试返回 `X-Idempotency-Replayed: true`，同键不同载荷返回 409。
- 消息正文在 Vue 模板中使用文本插值渲染，不使用 `v-html`：[`frontend/src/features/support-chat/presentation/widgets/SupportMessageList.vue`](frontend/src/features/support-chat/presentation/widgets/SupportMessageList.vue#L19-L75)。

## 5. 测试与扫描结果

### 5.1 黑盒测试

在专用 Docker 网络、全新 PostgreSQL、Redis 和应用数据卷上启动当前提交构建的镜像，正常运行态共执行 **129 项断言，全部通过**。覆盖：

- 全部用户和管理员客服 HTTP 路由；
- 用户与管理员 WebSocket 的认证、角色和 Origin 边界；
- 双用户 IDOR；
- 上传类型、大小、图片重编码和安全响应头；
- 64 KiB JSON 上限；
- 余额转账幂等性；
- 管理 API Key 与 JWT 查询参数的拒绝路径。

另使用全新数据目录单独验证首次安装面，复现 SEC-001。测试未访问生产数据、未复用现有数据库或 Redis，也未创建真实管理员。

### 5.2 构建、单元测试和静态检查

| 检查 | 结果 |
| --- | --- |
| `make check-docs` | 通过 |
| Docker Compose 安全配置回归脚本 | 通过 |
| Docker Go 1.26.6 `go test ./...` | 通过 |
| Docker Go 1.26.6 `go vet ./...` | 通过 |
| 前端 ESLint | 通过 |
| 前端 TypeScript 检查 | 通过 |
| Vitest | 286 个测试文件、1851 项测试全部通过 |
| `pnpm audit --prod --json` | 0 个生产依赖漏洞 |
| 当前提交完整 Docker 镜像构建 | 通过，镜像标签 `sub2api-security-audit:51ecc755` |
| `git diff --check` | 通过 |

远端 `DR-lin-eng/sub2api-no2api` 对同一提交的 GitHub Actions 已全部完成并通过：[`CI`](https://github.com/DR-lin-eng/sub2api-no2api/actions/runs/31904689329)、[`Docker Image`](https://github.com/DR-lin-eng/sub2api-no2api/actions/runs/31904689325)、[`Security Scan`](https://github.com/DR-lin-eng/sub2api-no2api/actions/runs/31904689355)。上游仓库同 SHA 的 `CLA Assistant` 失败属于另一仓库的贡献者协议工作流，不是本项目构建或安全测试失败。

### 5.3 Go 漏洞和启发式扫描

`govulncheck ./...` 未发现可达符号漏洞，也未发现已导入包中的漏洞。仅报告 3 个“模块存在但当前构建路径不可达”的提示：

- GO-2026-6180、GO-2026-6179：`golang.org/x/mod v0.38.0`，修复版本为 v0.40.0；
- GO-2026-5932：涉及 `golang.org/x/crypto/openpgp`，本项目未导入该包。

这些结果不是当前可利用漏洞，但应随常规依赖升级消除。

Gosec 全量扫描产生 271 个启发式候选，聚焦扫描产生 167 个候选；扫描器在聚合部分包时同时出现 Go 1.26 编译元数据错误，因此数量不能直接解释为漏洞数。人工复核的主要类别是固定 SQL 片段、受控配置路径、非安全用途随机抖动/负载均衡以及生成代码。支付外部订单号虽使用 `math/rand/v2`，但它不是认证凭据，支付回调另有签名校验，并且数据库对非空 `out_trade_no` 有唯一索引：[`backend/internal/application/service/payment_service.go`](backend/internal/application/service/payment_service.go#L56-L70)、[`backend/ent/schema/payment_order.go`](backend/ent/schema/payment_order.go#L186-L190)。因此本报告未将其列为安全漏洞。

Docker Scout 因本机 Docker Hub 未登录而无法执行。审查没有请求、读取或复用用户的 Docker Hub 凭据；镜像供应链结论以语言级依赖扫描、完整镜像构建和人工 Dockerfile/Compose 审查为依据。这是本次扫描覆盖的已知限制。

## 6. 非问题说明

以下模式已复核，未按漏洞报告：

- WebSocket 库的 `InsecureSkipVerify` 选项控制 Origin 校验，不是跳过 TLS 证书验证；当前业务仍对浏览器提供的 Origin 执行同源判断。
- EasyPay 的 MD5 用于第三方协议兼容签名，不用于密码存储。
- OAuth token 位于 URL fragment，fragment 不发送给服务器，也不进入 HTTP Referer。
- 静态 SVG 图标的 `v-html` 与先 HTML escape 后再高亮的展示链路不接收未净化活动 HTML。
- 当前动态 SQL 中复核到的排序、分桶和条件片段来自固定枚举，值通过参数绑定传入，未确认 SQL 注入。

## 7. 修复优先级与平滑升级建议

1. **立即修复 SEC-001**：先增加一次性认领令牌与 loopback 默认绑定，并补充远程安装兼容开关及黑盒回归。
2. **随后修复 SEC-002**：固定安装脚本信任根，设计可回滚的双钥匙轮换流程。
3. **新安装安全默认值**：step-up、URL allowlist、HTTPS-only 和私网限制对新实例采用安全默认；已安装实例保留原值并给出迁移提示，避免升级后中断私有上游或锁定管理员。
4. **前端内容隔离**：净化 `home_content`，为既有自定义内容提供预览和兼容告警。
5. 修复后重复本报告中的 129 项黑盒断言，并新增首次安装认领、安装端点限速、签名公钥固定和首页净化回归用例。
