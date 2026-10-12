# Responses 图片编辑 SSRF 检查与修复

2026-10-06：确认 **High** 风险并完成修复。修复前，持有有效网关 API Key、具有图片生成权限且请求进入 Responses 图片桥接的用户，可通过图片或遮罩 URL 触发后端访问私网、回环或元数据地址。Docker 对照测试复现了真实 HTTP 请求；修复后 84 个攻击场景均在访问危险目标前被拒绝，4 个公开图片与流式/非流式兼容场景通过。

## 发现 1：用户图片 URL 继承了管理员私网上游例外

- Rule ID：`GO-SSRF-001` / CWE-918。
- 严重性：High。攻击者可利用服务端网络位置访问内部 HTTP 服务或元数据入口。
- 状态：已修复；基线为 `9600705ea6a6ce9fc02512582e86ef57f9242f63`。
- 入口：`input[].content[].image_url` 与 `tools[].input_image_mask.image_url`；既包含共享 image plan，也包含账号 Images API 桥接。

源码证据：

| 位置 | 事实 |
| --- | --- |
| `backend/internal/application/service/openai_responses_image_api_bridge.go:424`，`resolveOpenAIResponsesImageInput` | 用户图片和遮罩的统一解析入口；基线直接调用通用生成图片下载器 |
| `backend/internal/application/service/openai_responses_image_plan.go:212`，`PrepareOpenAIResponsesImagePlan` | 共享预下载同时处理图片与遮罩；位于选取图片账号之前 |
| `backend/internal/application/service/openai_generated_image_proxy.go:281`，`validateGeneratedImageURL` | 基线使用全局 `AllowPrivateHosts`，没有为用户图片建立独立策略 |
| `backend/internal/infrastructure/repository/http_upstream.go:789`，`shouldValidateResolvedIP` | 基线仅在 URL allowlist 启用且私网例外关闭时执行 DNS/重定向地址校验 |
| `backend/internal/platform/config/defaults_runtime.go:74`、`:91`、`:92` | 默认 `enabled=false`、`allow_private_hosts=true`、`allow_insecure_http=true` |
| `backend/internal/transport/http/handler/openai_forced_image_responses.go:461` | HTTP handler 在编译/恢复 image plan 后调用共享预下载；WebSocket 路径也使用同一方法 |

基线关键调用为：

```go
b64, err := s.downloadGeneratedImageBase64(ctx, rawURL)
```

图片类型、长度与状态检查发生在 HTTP 请求返回之后，因此非图片报错不能防止 SSRF。本次模拟元数据返回普通 JSON，基线报 `non-image` 错误，但目标已收到 GET；这证明访问能力，不证明真实云凭据已被读取。是否能读取实际凭据仍取决于部署网络和元数据服务的鉴权要求。

## 修复边界

`resolveOpenAIResponsesImageInput` 为远程用户输入执行 `ValidatePublicHTTPURL`，并通过 application 定义的 `WithHTTPUpstreamPublicDestination` 向 HTTP port 传递强制策略。repository 的普通和 TLS 入口都执行该策略，不受管理员私网上游例外、连接池或实验传输开关影响。

独立下载器执行以下约束：

1. 仅允许 HTTP(S)，HTTP 仍遵循既有 `allow_insecure_http`；拒绝 URL 内嵌凭据。
2. 拒绝私网、回环、链路本地、共享地址及特殊用途地址，覆盖 IPv4-mapped IPv6、元数据地址和常见翻译/隧道地址段。
3. 对每次请求的全部 DNS 结果校验，任何危险结果都拒绝；连接目标使用已验证的数值 IP，避免检查和连接之间再次解析。
4. 每个重定向重新校验、解析和固定 IP，最多 10 跳；保留原 hostname 的 HTTP Host 与 TLS SNI/证书验证。
5. 保留账号代理/IPv6 出口。HTTP/HTTPS 代理通过 CONNECT 访问固定 IP；SOCKS 代理也收到固定 IP，避免代理再次解析用户域名。
6. 用户图片使用独立 transport，不继承普通上游连接池、cookie jar 或私网例外。下载总超时 30 秒，原有 50 MiB 上限保留。

