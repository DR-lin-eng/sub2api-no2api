# 上游主线同步审查记录（2026-09-15）

本记录冻结本轮上游审查、选择性移植、渠道补齐和已关闭差异。源码、测试、Docker 与最终 CI 是事实源。

## 冻结点与审查范围

| 项目 | 值 |
| --- | --- |
| 初始下游基线 | `origin/main` @ `16d03822490fedbec5b0a26dced55c35970d6fe5` |
| 发布合并基线 | `origin/main` @ `69810eab4e2bfee8cb002ae9594116a845ff2078`（PR #73，变更文件与本轮功能零重叠） |
| 上一审查点 | `upstream/main` @ `bdb42e22f81fcb633ff0a060961211dd2bcb515b` |
| 功能冻结点 | `upstream/main` @ `badfad8b7248b8aac0e6b503a06e392aa31cb294` |
| 最终上游关闭点 | `upstream/main` @ `32682a4f84c6a29104439050a0be14737552cec2` |
| 增量范围 | `bdb42e22f81fcb633ff0a060961211dd2bcb515b..32682a4f84c6a29104439050a0be14737552cec2` |
| 增量规模 | 45 commits，18 个 first-parent 合并 PR，1 个直接 README 提交 |
| 分支 | `codex/upstream-sync-20260915` |

上一轮代码语义已审到 `bdb42e22`，但 Git 提交图没有把该 SHA 记为第二父提交。本轮在完成语义移植后使用 tree-preserving `ours` merge 把 `badfad8b` 纳入祖先；CI 等待期间新增的 `32682a4f` 也已单独审查并纳入祖先，后续只需从最终关闭点向前审查。

## 本轮增量 PR

| PR | 结论 | 本项目实现 |
| --- | --- | --- |
| #7112 | 已移植 | 自动刷新按当前选择的间隔复位，并在最后一秒触发，不再固定回写 30 秒或多等 1 秒。 |
| #7073 | 已移植 | 批量生图账号优先级改为数值越小越优先；同优先级继续按 ID，计费与冻结公式不变。 |
| #7085 | 已移植 | 仅在 OpenAI OAuth input item 边界删除 `internal_chat_message_metadata_passthrough`，不触碰同名用户内容或 API-key 请求。 |
| #7111 | 已移植 | Proxy username/password 用字段存在性区分“省略保留”与“显式空串清除”，导入路径保持原值。 |
| #7082 | 已移植并补升级清理 | Antigravity token cache 改为 account ID 隔离；失效时同时删除旧 project key，适合滚动升级。 |
| #7094 | 已移植 | Responses 降级 Chat 时合并开头 system/developer，并把会话中途的 system/developer 降级为 user，兼容严格上游。 |
| #7076 | 已移植 | Antigravity token 刷新部分成功时同时返回更新后的 account 和 warning，前端保持列表新鲜并展示告警。 |
| #6925 | 已移植并与现有终态恢复合并 | 从 output-text done 恢复缺失尾部；只在无重复风险时从 terminal response 恢复文本。 |
| #6689 | 已移植 | Antigravity 不再混发 builtin web search 与客户端 function tools；混合时保留客户端工具。 |
| #6654 | 已移植 | 注册页增加密码确认与一致性校验。 |
| #7126 | 已移植 | API-key Responses Lite 识别 `input[].additional_tools` namespace 声明，保留匹配的历史工具调用 namespace。 |
| #7072 | 已移植并去重 | 选号耗时通过命名返回值真实返回；sticky ratio 用单一命中计数，避免 previous/session 双重计数。 |
| #6257 | 已移植 | Email/GitHub/Google 与 LinuxDo OAuth 注册继续携带已输入的 promo code。 |
| #7110/#7148 | 已关闭，无净变化 | 上游先加入后完整撤销 batch-image access cache 变更，不导入往返提交。 |
| #6691 | 暂缓 | API Key 批量编辑是独立用户写入面；本项目已有不同的 keys feature owner，需要单独的后端授权和交互审查。 |
| #7091 | 暂缓 | 订阅多动作批处理会扩大幂等、事务和恢复边界；现有批量分配保持不变，另立专题移植。 |
| #6769 | 暂缓 | Ollama 异步 rate-limit reset 引入额外探针、CAS 与后台调度；需与现有 Ollama session/refresh owner 单独压测。 |

上游直接提交 `32682a4f` 只调整三份 README 的赞助链接和赞助商表格。本项目三份 README 均保持无赞助广告规范，因此不导入内容，并由 tree-preserving merge `32f828212` 关闭差异。

## 渠道适配补齐

上游 #5666/#5730/#5773/#5817/#6758/#7062 的旧目录实现没有在此前模块化迁移中形成完整用户入口。本轮按当前 owner 重新接线：

* Kimi、智谱 GLM、DeepSeek、MiniMax 作为独立 `platform` 进入账号、分组、Composite、渠道定价、用户平台额度、Ops 筛选和渠道监控。
* 创建和编辑账号提供各平台官方 API Key 默认地址、平台化提示与 Key 占位符；自定义 `base_url` 继续可用，旧 OpenAI API-key 账号不迁移、不改写。浏览器验收发现并修正了新平台误用 Anthropic 提示的问题。
* 四个平台复用现有 OpenAI-compatible Responses/Chat/Messages bridge。默认上游出口使用各家共同支持的 Chat Completions，避免对智谱等平台盲发 `/responses`；Claude Code/Codex/OpenCode 的入站协议仍由现有 bridge 转换。
* `/messages/count_tokens` 使用本地估算，不向没有该端点承诺的供应商发送探针，不把常态 404 误记为账号故障。
* channel monitor 复用 OpenAI-compatible chat adapter；Responses probe 仍只允许 OpenAI。
* 迁移 `243_add_cn_provider_platforms.sql` 只放宽四张表的 CHECK 超集，不改旧行。三档额度全空时不建行，避免平台从 5 增至 9 后制造空记录和无效 cache 数据。
* 上游独立 CN balance/coding-plan 抓取服务未直接复制；它会新增 credential-bearing 后台请求和另一套 service/repository owner，需单独审查后再接入。

## 性能与升级边界

* 请求热路径的平台归一化为常数时间 switch。最终合并树在 Apple M4 上五轮 benchmark 为 `1.211-1.375 ns/op`、`0 B/op`、`0 allocs/op`。
* scheduler canonical bucket 数从平台表动态推导，预分配容量同步从固定 12 改为动态值，避免新增平台后的 slice 扩容；测试不再保存 5 平台魔数。
* Composite/全量 snapshot 会处理 9 个平台而非 5 个，但仍为固定有界配置/生命周期工作，不进入每请求数据库扫描。
* 没有修改 API JSON/SSE/WebSocket 的既有字段，新增平台与 warning 字段均为可选；旧节点可继续读取旧行，旧客户端可忽略新枚举。
* Antigravity cache key 在新节点切为 account scope，同时清理旧 project scope key；滚动期间不会复用另一个账号的 token。

## 验证与发布

完整命令、原始结果、Docker、回滚、PR 与 exact-SHA CI 状态记录在 `diagnostics/upstream-sync-20260915/VERIFICATION.txt`。发布后只以最终 merge SHA 查询 CI、Docker Image、Security Scan 和 GHCR manifest。

首轮远端 lint 捕获到新增测试未检查嵌套类型断言结果；最终实现拆分并逐级断言，单包 `errcheck` 已返回 `0 issues.`，以修复后的 SHA 重新执行全量 CI。
