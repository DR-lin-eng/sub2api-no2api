# httputil

HTTP 请求体读取、压缩解码、宽容 JSON 规范化以及 Cloudflare challenge 识别。文件索引：`body.go`, `cloudflare.go`。

解压输出必须完整通过 EOF/校验和且不超过既有 64 MiB 上限；超过上限明确返回错误。
zstd decoder 使用单并发和 8 MiB window。这里不引入共享 ingress 额度或读写 deadline；升级边界见
[同步审查](../../../../docs/UPSTREAM_SYNC_20261107.md)。
