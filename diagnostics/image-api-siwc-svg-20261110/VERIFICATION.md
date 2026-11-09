# Images API、SIWC 与 SVG 编码修复（2026-11-10）

## 修复行为

- API Key 与 SIWC OAuth 归为标准 API 调用，使用 `/v1/images/generations`、`/v1/images/edits`，保留原生字段。SIWC 使用其 bearer 与 API 身份，不进入 App conversation、Sentinel 或全局 Codex 中转。
- 普通 OAuth 仅对 App 可以表达的请求使用 App 协议。显式尺寸、质量、格式、GIF 帧参数、SVG 作者、mask/markup 等字段在首次出站前选择原有 Codex 路径，不先生成再重试，不修改全局设置，不输出内部回退提示。
- SVG 账号测试与公共 JSON 响应共享有界文件规范化：兼容标准/URL-safe/无填充 Base64、data URL、UTF-8 XML、BOM 标识的 UTF-16 与 gzip，输出 UTF-8 SVG 的标准 Base64；保留未知元数据和 `svg_usage`。损坏 XML、非法编码和冒充 SVG 的位图仍报错。
- GIF 保留流式文件块扫描，不解压全部像素。SVG/GIF 的现有非流式参数限制与计费策略保持有效。

## 实际 Docker 对照

基线为 `818667ced`，镜像 `sub2api:image-standards-baseline-20261110`，镜像 ID：
`sha256:144196e203786dfb28717d9d343d33479327dc4f1dc01aa3502f637fbec45a7d`。

修复镜像 `sub2api:image-api-siwc-svg-20261110`，镜像 ID：
`sha256:c1e9dd6616d6281798e9d487a510cf4cf541ea96f18add3c5edc0d3fe27b3bc2`。

使用真实网关二进制、PostgreSQL 18、Redis 8 与本地模拟图片上游，依次执行基线、升级、回退、恢复；32 项场景检查通过。逐项结果见 [result.json](runtime/result.json)，可复现入口为 [check.py](runtime/check.py)。测试凭据只用于隔离环境，结束后删除容器、网络和数据库卷。

| 场景 | 基线/回退 | 升级/恢复 |
| --- | --- | --- |
| API Key → 第二层网关 → 普通 OAuth，含尺寸和质量 | 原 `requires legacy image generation` 400 | 200，参数保留，直接选择 Codex Images |
| 上述 API Key 账号的管理端生图测试 | 失败 | 成功 |
| SIWC `gpt-image-2` 生图 | `/v1/responses` 图片工具 | 标准 `/v1/images/generations`，无 Codex 身份头 |
| OAuth SVG 测试，上游返回 UTF-16 SVG | invalid UTF-8 | UTF-8 SVG，预览成功，无回退提示 |
| API Key 标准请求，非流式/流式 PNG | 200 | 200，原参数保留 |
| API Key 公共 SVG 响应 | 非 UTF-8 原始编码 | 标准 Base64/UTF-8，作者用量保留 |
| 实际张数计费下，返回 4 帧 GIF | 4 张，一次结算 | 4 张，一次结算 |

各阶段均确认迁移数量不变、历史用量记录不变。没有表结构变更。

## 回归与边界

固定 Go 1.26.10 Docker 执行 Images/App/2.6、两层网关、SIWC、SVG 编码及账号测试专项回归；完整生产镜像构建、golangci-lint、文档链接和补丁格式检查通过。service 与 HTTP 全量回归通过，结果记录于本目录的 `records/`。

既有布局检查仍报告 `payment_fulfillment_test.go` 为 1201 行；基线 `git show HEAD:...` 同为 1201 行，本次未修改该文件。

本地 UTF-16 fixture 复现并修复了 invalid UTF-8 症状；未取得用户原始失败响应，不能据此认定其确切编码。没有调用真实收费上游，也未验证指定账号当前的真实 SVG/GIF 权限。
