# Docker 验证与性能记录

本项目基线 `1e629a9a75e043ae0a888a9231cf7c88ad8f37e8`，上游冻结至
`6b4b37db43443b5e88e424fe3c69602cc3810dfb`。逐 PR 决定见 [关闭台账](UPSTREAM_PRS.json)。

## 已完成的验证

- Docker Linux arm64，Go 1.26.10、Node 24、pnpm 11.17.0。
- PostgreSQL 18.1 / Redis 8.4 的全包 integration 通过；数据库、网络和容器独立创建并清理。
- 82 个相关前端用例和 ESLint 通过；4 GiB Node heap 的 typecheck 通过，31 文件/280 个关键用例通过。
- 冻结主线与候选的正式多阶段 Docker 构建通过；同库四阶段结果见下文。
- 实际浏览器检查 1280×720 桌面及 390×844 手机布局，中英文额度和显式 API Key 配置正确，手机 document width 为 390；无控制台错误。见 [UI.json](UI.json)。
- 新增契约测试验证图片缓存精确金额、图片倍率、Terra 的显式字段优先级、强制模式不探测、原生响应未知字段、真实 HTTP/1 读取 deadline 及取消后空响应。

首轮全包单元测试只在新增 2.6 路径失败：接入时替换了 builder 产生的压缩体而未清除 zstd 头；之后补齐
请求体/编码一致性和 `image_edit.completed` 计数。其余包通过。一次并行编译被 Docker 内存限制终止，后续采用
顺序编译和受限 Go 并发；通过结果见下文。原 lint 镜像的内置 Go 版本不足，因此使用同版 lint 二进制和
Go 1.26.10 组成验证镜像，未修改项目依赖或 CI 规则。

## 性能

[原始结果](PERFORMANCE.json) 保存全部三次样本。禁网 Docker、2 CPU 配额、benchmark 固定 1 CPU、
4 GiB 内存，200ms × 3，下面取中位数。B/op 是累计分配，不是峰值 RSS。

| 固定夹具 | 对照 → 本轮 | 累计分配 B/op |
| --- | --- | --- |
| 1024 项 × 256 字节缓冲 reasoning | 冻结主线 8.714 → 0.153 ms | 138,003,296 → 1,448,800 |
| 8 张、合计 2 MiB base64 的模型/URL 改写 | 冻结上游逐图整包算法 42.779 → 37.448 ms | 126,658,784 → 17,109,148 |
| Terra instructions | 上游逐请求 JSON 解析 233.937 μs → 启动缓存查询 47.74 ns | 44,088 → 0 |

缓冲 reasoning 使用相同源码夹具分别运行在冻结主线和候选。多图和 Terra 比较固定上游算法与当前实现，
没有把本项目原有的常量 prompt 误称为逐请求解码。多图夹具包含 base64 尺寸检测和 URL 字段改写，
保留未知字段的回归独立验证。测试数据不含网络、上游推理或生产负载，不能外推整站吞吐。

## 升级回退与最终检查

最终候选镜像与冻结主线均完成正式构建。独立 Compose 项目在同一 PostgreSQL/Redis 上完成
BASELINE → MODIFIED → ROLLBACK → RESTORED 四阶段，24 个真实 HTTP 场景全部通过，见
[runtime/result.json](runtime/result.json)。迁移数始终为 **314**；用户/密码摘要、API Key、订阅和历史
用量行摘要保持一致，每次成功请求只结算一次，`余额 + 累计 actual_cost = 初始余额` 在四阶段均成立。
Responses JSON/SSE 均结算 1000/100 token、$0.004，图片实际两张结算 $0.20。新增缓冲 reasoning 在候选/恢复节点出现，
在旧节点回退为旧行为，且每阶段均只有一条结算。后端全包 unit 和官方 MCP Go SDK OAuth 互操作检查通过（service 全集 229.254 秒）；
最后的账号原生图片预览、畸形输出和额度展示规模限制又通过定向 Docker 回归。追加的原生图片流回归覆盖“已有用量后收到空 usage”的结算保留，Docker 定向集通过（0.023 秒）。固定 Go 1.26.10 / golangci-lint 2.9.0
的检查报告 **0 issues**。生产节点未部署。

`make check-docs` 通过。源码布局门禁目前只报告未修改的 `payment_fulfillment_test.go` 为 1201 行；
本轮已按职责拆分新增 WS 计时测试和图片静态价卡，没有扩大 `.source-layout-allowlist`。

远端 CI 的最终事实以本轮 PR 和合并提交的 GitHub Actions 记录为准。本地 Docker 结果不代表远端 CI 或生产部署。


## CI 时序回归收尾

首次远端后端失败是既有 `TestOpenAIUsageHandlersValidateAndStream` 在 peer 看到 Close 后立即断言
subscription release，观察到 0 次。handler 与测试在冻结主线到本轮之前均未修改；Close defer 先于
后续 release defer，客户端读失败不构成清理已完成的同步点。测试改为有界等待原子 release 计数，
保留次数、来源校验和协议断言，产品生命周期不变。Docker 连续 200 次通过（0.066 秒）。
具体运行和关闭证据见 [CI_TEST_FIX.json](CI_TEST_FIX.json)。
