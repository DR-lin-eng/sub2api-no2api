# 上游同步审查记录（2026-08-22）

本次审查以本项目 `origin/main` 的 `d6fd2a397` 为源码基线，将
`Wei-Shaw/sub2api` 的 `main` 冻结在
`d45135d87df16d48637f04ccd245727bc955ba54`。上次冻结点是
`4033387fd49b15b676d2e5c9fd3833f156050f57`；本记录覆盖该边界之后的合并 PR、
选择性同步和后续增量复核。

## 选择性同步

- #5549：空的 `openai_capabilities` 容器按未配置处理，避免导入/升级后的
  OAuth 账号被静默排除；不改变非空能力集合的显式限制。
- #6049：Chat 粘性种子只使用首个用户轮次之前的 system/developer 前缀，忽略
  后续动态 system 消息，保持会话亲和并减少热路径字符串拼接。
- #6016：Prompt Guard 配置刷新只在首次加载、版本/风险开关变化或错误恢复时记录
  `config_loaded`，不再把固定周期刷新变成日志心跳。
- #6048：Composite 分组保留 Messages 调度配置；目标为 Grok 或已支持的 CN provider
  时自动放行并使用 provider 原生映射，目标为 OpenAI 时继续要求分组显式开关。管理端
  表单同步支持 Composite。
- #5654：Composite 的视频生成创建请求与已有状态/内容查询一致，交给 Grok handler。
- #5612/#5625：付费 Antigravity 账号才走官方 daily 端点，并将 daily 域名更新为
  `daily-cloudcode-pa.googleapis.com`；免费账号和旧配置仍走生产端点，显式环境变量优先。
- #5954：桥接路径按映射后/原始模型识别 DeepSeek、Kimi、GLM 等原生 `max` 推理强度，
  其他模型继续沿用 `max -> xhigh` 兼容规则。

## 不重复或暂缓

- #5888 及其后续兼容修复没有整包移植。其 164 个旧 `handler/service` 文件的
  failover、WS、Responses、工具和运维日志改动，与本项目已模块化的
  `application/service`、`transport/http`、有界重放和错误语义重复；整体合入会造成
  双重重试、路由漂移和热路径额外解析。
- #5911/#5913/#5919/#6009/#6011 依赖上游独立 CN provider owner/迁移，本项目没有对应
  的 `internal/service` 目录，未复制旧路径。
- #5906 仅修上游测试 fake 的并发 append；#5662 文档链接和 #5708 Model Plaza
  已在本项目分别修正/完成，不重复处理。
- 其余 Antigravity、Grok、Responses 和 OpenAI 小修在当前代码中已有等价实现，按函数
  行为和测试覆盖确认后跳过。

## 性能与升级边界

本次没有新增数据库迁移、后台任务或无界队列。Prompt Guard 刷新仍保持原周期，只减少
重复日志；会话种子和模型能力判断均为有界内存操作；Composite 目标映射在请求上下文中
完成，不增加数据库查询。Antigravity 付费路由只读取已持久化的 `plan_type`，显式旧配置
优先级保持不变。旧 JSON、SSE 和 WebSocket 契约不变。

## 验证入口

代码与测试是最终事实来源。提交前运行后端聚焦测试、前端 typecheck/Vitest、文档检查和
Docker 构建/启动回归；发布后分别按推送提交的精确 SHA 核对 `CI`、`Security Scan` 和
`Docker Image`，不能用本地 Docker 结果替代 Actions 证据。

## 本轮增量复核（origin `1532bae4` / upstream `d45135d8`）

在上述记录之后，上游又合入 #6068（Codex guardian parent affinity）。该 PR 引入新的
调度层、隐私门控和 parent sticky 绑定，涉及 700 余行旧 `internal/service` 代码，
会改变账号选择、缓存绑定和多实例行为；当前模块化项目没有等价 owner，本轮只完成
只读性能/升级审查，暂不移植。

本轮选择性接入了仍未被当前主线吸收、且能落在现有 owner 的窄修复：

- #5846：Responses→Chat 历史和流式桥在终态拒绝畸形 JSON 工具参数，并跳过对应孤儿
  输出，避免截断的调用污染后续请求。
- #5487：Chat `file` 内容块转换为 Responses `input_file`，保留文件名、`file_data`
  和 `file_id`。
- #5864：HTTP bridge 重放前清除没有匹配输出的历史工具调用；沿用现有 512 项/2 MiB
  有界回放限制。
- #5685：Anthropic 兼容 Chat/Responses 池模式错误在账号未被停用时允许有限同账号重试。
- #6065：OAuth 图片生成无图但返回普通文字时标记图片能力不可用，写入已有模型级
  冷却并切换账号；明确安全/审核文字仍返回 400，并发送 `OpenAI-Beta` 响应实验头。

这些改动没有数据库迁移、后台任务或无界队列；工具参数校验和文字分类都在已有协议
边界执行，普通请求不增加外部 I/O。上游 #6067 Ollama、#5912 DeepSeek、CN/Composite
默认路由和账单相关改动仍暂缓，避免重复重构或改变滚动升级口径。
