# 性能审查与平滑升级边界（2026-11-27）

## 重点性能重构

1. **#9012 流式工具名跨分片还原（`gateway_tool_rewrite.go` / `gateway_tool_rewrite_stream.go` / `gateway_upstream_response.go`）**
   - **上游问题**：上游对每个 `text_delta` / `input_json_delta` 逐一遍历全部混淆工具对调用 `restoreToolNamesInBytes`（旧 `replaceAllBytes` 每次调用还会 `[]byte(from)` 分配一次堆内存），并在 `gateway_upstream_response.go` 直接调用 `flusher.Flush()`，破坏了本仓 PR #231 的 `streamFlushCoalescer` 合并刷写优化。
   - **本仓重构**：
     - `replaceAllBytes` 改为 `unsafe.Slice(unsafe.StringData(from), len(from))` 零拷贝只读匹配，未命中零分配。
     - `toolNameStreamRestorer` 在流初始化时预计算 256-bit 首字节位图 `firstByteMask` 与下划线必需位 `requireUnderscore`：普通文本 token 通过单次 SIMD `bytes.IndexByte(slice, '_')` 与尾部字节位图即可在常数时间内跳过全部工具名扫描，并完整接入 `clientFlusher.Flush(len(events) > 0)` / `clientFlusher.Drain()`。
2. **#9080 内容审核异步队列按需分配（`content_moderation_queue.go`）**
   - **上游与旧实现对比**：本仓此前已将队列元素改为 `*contentModerationTask` 指针（避免上游值通道启动即占约 29.6 MB），但仍预分配了 100,000 槽位缓冲（约 800 KB）；上游 #9080 换用通道替换扩容但每次 `dequeueAsyncTask` 均分配 `time.NewTimer`，且并发消费与迁移旧通道存在乱序窗口。
   - **本仓重构**：采用 `sync.Mutex` 保护的按需倍增环形指针切片（未启用时容量 0 字节，首次入队从 64 起步按需倍增至 `QueueSize`）配合缓冲为 1 的唤醒通道，`dequeueAsyncTask(ctx, 0)` 热路径零定时器分配，扩容复制严格保持 FIFO 顺序。
3. **#9064 账号上游错误统计（`usage_log_repo_account_stats.go`）与 #8860 Gemini 用量刷新键（`accountUsageRefresh.ts`）**
   - 每日错误聚合统一使用 `resolveUsageStatsTimezone()`（修复上游硬编码 `'UTC'` 与本地日统计时区不一致问题），查询条件命中既有 `idx_ops_error_logs_account_time` 复合索引。
   - Gemini `base_url` 刷新键归一化为 `official` / `custom` 两态，避免第三方中转地址互改触发无意义的 `/usage` 轮询重拉。
4. **#9099 渠道公开响应模型（`channel_service.go`）与 #9088 代理检测时间（`admin_proxy.go`）**
   - `ResponseModelExposedChannels` 直接读取 `ChannelService` 内存快照，不增加单次 `/api/v1/usage` 请求的数据库查询。
   - 代理列表 `latency_checked_at` 直接复用 Redis `ProxyLatencyInfo.UpdatedAt` 现有缓存结构，零额外网络或存储开销。

## 平滑升级与回退

- **数据库与迁移**：零新增 SQL migration、零 Ent schema 变更。`channels.features_config`（JSONB）中的 `expose_response_model` 与 `accounts.extra`（JSONB）中的 `openai_chat_reasoning_content`、`image_input_models` 均为可选布尔/数组键，旧版本节点读取时自动忽略未知键，默认行为完全不变。
- **Redis 兼容性**：#9004 沿用既有 `sticky_session:{groupID}:{sessionHash}` 键名与十进制账号 ID 字符串值格式，仅将终态无条件 `SET` 收紧为单轮 Lua 原子条件写回，新旧网关实例混跑完全兼容。
- **配置兼容性**：新增 `gateway.antigravity_gemini_omit_include_thoughts` 默认 `false`，升级前后默认请求体字节一致；回退旧镜像无需清理配置或数据。

## Docker Go 1.26.11 基准测试实测（`linux/arm64`）

| 基准用例 | 吞吐 / 延迟 | 每次分配字节 | 每次堆分配数 | 说明 |
| --- | --- | --- | --- | --- |
| `BenchmarkToolNameStreamRestorer_ProseDelta32Tools/StreamRestorer_SSEBlock` | `478.6 ns/op` | `0 B/op` | `0 allocs/op` | 32 个动态混淆工具 + 16 个静态规则下完整 SSE `text_delta` 块热路径（对比逐块 `json.Unmarshal` 的 `3080 ns/op`、`568 B/op`、`14 allocs/op` 提速 **6.4x** 且零分配） |
| `BenchmarkToolNameStreamRestorer_ProseDelta32Tools/StreamRestorer_RestoreInto` | `110.8 ns/op` | `0 B/op` | `0 allocs/op` | 纯字节片段跨分片还原核心（对比逐规则扫描 `restoreToolNamesInBytes` 的 `605.8 ns/op` 提速 **5.5x**） |
| `BenchmarkContentModerationDequeueNoTimer` | `121.5 ns/op` | `288 B/op` | `1 allocs/op` | 入队 + 出队全链路（唯一 1 次分配为基准入队构造的 `*contentModerationTask` 本身，出队热路径零 `time.Timer` 分配；未启用状态队列预分配为 `0 B`） |