base64 data URL、本地 `/generated/<hash>` 和账号 `file_id` 路径保留。图片 URL 去重、共享 image plan、流式事件、子请求与计费流程保留。管理员配置的内部模型上游继续使用原有策略。

兼容性变化：直接提供私网图片 URL 会被拒绝；可改用 data URL、账号文件或已生成的本地图片。使用 HTTP/HTTPS 代理下载公开图片时，代理需要允许对图片目标端口建立 CONNECT 隧道；不允许的代理会明确失败，不会绕过账号出口。IPv6 显式出口没有可用公开 IPv6 目标时仍失败关闭。

## Docker 证据

测试使用 `golang:1.26.6-bookworm`，直接运行当前 application service 与真实 repository HTTP port。三条 `--internal` Docker 网络承载私网、模拟元数据和公开地址夹具；不发布宿主机端口，不访问真实云元数据或互联网。公开夹具使用 `93.184.216.34`，该地址在测试中只分配给隔离容器。

下表为默认配置下每个场景的代表请求；所有场景均分别覆盖 `stream=false/true` 和图片/遮罩输入。

| 目标 | 基线目标 GET 数 | 修复后目标 GET 数 |
| --- | ---: | ---: |
| `10.253.77.20` 私网图片 | 1 | 0 |
| `127.0.0.1` 回环图片 | 1 | 0 |
| `169.254.169.254` 模拟元数据 JSON | 1，访问后报非图片 | 0 |
| 域名解析至私网图片 | 1 | 0 |
| 公开夹具重定向至私网 | 1 | 0 |
| 公开夹具重定向至模拟元数据 | 1，访问后报非图片 | 0 |
| 公开夹具重定向至私网域名 | 1 | 0 |

仅把 `allow_private_hosts=false`、保持 `enabled=false` 时，基线仍能通过 DNS 或重定向访问内部目标。`enabled=true` 且 `allow_private_hosts=false` 的静态对照场景可阻止访问；修复后的用户输入策略对三种配置均生效，共 84 个攻击场景通过。公开图片、相对公开重定向、非流式 JSON 与流式 Responses 合成事件的 4 个完整 multipart 转发场景通过。

补充单元回归覆盖特殊 IP、IPv6、混合公私 DNS 结果、固定 IP/Host/TLS 身份、HTTP/SOCKS 代理、取消、IPv6 失败关闭、旧桥接、data URL、file ID 和本地生成图片输入。测试不替代生产环境的完整数据库/API Key 部署验收。

从仓库根目录复现：

```sh
bash backend/scripts/test-responses-image-ssrf.sh
```

脚本固定漏洞基线提交，使用 Go overlay 注入同一套外部测试，再验证当前工作区源码；要求输出真实测试的 PASS 标记，避免集成测试被跳过却返回成功。日志位置在运行时输出，退出时自动清理自己创建的容器和网络。缓存要求见 [脚本说明](../backend/scripts/README.md)。

运行相关后端回归时，在 Docker Go 1.26.6 容器的 `backend/` 目录执行：

```sh
go test ./internal/shared/urlvalidator ./internal/application/service/... ./internal/infrastructure/repository ./internal/transport/http/... -count=1 -timeout=10m
```

仓库文档验证从根目录执行 `make check-docs`。源码布局检查发现基线已存在两处超长文件：`internal/application/service/openai_ws_v2/passthrough_relay.go`（1273 行）与 `internal/application/service/pricing_service.go`（1238 行）；本次未修改这两处。

本次验证结果：相关 application service（含 WS v2）、repository、HTTP handler/admin/DTO、middleware 与 routes 单元测试全部通过；新图片下载与代理传输的 `go test -race` 通过；`make check-docs`、脚本语法检查与 `git diff --check` 通过。源码布局门禁因上述两处基线问题返回失败，不能宣称仓库全部门禁通过。

## 部署与缓解

部署包含该修复的后端即可启用用户图片限制，无需修改管理员私网上游配置或迁移数据库。网络层限制应用对内部服务及元数据地址的出口可作为额外防护。旧版本临时启用 upstream allowlist 并关闭私网例外可以阻止已测试的静态 DNS/重定向场景，但会影响内部模型上游，也不能替代本次连接 IP 固定。
