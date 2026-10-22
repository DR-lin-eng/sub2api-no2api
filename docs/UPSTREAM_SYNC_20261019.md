# 上游主线增量同步审查（2026-10-19）

本轮以本项目 main `b75c196600d28f5831f86de9107e802d758cbea2` 为基线，延续已审查上游边界 `b0a02a3304a0a78cdd003ddda1dd23ba02c03084`，冻结至 `8f2cbb53d9d1c40e1865b07ac460571f7d4c48de`。增量为三个主线合并 PR；原链接工作树保持原始 HEAD 和字节，实施与发布在独立可写副本进行。

## 逐 PR 关闭决定

| 上游 PR | 决定 | 当前 owner 与适配内容 |
| --- | --- | --- |
| [#8223](https://github.com/Wei-Shaw/sub2api/pull/8223) | 补齐内容检查；既有网络保护复用 | `application/service/image_storage.go` 拒绝声明、嗅探均非图片的下载结果，并规范化 Content-Type 大小写。原有 `http_upstream_public.go` 已逐跳验证全部 DNS 结果并固定目标 IP，覆盖 HTTP/HTTPS/SOCKS 代理及账号 IPv6 出口；保留它，避免恢复上游全局环境代理客户端，或放宽管理员私网例外。 |
| [#8168](https://github.com/Wei-Shaw/sub2api/pull/8168) | 按本项目三条协议路径适配 | Native、Messages、Chat 在客户端取消后保留最后累计用量，带 `ClientDisconnect` 返回原可靠结算链。没有用量或已送达图片的读取中断仍报错；存活客户端的上游错误，包括内部 `context.Canceled`，不转为成功。失败残行不透传、不计数。写失败立即停止读写，不后台排空上游。 |
| [#8177](https://github.com/Wei-Shaw/sub2api/pull/8177) | 四个当前转换点适配 | `extractGeminiUsage`、Antigravity 非流式、流式累计及首帧使用 `max(prompt-cached, 0)`；保持上游缓存读 Token、输出与 reasoning 用量。 |

[机器台账](../diagnostics/upstream-sync-20261019/UPSTREAM_PRS.json) 固定每个 merge SHA、原始 diff SHA-256、owner、覆盖项和重开条件；[冻结输入](../diagnostics/upstream-sync-20261019/UPSTREAM_REVIEW_INPUT.json) 固定历史台账哈希。旧的 ported、covered、有意差异和专项 deferred 状态继续有效，只有新的上游语义或当前 owner 的实际回归才重开。近期已发布的搜索、退休模型、时间戳、支付和 Responses 准备阶段保活不重复处理。

## 本地协议和计费边界

本项目 Chat 流已经将 Gemini inline image 转成 Markdown。上游 #8168 按“Chat 流不输出图片”处理，不能直接照搬：本轮保留本地输出，对中途断开的 Chat 只计成功写出的图片；Native 也只在成功写出后更新图片观测，Messages 未结束时不使用模型名推测图片。正常结束仍保留既有模型名兜底。已送达是应用层完整写入成功的观测，不代表客户端应用已消费或保存。

用量使用最后完整分片的累计值，不累加重复分片，不估算尚未返回的用量。读取错误须有下游请求取消证据；上游内部取消或错误不触发该分支。未结束、没有任何可计费观测的断流继续报错。此次没有新增 worker、goroutine、队列、数据库或 Redis 调用，也没有改变幂等结算、调度、账号出口、失败排除或并发槽位释放规则。

修改涉及的 Messages 和 Native 流实现分别移到 `gemini_messages_stream.go`、`gemini_native_stream.go`，请求内写失败跟踪与取消判断放入 `gemini_stream_client_abort.go`。包装器保留 Gin/net/http 的 `WriteString` 路径，避免对大型 Native SSE 图片行新增整段 byte 复制；大字符串写出夹具验证零分配。既有大型文件缩小，不扩大源码 allowlist，也不重复处理与本轮无关的布局债务。

## 性能

相同 Docker Go 1.26.9、linux/arm64、GOMAXPROCS=2、每项三轮 500ms，在其它编译/检查结束后连续执行基线与候选；写出夹具实现与 Gin/net/http 一致的 StringWriter。Native 文本流耗时中位数变化 +5.09%，8 MiB 图片流变化 +2.63%；Messages、Chat 分别为 -2.68%、-1.24%。Native 每请求增加约 49 字节、两次分配，Messages 增加约 53 字节、一次分配，Chat 分配次数不变；没有新增一份完整图片/历史复制。该结果仅覆盖测试机和固定夹具，不能推断生产网关吞吐或上游生成时间。并行编译期间的性能记录明确排除，不用于比较。

[PERFORMANCE.json](../diagnostics/upstream-sync-20261019/PERFORMANCE.json) 保存原始样本和中位数，测试夹具与命令保存在同目录。公网图片下载继续使用既有隔离传输，未为了共享连接池放宽 DNS 固定、代理或跨账号边界。

## 平滑升级和回退

本轮没有 schema、迁移、Ent/Wire、依赖、必填配置、前端协议或本项目版本变更。旧节点、新节点及回退共用原数据结构。修正只作用于新处理的请求，不回写历史账单；混合版本期间取消请求的结算和缓存专有用量输出可能不同，滚动发布完成后行为才一致。

异步结果 URL 的声明和嗅探均非图片时，现在转存失败并沿用原任务失败流程；依赖错误 Content-Type 且正文也不能识别为图片的来源需要修正响应。声明为合法图片但 Go 无法嗅探的 AVIF 等格式继续接受。既有下载大小、超时、重定向、公网和账号出口限制保持。

## 验证和证据

同输入 Docker 源码探针已观测：基线缓存专有输入 `-30`，三路径取消均返回错误、不产生 ForwardResult，非图片转存成功；候选输入 `0`、缓存读仍为 `30`，三路径均保留 `10/5` 用量并标记断开，非图片转存拒绝。最终生产镜像同库 `BASELINE → MODIFIED → ROLLBACK → RESTORED` 全部通过，四阶段迁移数始终为 305，身份/Key/订阅快照 hash 一致。真实断连后三路径分别只落库一条 `10/5` 用量，回退恢复基线行为，再次升级恢复修复。完整后端 unit/integration、定向 race、前端 lint/typecheck 和 247 项关键测试通过；独立源码回退、静态检查及发布的逐字结果以 [VERIFICATION.txt](../diagnostics/upstream-sync-20261019/VERIFICATION.txt) 和相邻记录为准。

源码角色为 [MODIFIED_FILE.tar.gz](../diagnostics/upstream-sync-20261019/MODIFIED_FILE.tar.gz)、[DIFF_FILE.patch](../diagnostics/upstream-sync-20261019/DIFF_FILE.patch)、[VERIFICATION.txt](../diagnostics/upstream-sync-20261019/VERIFICATION.txt)、[ROLLBACK.sh](../diagnostics/upstream-sync-20261019/ROLLBACK.sh)。回退只接受独立副本，使用 sibling 基线恢复本轮路径并删除新增路径；hash 核验和补丁重建另行记录。

基线与候选的源码布局检查均报告 `openai_gateway_forward.go` 和 `payment_fulfillment_test.go` 各 1201 行，两文件与基线逐字相同；保留为既有问题，不扩大 allowlist、不描述为本轮回归。最终 PR、精确 main 的 CI/Security/Docker 与 GHCR 多架构证据独立核对，本地测试不代表远端发布已完成。
