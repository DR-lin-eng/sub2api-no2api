# 上游主线增量审查（2026-11-27）

本项目基线为 `1dbeb9c31`，上游增量为 `0f6671457..81ce8356d`，共 22 个首父合并 PR。每项的固定提交、净 diff SHA-256、当前模块 owner、关闭状态与重开条件见 [机器台账](../diagnostics/upstream-sync-20261127/UPSTREAM_PRS.json)。台账继承 2026-11-24 的历史摘要并通过追踪合并链接 `81ce8356d`，已处理范围后续不再重复扫描。

## 决策概览

| 数量 | 状态 | 覆盖 PR |
| --- | --- | --- |
| 19 | `adapted-local-scope` | `#8860`、`#8994`、`#8995`、`#9004`、`#9012`、`#9034`、`#9060`、`#9064`、`#9069`、`#9076`、`#9079`、`#9080`、`#9081`、`#9087`、`#9088`、`#9093`、`#9094`、`#9095`、`#9099` |
| 1 | `reused-current-implementation` | `#9046`（本站从未对智谱 429 锁定 5h 窗口，既有秒级限流已覆盖 `1305` 过载，仅补回归单测） |
| 2 | `retain-local-contract` | `#9084`（无 `PlatformCommandCode` 与分组扫描合成目录，`capability_complete=false`）、`#9085`（未注入硬编码旧版 OpenCode UA，保留本地透传/覆写契约，`capability_complete=false`） |

## 本批适配与性能重构要点

- **网关与协议正确性**：#9004 通过 Redis Lua 脚本 `CompareAndSetSessionAccountID` 消除长流式请求结束后覆盖并发粘性改绑的竞态；#9012 新增跨 SSE 分片工具名还原器 `toolNameStreamRestorer`，以 256-bit 首字节位图与 SIMD 下划线预筛避免普通文本分片逐工具扫描和堆分配，并保留 PR #231 的 `streamFlushCoalescer` 合并刷写；#9081 跨换号重试累计等待期 SSE 心跳字节与提交状态；#8994 归一化独立 `terminal_info` 上下文；#9034 将 OpenAI 网络/代理可达性故障冷却缩短为 30 秒（代理鉴权拒绝保持 10 分钟）；#9060 保留尾随空 `usageMetadata` 前已采集的 Gemini 流式用量；#9076 增加默认关闭的 `gateway.antigravity_gemini_omit_include_thoughts`；#9079 补齐无版本号 Codex 身份前缀中性化；#9093/#9094/#9095 分别在本地 owner 补齐 API Key 原始 CC `reasoning_content` 显式开关（单遍线性拼接）、`image_input_models` 模态投影与 Anthropic 桥 DeepSeek 默认 `high` 推理强度。
- **后台与管理端体验**：#9080 将内容审核异步队列改为零初始堆占用的按需倍增环形指针队列，并消除热路径 `time.NewTimer` 分配；#9064 在账号今日统计与 30 天趋势中展示对齐应用时区的上游错误次数；#9088 在代理列表展示去重省/州、出口 IP 与最后检测时间；#9099 支持渠道显式开启对用户展示实际响应模型（直接命中内存渠道缓存，零新增 SQL 查询）；#8860、#8995、#9069、#9087 分别修复 Gemini 用量自动刷新键、Ops 请求错误详情调用方展示、账号弹窗代理计数加载与 `HelpTooltip` 滚动定位。

## 平滑升级与验证

本批无新增数据库 migration、Ent schema、Redis 键格式或计费持久化结构变更，`origin/main` 与本批候选镜像可共享同一 PostgreSQL 与 Redis 平滑滚动升级或回退。详细基准数据与容器验证命令见 [验证说明](../diagnostics/upstream-sync-20261127/README.md) 与 [性能记录](../diagnostics/upstream-sync-20261127/performance.md)。
