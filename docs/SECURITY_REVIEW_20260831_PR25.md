# PR #25 / #28 / #29 安全审查（2026-08-31）

## 执行摘要

审查基线为 `14a69a2a29085094e0ea0e33f5f71401bedb7e7d`，覆盖 PR #25、#28、#29 最终合并后的客服、嵌入页能力令牌、自定义模型请求模板、媒体工坊、OpenAI Images 与 Agnes 视频链路。

本轮确认并修复 5 组问题：2 个中危资源/审核边界、1 个中危上游访问边界，以及 2 个低危浏览器凭证与 URL 规范化问题。修复后没有未处理的 Critical、High 或 Medium 发现。自定义菜单的主 URL iframe 仍以管理员配置为信任边界；同源嵌入内容必须视为受信任代码。

## 发现与修复

### SEC-PR25-001：multipart 图片被静默截断（Medium，已修复）

- 规则：GO-HTTP-002
- 位置：`backend/internal/application/service/openai_images.go:451`；`backend/internal/application/service/openai_images_request_adapter.go:741`
- 证据：原实现对每个 part 使用 `LimitReader(..., 20 MiB)` 后直接接受结果，没有读取第 `limit+1` 字节。
- 影响：超过 20 MiB 的图片会在本地审核视图中被截断，而无 adapter 的 API Key 路径仍可能转发原始完整 multipart，造成审核内容与实际发送内容不一致；大量小 part 也可增加解析开销。
- 修复：统一读取 `20 MiB+1` 并拒绝超限；普通解析与 multipart-to-JSON 共用同一 helper；整个请求最多 64 个 part。
- 验证：20 MiB+1 与 65-part 回归测试均从“错误接受”变为明确拒绝。

### SEC-PR25-002：自定义模板可放大请求体并改变路径语义（Medium，已修复）

- 规则：GO-HTTP-002、GO-SSRF-001
- 位置：`backend/internal/application/service/openai_images_request_adapter.go:210`、`:229`、`:306`、`:546`
- 证据：64 KiB 模板可多次展开 `request.body`、`request.images` 等大变量；渲染后的 path 未重新校验。
- 影响：已启用模板的 API Key 用户可通过大请求触发多倍 JSON 分配；动态值还可生成 `../`、双斜杠、控制字符或类似绝对 URL 的路径并携带上游凭证。
- 修复：字符串展开改为单遍且受预算限制；JSON 编码前做无大对象分配的大小预检；最终 body 上限为输入体 base64 理论膨胀值加 256 KiB；渲染后再次拒绝绝对 URL、双斜杠、反斜杠、查询/片段字符和 `.`/`..` 段。
- 验证：重复两次 1 MiB `request.body` 被拒绝，单次投影仍通过；JSON 预估与实际编码边界测试通过。

### SEC-PR25-003：Agnes 绕过上游 URL 策略并记录完整错误体（Medium，已修复）

- 规则：GO-SSRF-001、GO-CONFIG-001
- 位置：`backend/internal/application/service/agnes_video.go:134`、`:242`、`:270`、`:316`
- 证据：Agnes 使用账号 `base_url` 直接构造带 Bearer 凭证的请求，没有调用统一 URL allowlist；错误响应体完整写入日志；鉴权视频响应沿用上游缓存语义。
- 影响：错误配置可绕过部署的上游主机限制；上游错误中的 token、提示词或内部信息可能进入日志；共享缓存可能保留生成视频。
- 修复：发送凭证前执行统一 `validateUpstreamBaseURL`；日志先限制到 2 KiB 再脱敏；视频内容强制 `private, no-store` 与 `nosniff`。
- 验证：非 allowlist 主机在任何凭证/网络访问前被拒绝；日志 token 被替换且长度受限；206 Range 响应与新增安全头同时通过。

### SEC-PR25-004：媒体工坊会话凭证响应缺少缓存禁用（Low，已修复）

