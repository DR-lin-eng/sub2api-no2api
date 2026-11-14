# ChatGPT App 生图默认通道与旧版回退验证

来源为用户提供的 20261107 静态报告，macOS `26.1103.41825 / build 25679`、Linux
`26.1103.32239 / build 25471`。实际读取的证据哈希见 [SOURCE_HASHES.json](SOURCE_HASHES.json)。
本次在同一工作区已完成的 OAuth 身份对齐之上继续实现图片适配，保留此前改动。
基础提交为 `345e85e7e`，没有上传、提交、发布或部署。

功能和限制见 [App 生图与旧版回退](../../docs/CHATGPT_APP_IMAGES.md)。核心变化：

- `openai_images_legacy_enabled` 默认 false，主设置 API/管理页可保存旧版回退，同一请求固定版本。
- 新版将 Images API 转为 App `picture_v2` conversation，先执行服务端验证准备；旧 Codex 实现完整保留。
- App profile、账号主体/路由与全部辅助请求共用账号出口；参考图通过文件创建、无 bearer 的存储 PUT 和 NDJSON 就绪事件上传。
- v1 通道/字段继承/patch/数组插入/append/truncate、最终图片识别、文件下载及有限 SSE/topic/历史恢复。
- 按实际完成张数保留结算；下载失败保留已完成结果元数据，不重发生图。显式 token 价格在发送前拒绝。
- 媒体工坊依据协议能力隐藏不支持的原生参数，纯原生账号分组继续展示原生参数。
- 已配置账号管理测试也使用同一通道；没有将 Library 单图合同替换进独立 Images 页面。

Docker 使用 `golang:1.26.10-alpine`、Linux ARM64、现有模块/构建缓存，测试关闭外网。
最后后端回归采用 `GOMAXPROCS=2 GOMEMLIMIT=1000MiB GOGC=40`、`-p 1`，减少巨大 service 测试包
与前端构建同时执行的内存压力。最后的专项、编译及 lint 串行复验使用 `GOMEMLIMIT=1400MiB GOGC=70`，其余离线设置相同。后端 lint 在带固定工具链的 Docker 镜像执行。

Docker 内 `backend/` 复现：

```sh
go test -json -tags=unit -p 1 -count=1 -timeout=10m \
  ./internal/modules/chatgptimages ./internal/application/service \
  ./internal/transport/http/handler/admin ./internal/transport/http/server ./internal/transport/http/handler \
  -run 'ChatGPTAppImages|PictureV2|DeltaImage|CurrentDelta|IntermediatePixel|MessageIDFallback|DistinctImageIDs|UserReferences|ImageResultsDeduplicate|MalformedDelta|OpenAIImages|GPTImage26|SettingHandlerImagesMode|OpenAIOAuthGateway|APIContracts|BuildSystemSettingsUpdatesGolden|Images_ServerError'
go test -json -tags=unit -p 1 -count=1 -timeout=15m \
  ./internal/application/service/... ./internal/transport/http/... ./internal/shared/openai/... ./internal/modules/chatgptimages/...
go build -p 1 -o /tmp/sub2api ./cmd/server
golangci-lint run --timeout=10m ./...
```

前端 Docker 使用现有安装好的依赖与当前源码/配置：设置页面、媒体工坊及 route locale coverage
共 9 个测试文件；之后运行 typecheck、lint 和生产 build。代码/检查哈希、退出状态及最终统计见
[RESULTS.json](RESULTS.json)，原始日志压缩后在 [records](records/) 保存。

已完成的功能验收：最终专项 261 个测试及子场景通过；完整相关回归 13 个包通过，
17776 个 pass 事件，4 个既有测试跳过。前端 9 个文件 / 109 项测试通过，typecheck、lint 和生产构建通过。
最终专项覆盖完整回归之后补强的原样 prompt 空白及带 event 前缀的 encoded SSE。
最终 Docker 后端编译及全量 golangci-lint 通过，lint 输出 `0 issues.`；关闭同类告警截断。
文档门禁与 diff 空白检查通过。源码布局门禁仅被基线已有的 `payment_fulfillment_test.go` 1201 行（上限 1200）阻挡。

Playwright CLI 打开 Docker 提供的真实 `SettingsOpenAIImagesModeFields.vue` 预览。
验证 1440×900、390×844，默认新模式、选择旧模式、保存/刷新恢复及切回新模式。
390px 下 `documentElement.scrollWidth == innerWidth == 390`；预览使用隔离 localStorage 模拟保存，
设置 API 的往返、部分保存和运行时缓存由 Go handler 测试（内存 repository 替身）及完整设置页面 Vitest 验证。
页面使用真实中英文词表 owner，不依赖真实登录或生产服务。

中间失败保留在记录中：前端依赖镜像最初未挂载当前 Vite/TS/HTML 配置导致启动/构建失败，补齐后通过；
旧 base64 padding 错误由新 data URL 测试复现后修正；完整回归新增设置字段快照/键数及旧原生参数场景后通过；
一次 gofmt 文件 glob 重复已改为唯一文件清单；一次并发后端编译被系统以 signal killed 终止，之后串行重跑。
全量 lint 的第一轮报告 7 个未显式处理的错误返回值；由于同类告警显示数量限制，随后又显示 2 处 transport 关闭返回值。已统一补齐资源关闭、字符串写入和测试断言，并关闭告警截断作最终检查。
这些环境/快照失败不作为产品行为验收。

测试使用虚构 OAuth 凭据与 `httptest` 本地 HTTP/SSE，WS topic 使用受控连接测试替身。
下载验证包含真实图片字节、用户输入不计图、nullable ID/历史 catchup、跨域不带 bearer，
以及生成完成后下载失败仍保留结算信息。没有真实 OpenAI 推理、生图、Apple 证明或生产风控验收。
Go 网关的验证准备明确返回浏览器上下文不可用信封；需要浏览器 challenge 的账号会得到明确错误，
未声称硬件/浏览器证明被模拟成功。
