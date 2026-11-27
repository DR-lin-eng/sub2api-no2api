# Codex WS 图片压缩验证

本诊断验证 Codex Responses WebSocket 的 132 MiB 网关读取边界、OAuth 固定 128 MiB、API Key 可配置 WS 单帧预算、图片压缩开关、10 MiB 软目标和 32/64/128 MiB 路由硬上限。测试覆盖原生 WS、HTTP bridge、流式响应、1009 恢复、透明 PNG、字符串化 `function_call_output.output` 和压缩 benchmark。

从仓库根目录运行：

```sh
diagnostics/ws-image-compression-20261127/run.sh
```

脚本使用 Docker `golang:1.26.11-alpine`、2 CPU、4 GiB 内存、`GOMAXPROCS=2`、`CGO_ENABLED=0` 和 `--network none`。源码只读挂载，Go module/build cache 使用已有 Docker volume。输出保存在 `/private/tmp/sub2api-ws-image-compression-20261127/`，包含完整日志和 `RESULTS.json`。

Docker fixture 验证网关逻辑、请求体大小和事件恢复语义。它不代表真实 OpenAI 上游的服务端硬限制或模型视觉质量。