- 规则：GO-CONFIG-001、GO-HTTP-002
- 位置：`backend/internal/transport/http/handler/media_studio_handler.go:20`、`:100`、`:128`
- 证据：`POST /media-studio/session` 返回持久 API Key，但没有 `Cache-Control: no-store`；两个媒体工坊 JSON 写入口仅依赖 256 MiB 全局限制。
- 影响：不符合凭证响应的最小缓存边界；已认证请求可为很小的 DTO 触发不必要的大体积解析。
- 修复：会话响应从 handler 入口即设置 `no-store/no-cache`；会话与管理分组更新统一限制为 256 KiB，并映射为 413。
- 说明：前端仅把该 API Key 保存在 Vue 内存状态，不写入 localStorage；它只能调用用户本来有权绑定的网关分组，不能登录管理面。

### SEC-PR25-005：相对 URL 反斜杠可变成跨域地址（Low，已修复）

- 规则：VUE-XSS-004
- 位置：`frontend/src/core/utils/url.ts:22`
- 证据：`/\\evil.example/path` 通过“单斜杠相对路径”检查，但浏览器 URL 解析会把它规范化为 `https://evil.example/path`。
- 影响：管理员 Markdown 中的 iframe 可绕过协议相对 URL 拒绝规则。
- 修复：允许相对 URL 时拒绝任何反斜杠；DOM 回归测试确认 iframe 被移除。

## 已验证安全边界

- 嵌入能力令牌：90 秒 HS256 独立派生密钥，绑定 issuer、audience、菜单、Origin、用户、角色与 TokenVersion；普通 Sub2API JWT 中间件不接受该令牌。URL 构造会删除 `token/access_token/auth_token`，仅在开关开启时通过 `postMessage` 发送，并校验 `event.source` 与精确 Origin。
- 客服：用户 JWT、管理员角色与功能开关位于路由边界；图片上传限制 5 MiB、4096 边长、1600 万像素和 2 个并发解码槽位，解码后重新编码；资产读取绑定用户/会话/消息或管理员上传者。
- 客服清理：`retention_days=0` 明确禁用，批次之间重新读取管理员设置；异常值失败关闭；`balance_transfer` 永久排除；删除后同事务重算未读计数。
- 自定义模板：CRUD 仅在管理员路由；模板定义限制 64 KiB；Authorization、Host 等敏感请求头不能覆写；运行配置使用 5 秒编译缓存，不在每个请求查询数据库。
- Agnes 任务：任务与 `user_id + api_key_id + group_id` 绑定，任务 ID 使用路径转义；状态与内容不能跨用户/API Key 复用绑定。

## 性能结果

64 KiB JSON adapter 基准（Linux arm64，5 次、1 秒/次）：

| 版本 | median ns/op | 吞吐 | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| 基线 `14a69a2a` | 516,910 | 约 126 MB/s | 约 218.5 KiB | 129 |
| 安全修复 | 618,199 | 约 106 MB/s | 约 219.8 KiB | 132 |

适配器纯 CPU 阶段约增加 19.6%（约 0.10 ms/请求），内存增加约 0.6% 和 3 次分配。该路径只在自定义图片 adapter 启用时执行，随后是网络上传与秒级图片生成，因此没有端到端显著性能影响；换取的是在编码前阻止无界内存放大。

## 验证清单

- Docker Go unit：`go test -p=1 -tags=unit ./...`
- Docker 前端：347 个文件、2130 个测试通过
- 前端：`vue-tsc --noEmit`、`eslint .`
- Go 静态检查：`golangci-lint` 0 issues（含 gosec/staticcheck/govet/errcheck）
- 依赖：`govulncheck` 0 个可达漏洞；pnpm production audit 0 high/critical
- 构建与运行：完整 Docker 镜像、健康检查、浏览器页面与鉴权资源响应另见本次验证记录

## 剩余信任边界

自定义菜单主 URL iframe 为了保留精确 Origin 的 `postMessage` 协议，没有使用 opaque-origin sandbox。跨域页面受浏览器同源策略隔离；同源页面则拥有同源脚本权限，因此管理员不得把不受信任内容配置为同源自定义菜单。能力令牌本身不能换取 Sub2API 会话或访问 `/auth/me`。
