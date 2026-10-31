# 多图请求 WebSocket 1009 修复验证

基线：`8119c146220aae5ca21abae54c14299aac0b04d9`。验证日期：2026-10-31。
本记录描述修复的本地 Docker 验证，不代表线上部署或真实上游可用性。

## 问题与修复

上游通过 close code `1009 / StatusMessageTooBig` 拒绝大 WS 消息。多张图片的 Base64
字节数可能很大，即使请求低于现有 15 MiB 预分流阈值，也可能触发上游限制。
原实现还把“发送前选择 HTTP”绑定到“发送失败后可安全重放”的条件：声明
`image_generation`、`web_search` 或 MCP 等托管工具时，HTTP 预分流也被拒绝。

修复将这两种决策分开：完整请求包含结构化 `input_image`/`image_url` 的内嵌
`data:image/...` 时，在第一次上游发送前选择 HTTP Responses；其他大消息沿用现有
15 MiB 阈值。发送前选择 HTTP 允许保留托管工具和显式存储声明。失败后的重放继续使用
原有严格条件，不重放托管工具、存储副作用、增量状态或已输出的内容。

覆盖 HTTP 入站的 Codex 预热路径，以及原生 WS 的 ctx_pool/passthrough 首包。
原生 WS 客户端在同一连接接收桥接后的 Responses 事件。远程图片 URL、file ID 和文本中
提到的 data URL 不单独触发图片分流。该改动复用 `gateway.openai_ws.http_bridge_enabled`。

## Docker 复现与验证

使用 Go 1.26.10 的 `sub2api-backend-test:upstream-sync-20261030`，挂载当前 `backend/`
源码运行真实 HTTP/WS 端口测试。模拟上游在 WS 收到请求后发送 1009，HTTP 端返回成功
Responses SSE。多图输入由标准库生成真实 PNG，并使用 Base64 data URL 发送。

| 场景 | 原实现 | 修复后 |
| --- | --- | --- |
| 三张 PNG + image_generation，4,198,960 字节 | 1 WS、0 HTTP，502 | 0 WS、1 HTTP，200 |
| 三张 PNG，4,198,922 字节 | 1 WS 失败后 1 HTTP，200 | 0 WS、1 HTTP，200 |
| 大请求 + MCP，16,777,451 字节 | 1 WS、0 HTTP，502 | 0 WS、1 HTTP，200 |
| 三张 PNG + image_generation，16,789,491 字节 | — | 0 WS、1 HTTP，200 |

专项矩阵同时覆盖 JSON/SSE、多图配合网页搜索/MCP、显式存储、预热本身拒绝、普通断连
的有限重连、HTTP 错误及上下文超限 400。逐张检查图片 SHA-256、数量、顺序、detail 和
文本哈希，并检查工具类型保留。没有裁剪、压缩或丢弃图片来缩小请求。

原生 WS 首包测试分别覆盖 ctx_pool/passthrough：上游 WS dial 次数为 0，客户端收到
`response.completed`，两个图片输入及工具声明保留，BeforeTurn/AfterTurn 各执行一次。

在仓库根目录执行的主要验证命令如下，容器工作目录为 `/app/backend`：

```sh
docker run --rm \
  -v "$PWD/backend:/app/backend:ro" \
  -v sub2api-go-build:/root/.cache/go-build \
  -v sub2api-go-mod:/go/pkg/mod \
  -w /app/backend sub2api-backend-test:upstream-sync-20261030 \
  go test -json -tags=unit ./internal/application/service/... ./internal/transport/http/... -count=1

docker run --rm \
  -v "$PWD/backend:/app/backend:ro" \
  -v sub2api-go-build:/root/.cache/go-build \
  -v sub2api-go-mod:/go/pkg/mod \
  -w /app/backend sub2api-backend-test:upstream-sync-20261030 \
  go vet ./internal/application/service/... ./internal/transport/http/...

docker build --progress=plain --build-arg COMMIT=8119c14-multi-image \
  -t sub2api:multi-image-20261031 .
docker run --rm --entrypoint /app/sub2api sub2api:multi-image-20261031 -version
make check-docs
git diff --check
```

结果：10 个有测试的包通过；16,433 个测试项通过（包含父测试和子测试），0 失败，4 跳过。
机器可读汇总见 [results.json](results.json)。Go vet、生产镜像构建、二进制运行、文档检查和
diff 检查通过。镜像报告 `Sub2API 0.1.230 (commit: 8119c14-multi-image)`，本地标签为
`sub2api:multi-image-20261031`；manifest list 摘要为
`sha256:58cd6ca374929621a5a88d6a7dfe88d9ab6548f3b6acdedfedf317da48570d38`。

原始测试/构建日志保存在本机 `/private/tmp/sub2api-multi-image-20261031/`，包括
`baseline.log`、`targeted.log`、`regression.jsonl`、`vet.log` 和 `build.log`。

仓库 source-layout 检查仍有两个基线问题：`payment_fulfillment_test.go` 1201 行和
`pricing_service.go` 1218 行超过 1200 行上限；已核对 HEAD 中的行数相同，本次未修改这两个文件。

## 边界

这是使用模拟上游的本地 Docker 验证，没有调用用户账号 823，也未验证线上真实断联率。
带 `previous_response_id` 的增量状态、未配对工具结果、未解析引用、蒸馏请求和强制原连接
续接仍遵循原有状态保护；已建立的原生上游 WS 会话中途上传图片不由本次首包路由规则接管。
HTTP 传输仍受模型上下文、上游 HTTP 请求体和入口配置的限制。
