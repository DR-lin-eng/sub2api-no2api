# TypeSafe

TypeSafe Jev 原生 System One 协议的独立模块，不依赖 application、transport 或 infrastructure。

- `systemone.go`：非流式请求校验，支持 `noul`、`choice` 和 `score`；拒绝重复字段、关键字段大小写变体及 `stream:true`。根对象只解析一次。
- `client.go`：Bearer 请求构造、4 MiB 有界响应读取、原始 JSON 保留和兼容用量解码。未发出成功请求前的校验错误不计费；异常上游响应留待应用层记录运维证据。
- `systemone_test.go`、`systemone_benchmark_test.go`：协议与性能回归。

账号筛选、代理/IPv6 出站、失败转移及可靠结算由 `application/service/gateway_systemone.go` 和 `transport/http/handler/gateway_systemone.go` 编排。管理端账号连接测试仍使用 SSE 包装测试进度，模型接口本身只返回 JSON。

协议来源：[官方 OpenAPI](https://api.typesafe.ai/openapi.json)。接入、升级和验证记录见 [专项适配](../../../../docs/UPSTREAM_ADAPT_20261006.md)。
