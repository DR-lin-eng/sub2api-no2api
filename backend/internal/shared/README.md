# Shared Packages

本目录包含无业务流程、可被多个上层模块复用的低层包。

## 包索引

| 分组 | 包 |
| --- | --- |
| 协议与兼容 | `anthropicfp`, `apicompat`, `claude`, `gemini`, `googleapi`, `openai_compat` |
| 上游客户端 | `antigravity`, `geminicli`, `openai`, `xai`, `websearch` |
| HTTP 与网络 | `httpclient`, `httputil`, `ip`, `proxyurl`, `proxyutil`, `response`, `responseheaders`, `servertiming`, `urlvalidator` |
| OAuth 与安全 | `oauth`, `oauthstate`, `tlsfingerprint`, `logredact` |
| 通用基础 | `ctxkey`, `errors`, `logger`, `pagination`, `sysutil`, `timezone`, `usagestats` |

shared 包不得导入 `application`、`infrastructure`、`modules` 或 `transport`。出现业务名词和持久状态时，应建立模块或 application 端口，而不是继续扩充 shared。
