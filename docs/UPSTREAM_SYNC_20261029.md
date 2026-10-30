# 上游主线增量同步审查（2026-10-29）

本轮以本项目 `c050e6cbf4d8c8fee4609f15171181b3e521714d` 为基线，冻结上游
`f178e5ae32659e8901644a9f2378a0439ffa36ae..a3298b46ebe239fb0720b068a1eb3383955bdac2`，
共 7 个主线 PR、1 个依赖提交和 1 个 VERSION 提交。保留另一位 agent 已合入的 API Key Bloom
和 vue-i18n 安全更新。此前已关闭差异不重做；已有专项暂缓不因 tracking merge 变为已实现。

| 上游条目 | 本轮决定 | 当前 owner 与边界 |
| --- | --- | --- |
| #8352 管理员保护 | 适配并关闭 | `admin_user.go` 对禁用/删除管理员、最后管理员降级返回 400 和稳定 reason；拒绝时不落库 |
| #8390 渠道 Web Search 维度 | 适配并关闭 | `channel_service.go` 对该键按平台合并，缺省继承、显式空对象/null 清空；其它键仍整块替换。前端 codec 复制未编辑维度，按本项目实际支持显示 Anthropic/OpenAI 开关 |
| #8395 Gemini 模型元数据 | 适配并关闭，保留兼容差异 | `gemini_declared_models.go` 复用现有调度快照/仓储读取；全部 Gemini 账号有映射时返回映射并集，包含混合调度的 Gemini 模型；通配符、customtools、白名单按当前规则处理。无映射/查询失败走原透传路径 |
| #8287 Gemini 同号重试配置 | 适配并关闭 | 新增 `gateway.max_same_account_retries_gemini`，默认 4、范围 0–10，不含首次请求；三条转发路径共用，签名降级独立计数，已有传输错误重试也遵守该预算 |
| #8369 显式缓存同号重试 | 继承 #8006 专项暂缓，未实现 | 本项目尚无 cachedContents 资源、token-hour 可靠结算和账号绑定 owner；不引入孤立的绑定缓存重试分支。原专项条件仍按 10 月 13 日台账执行 |
| #8373 manifest 并发冷启动 | 适配并关闭 | `openai_codex_models_service.go` 在 singleflight 内复查新鲜缓存，避免先读 miss、迟到进入刷新造成重复上游调用 |
| #7979 OAuth 非流式 Responses | 适配并关闭 | 保留客户端 stream 标志，OAuth 出站继续 SSE；普通/透传非流式路径对 completed/incomplete/cancelled/canceled 聚合 JSON，覆盖聚合后的媒体类型，保留最终 request ID 和可靠用量 |
| 0a2a29b32 依赖修复 | 已覆盖，关闭 | 基线已使用 x/net v0.62.0，保留当前依赖集合 |
| a9240d501 VERSION | 有意差异，关闭 | 沿用本项目版本线及发布工作流 |

[机器台账](../diagnostics/upstream-sync-20261029/UPSTREAM_PRS.json) 保存每个 merge SHA、原始 diff
SHA-256、当前 owner、关闭或专项状态和重开条件。#8006 与 #8319 等既有暂缓没有被虚假关闭。

## 平滑升级

没有新增 SQL migration、Ent/Wire 生成、持久字段、凭据格式或缓存格式。新增配置由 YAML 和
`GATEWAY_MAX_SAME_ACCOUNT_RETRIES_GEMINI` 读取，示例与 Compose 同步；默认值保持原来的
首次请求加 4 次同号重试。显式 0 表示不做瞬时错误重试。签名兼容降级最多两次，独立于该预算；
默认配置中同时遇到两阶段降级和瞬时错误时，最多 7 次请求。

Gemini 补齐新的模型候选和 Flash Lite Image 识别，保留当前旧模型 ID、已保存白名单和映射。
现有价格数据没有为全部 2.5 条目标注退役日期，不能仅凭上游目录删除它们。默认账号测试改为
3.5 Flash，旧 Google One 未指定模型的测试继续使用其原模型。组合/强制平台保留原元数据路径。

声明模型与透传回退共用一次请求内账号读取，不写持久快照、不启动 worker，不访问模型上游；
排队后的账号资格复查仍由既有准入 owner 执行。渠道新前端写显式 false，旧前端遗漏维度时由
新后端保留。回退旧后端后仍须避免使用旧前端覆盖多平台 Web Search 配置。

Responses 的 incomplete/cancelled 聚合只在新节点生效。混合版本期间，同一非流式请求可能
获得 JSON 或旧 SSE，应完成全部节点升级后再依赖新行为；已落库账单不重算。二进制回退无需 Down SQL。

## 性能

manifest 新增一次内存缓存读取，减少冷启动窗口内重复请求。Gemini 元数据复用现有账号源，
用一个集合去重，最后一次排序；透传回退不再次访问仓储。该逻辑只用于 metadata，推理入口不会
构造目录集合。账号上下文不跨请求缓存，避免引入失效协议与来源污染。

非流式终止响应提取合并为一次 SSE 遍历，保留 compact/image 调用方原来的 completion-only
提取契约。渠道合并只复制 Web Search 平台维度，不进行通用深合并。量化结果与资源条件以
[本轮验证记录](../diagnostics/upstream-sync-20261029/VERIFICATION.md) 为准，不外推生产吞吐。

## 验证与发布

精确命令、完整压缩输出及退出码保存在 [records](../diagnostics/upstream-sync-20261029/records/)。
Docker 验证覆盖三条 Gemini 重试路径、metadata 原透传回退、单次仓储读取、OAuth 两条转发路径
的四种终止状态、缺失 Content-Type、用量、渠道维度和管理员保护。完整单元、集成、前端、性能、
生产构建、同库升级回退、UI 宽度检查和精确 SHA CI 已完成；最终命令、退出码、摘要和哈希见验证 records。

源码行数门禁中的 payment_fulfillment_test.go（1201）与 pricing_service.go（1218）和基线相同，
本轮没有扩大 allowlist 或重复重构它们。最终 Docker 全单元、前端 lint/typecheck/build、同库四阶段升级回退、UI 桌面/移动宽度和 PR #168 远端 CI 均通过。
