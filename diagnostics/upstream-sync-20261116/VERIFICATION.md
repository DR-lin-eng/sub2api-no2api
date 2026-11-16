# 验证记录

冻结主线 `d8d23aa16663e2262f51e9543062b90812c33c3c`，上游至 `8ed701382cbc4eadecc0de0684698ad85cf7cce0`。
本文件按实际记录汇总。首次失败与修正后的通过分开保留；本地和远端发布是独立门禁。

| 最终本地门禁 | 结果 | 记录 |
| --- | --- | --- |
| 全后端 unit / MCP OAuth | PASS，23434 子测试通过、28 条件跳过 | `backend-release-unit.json`, `UNIT_SUMMARY.json` |
| PostgreSQL/Redis 全包 integration / 独立审计库 | PASS | `integration-verified.json` |
| 全部前端测试 / 最终相邻补验 | PASS，2637 / 69 | `frontend-checks.json`, `frontend-final.json` |
| 最终 lint / typecheck | PASS，后端 0 issues | `golangci-release.json`, `frontend-final.json` |
| Race：启动、冷却、批次查询 | PASS | `race.json` |
| 正式根 Dockerfile 基线 / 最终候选 | PASS | `build-baseline.json`, `build-final-sources.json` |
| 同库四阶段 | PASS，70 场景，317 migrations | `runtime-final-sources.json`, `runtime/result.json` |
| 基线 / 候选 / 回退性能 | PASS，四个固定输入各三次 | `PERFORMANCE.json` |
| 61 项台账 / 净 diff / 继承台账 | PASS | `verify_ledger.py` |

本地镜像与 fake provider 不证明生产 OAuth/支付账户或生产吞吐。PR/main 精确 SHA CI 和镜像发布独立核验。

Docker 前端 ESLint、vue-tsc 及全套 427 files / 2637 tests 已通过（`frontend-checks.json`）。
初轮移植的编译失败保留在 records；原因包括上游旧模块路径、额外策略/审计字段与本地接口不同。
这些字段未恢复为第二套实现，测试已按当前 owner 调整。

后端首次全量发现两处剩余问题：本地 tuple cleaner 已有更强的整数类型保留，新增夹具不应期望降级为 string；
CN 原生端点虽然已写入 context，公共错误读取器仍只读取 OpenAI/Grok/Multimodal，现补齐受支持 CN 平台。
全量结果与镜像/同库验收以本目录后续实际记录为准。

已完成 Docker PostgreSQL/Redis 全量 integration 与独立 prompt_audit 存储队列验证（`integration-verified.json`）。
首次认证 limiter 在分钟边界的单次失败未修改运行规则；冻结基线同测试重复三次通过，后续完整候选也通过。
跨标签 first-output 测试补齐 fmt 导入；调度隐私 fixture 的公共成员查询方法统一放在所有标签可见文件，保留原分组过滤。
早期主动停止的旧源码测试容器退出 137 是本任务发出的 stop，不能记为 OOM 或源码崩溃。

初轮正式镜像的同库四阶段 62 场景通过（`runtime.json`）。后续追加审查新增 Gemini 模型与 session 后，
最终镜像及扩展矩阵以最终记录为准；不把旧镜像结果直接视为新增源码的运行证明。

固定 Go 1.26.11 linux/arm64、无网络、1 个 benchmark CPU、300ms×3，中位数如下。
首次性能选择器包含其它历史 benchmark 导致汇总集合断言失败，修正选择器后保留原记录并完整重跑。

| 固定输入 | 基线 | 最终候选 | 回退 | 基线 B/op | 候选 B/op |
| --- | ---: | ---: | ---: | ---: | ---: |
| 100 条 OAuth input metadata | 0.661 ms | 0.069 ms | 0.703 ms | 1426074 | 46912 |
| 1000 条 OAuth input metadata | 52.626 ms | 0.657 ms | 53.467 ms | 126718038 | 455424 |
| 1 MiB cache_control 无修改 | 2.286 ms | 0.283 ms | 2.393 ms | 0 | 0 |
| 500 个 Anthropic delta | 1.385 ms | 0.554 ms | 1.467 ms | 1225790 | 351860 |

见 `PERFORMANCE.json`、`performance-results.json` 和各阶段原始日志。量化范围为固定局部输入，不代表生产吞吐。

最终 61 PR 源码正式镜像构建通过（`build-release-candidate.json`），并在同库四阶段通过 70 个 HTTP 场景（`runtime-acceptance.json`）。
每阶段迁移数保持 317。候选/恢复中新 Hydrogen 模型发现存在，原生 JSON/SSE 各返回 1000/100 token，
各自仅一条用量，标准费用为 $0.00225。旧镜像目录中不存在该新模型，回退验证仍保持历史账单字节摘要和余额等式。
目录回退夹具首次返回错误形状后修正为实际识别的 insufficient authentication scopes；这不改变项目运行源码。
最后的前端 lint/typecheck 与 69 个相邻测试通过（`frontend-final.json`）；此前完整 2637 测试保持独立记录。

最终全后端 unit 和官方 MCP Go OAuth 客户端互操作通过（`backend-release-unit.json`），
逐测试结果计数见 `UNIT_SUMMARY.json`，包含嵌套子用例，条件跳过保留原始日志。
最终全后端 golangci-lint 2.9 / Go 1.26.11 为 0 issues（`golangci-release.json`）。
`SOURCE_HASHES.json` 的 79 个运行输入与最终镜像构建之后的本地内容逐一一致；后续只补测试与文档。

全量回归后只校正 Kimi 函数注释：月度 reset 取较晚值，周窗口 used_ratio 优先；运行表达式不变。最终镜像重新构建并复验，避免源摘要与构建输入不一致。

Go race detector 专项通过（`race.json`）：异步价卡启动、快照冷却到期和批次查询结果隔离。并发工具链为 Go 1.26.11，构建用 GCC 只存在于测试镜像。
