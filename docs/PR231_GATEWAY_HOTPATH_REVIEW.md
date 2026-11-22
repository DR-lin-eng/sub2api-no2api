# PR #231 网关热路径性能审查

本记录固定审查 `DR-lin-eng/sub2api-no2api#231` 的 head
`f2c607e45b42bdac2b6889c41e318d894ee988eb`，基线为 `8f5a3e3113caed852514f971ab4c95245b2138a6`。
PR 的范围是网关流式和请求体热路径的性能优化，包含 H2 请求体索引、H3 Responses 文本增量快路径、H4 Chat 分片直接编码、H5 已有流性能开关下的 Flush 合并和 H6 beta 策略运行时缓存。

## 兼容性和升级

这组改动没有新增数据库迁移、Ent/Wire 生成结果、Redis key、队列格式或公开 JSON/SSE/WS 字段。现有 `stream_mode_performance_enabled` 默认仍为关闭，关闭时保留逐事件 Flush 行为。开启后只合并同一读取批次中已经排队的事件，并在队列排空、尾部和返回前补 Flush。Responses、Chat、Anthropic 的用量、终止事件、客户端断连排水和失败换号路径继续由原 owner 处理。

beta 策略缓存首次加载失败仍返回错误，已有成功值在刷新失败时继续生效。本节点写入后立即发布，其他节点最多在 10 秒刷新周期后看到新值。回退不需要 Down SQL。滚动升级时可以先保留性能开关关闭，待所有网关节点完成升级后再启用。

## 性能证据

Docker Linux arm64、Go 1.26.11、相同 benchmark 参数下，内容会话 seed 基准结果如下。PR 版本约 163–166 µs/op，基线约 323–326 µs/op，耗时下降约 49%，分配量保持 1752–2288 B/op。4–32 MiB Responses 请求元字段基准两边均约 0.8–6.5 ms，未见回归。

PR 新增的 lenient JSON 批量扫描基准为约 66 µs/op、0 B/op、0 allocs/op。随机等价测试覆盖请求体索引、重复字段、前缀、嵌套值、非法 JSON 和 gjson 路径，5000 个随机 body 全部通过。

## 验证

- Docker `go test -tags=unit ./...` 通过。
- Docker `go test -tags=integration ./...` 通过。
- Docker `golangci-lint run ./...` 通过，报告 0 issues。
- `make check-docs` 和源码布局检查通过。
- 使用临时提高 Node 构建堆上限的同一发布 Dockerfile 构建完整镜像通过，后端 release 编译通过。默认 2 GiB Node 堆的宿主构建因前端 OOM 失败，提升构建堆后复现成功，PR 没有前端差异。
- 使用同一 PostgreSQL/Redis 数据库完成 `origin/main → PR #231 → origin/main` 的容器启动切换，三个阶段 `/ready` 均为 `{"ready":true}`。

真实供应商、OAuth、WebSocket 外部上游和生产并发容量不在本地夹具覆盖范围内。PR 的 CI、Docker Image、安全扫描和 Devin Review 在固定 head 上均通过。

## 差异状态

`diff_review_closed=true`，`capability_complete=true`。当前差异已按本项目 owner 实现并验证，后续不因同一 head 或同一优化范围重复检查。只有 PR head 变化、当前 owner 出现回归，或明确要求改变 Flush 默认策略、缓存一致性窗口或协议范围时重新审查。
