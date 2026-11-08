# ChatGPT Images 验证协议补齐

基础提交 `31c14bf65`。来源为用户提供的 20261107 报告第 12 节（2026-11-08 补充）
及 `evidence/browser-verification-modules/current_mac/lXEa.b087d4eb11.js`。
原始模块 SHA-256 为 `57b731e3438e640d816c78bb377207e00de6631c50133a8dfda3655b3c4888bc`。

实现和边界见 [App 生图说明](../../docs/CHATGPT_APP_IMAGES.md)。纯协议实现无需额外容器服务、
浏览器、Node 或 CGO。现有 Dockerfile 和部署配置保持兼容。

原实现固定发送环境不可用信封，并在服务端要求 PoW/VM 时直接报验证错误。
现在按每轮 prepare 执行 25 槽 `p`、原生 Go PoW 和进程内受限 VM，然后发送条件性 Sentinel 头。
已有 `extra.openai_device_id` 由原身份 owner 读取，既有持久设备 ID 与本轮新 UUID 分别使用。
保留账号出口、认证/工作区校验、结果恢复和可靠计费路径。

`reference-fixtures.cjs` 在禁网 `node:24-alpine` 中执行准确原始模块，生成 17 组 VM
golden 与 4 组独立原哈希表达式向量，涵盖 Latin-1、UTF-16 astral seed、
闭包参数、子队列错误、Promise、严格条件、数组找不到时删除末项、未注册 opcode 和 500ms timer。
使用虚构输入，不下载远端 SDK、不读取真实账号、不执行线上挑战。

重新生成 golden 时，从仓库根目录将 `CHATGPT_IMAGES_REFERENCE_DIR` 指向原始模块所在目录：

```sh
docker run --rm --network none -v "$PWD:/work" \
  -v "$CHATGPT_IMAGES_REFERENCE_DIR:/reference:ro" -w /work node:24-alpine \
  node diagnostics/chatgpt-integrity-20261108/reference-fixtures.cjs \
  /reference/lXEa.b087d4eb11.js
```

环境模型不是浏览器/Apple 证明。缺失 DOM/设备环境或预算耗尽时明确失败；
PoW 耗尽不发送原模块的错误信封作为成功证明。constructor/eval/prototype 通路被禁止，
嵌套和内存扩张受限；这些是服务端协议模拟与原浏览器执行环境的有意差异。

验证环境为 Docker Linux ARM64、`golang:1.26.10-alpine`，测试禁网。
使用已有 `sub2api-followup-gomod`/`sub2api-followup-gocache` 缓存，
`GOMAXPROCS=2 GOMEMLIMIT=1400MiB GOGC=70 GOPROXY=off GOSUMDB=off`。

从仓库根目录进入容器后，在 `/app/backend` 执行：

```sh
docker run --rm -it --network none \
  -v "$PWD/backend:/app/backend" \
  -v sub2api-followup-gomod:/go/pkg/mod \
  -v sub2api-followup-gocache:/root/.cache/go-build \
  -w /app/backend -e GOPROXY=off -e GOSUMDB=off \
  -e GOMAXPROCS=2 -e GOMEMLIMIT=1400MiB -e GOGC=70 \
  golang:1.26.10-alpine sh

go test -count=1 ./internal/modules/chatgptimages
go test -tags=unit -p 1 -count=1 -timeout=10m ./internal/application/service \
  -run 'ChatGPTAppImages|OpenAIImages|GPTImage26'
go test -tags=unit -p 1 -count=1 -timeout=15m \
  ./internal/application/service/... ./internal/transport/http/... \
  ./internal/shared/openai/... ./internal/modules/chatgptimages/...
go test -run '^$' -bench Integrity -benchtime=2s -count=1 ./internal/modules/chatgptimages
CGO_ENABLED=0 go build -p 1 -o /tmp/sub2api ./cmd/server
/tmp/sub2api -version
```

最后一次基准（2 秒采样，GOMAXPROCS=2）：

| 样例 | 单次时间 | 分配字节/次 |
| --- | ---: | ---: |
| 新 prepare `p` | 2.22 μs | 2305 B |
| 单条 resolve 的 VM | 91.08 μs | 140621 B |
| PoW，difficulty=`00` | 124.21 μs | 4599 B |

这些数值是本地虚构样例的计算开销；实际 dx、difficulty、并发和上游网络耗时另行决定。

上述模块测试、Images/GPT Image 2.6 专项、扩大回归和后端构建均通过。
扩大回归包含 application/service（236.6 秒）、HTTP handler（60.3 秒）及其余
HTTP/认证/路由/OpenAI 共享协议包。最后的专项还覆盖已有设备 ID、影子账号读取凭据 owner、
每轮 UUID/令牌隔离、失败清空旧令牌、强制登录、条件性四项头、流式/非流式双图、
Base64 去 padding/空白、Unicode JSON 与字面转义、预算/取消和并发寄存器隔离。

协议模块 `golangci-lint v2.9.0`（Go 1.26.0 构建）返回 `0 issues`。
仓库根目录 `make check-docs`、`git diff --check` 通过。

原始基线 `31c14bf65` 的检查问题另行记录：全量 lint 中 `openai_usage_sync.go:131` 的接口类型断言，
及 `openai_usage_sync_test.go:96,114,115` 的 Close/类型断言，共 4 项 errcheck；
布局门禁指出 `payment_fulfillment_test.go` 有 1201 行，超过 1200 行上限。
这三个文件不属于本次修改。提交集成基线已更新为 `d7ce1323f`，其中
`24aa1a282` 已修正上述 4 项 lint；支付测试文件的原有布局告警仍存在。

本次仅执行本地 Docker 协议/回归/构建；未调用真实 OpenAI 生图、设备证明或部署生产。
