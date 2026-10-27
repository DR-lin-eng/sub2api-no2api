# 上游主线增量同步审查（2026-10-27）

本轮从本项目 `a8ced32f6d32f41156644495fde3924e1a903c26` 开始，冻结上游
`ba81c7fd758e425d67f856d601147ef0c5263d3f..15701ba187af5827d740901d53421bd69ed97717`。
仅审核这两个新增主线 PR；继承 10 月 26 日及其前序台账，不重做已适配、已覆盖和有意差异。
#8319 资源级授权继续保留原专项暂缓状态，不能把本轮 tracking merge 当作其实现完成。

## 逐项结论

| 上游条目 | 本轮结果 | 当前 owner |
| --- | --- | --- |
| #8364 Terra 目录、价格、能力、Codex 和前端预设 | 已由本项目 #161 覆盖，关闭 | `openai_model_alias.go`、模型价格资源、modelcatalog 和 admin-accounts/keys feature |
| #8364 Sol/Terra 兼容后缀与 Chat-only 工具限制 | 补齐并关闭 | `openai_compat_model.go`、三个 raw Chat 转发入口及 apicompat |
| #8339 Grok 完整目录解析与诊断 | 适配并关闭 | 现有 modelcatalog、`grok_model_catalog.go`、账号详情/同步及 admin-accounts feature |
| #8339 目录并发、旧表单与旧节点写入保护 | 复用既有实现，关闭 | `UpdateUpstreamModelCatalog`、账号 row lock、已发布迁移 249 |
| #8339 Responses 模型审计与缓冲流冲突 | 补齐并关闭 | `forwardGrokResponses`、共享响应观察器和 buffered reader |
| #8339 第二套目录快照、查询配额时新建异步目录请求、持久失败历史 | 有意差异，关闭该冻结方案 | 保留已有 OAuth 模型 worker 和手动同步；失败由现有 HTTP 错误及界面反馈，最后成功快照不变 |

机器台账在 [诊断目录](../diagnostics/upstream-sync-20261027/UPSTREAM_PRS.json)，登记 merge SHA、
原始 diff SHA-256、逐子项状态和重开条件。仅新上游语义或当前 owner 的可复现回归重开已关闭差异。

## 行为与兼容性

Terra 的标准/Fast/Flex、272K 长上下文、缓存价格、模型能力和前端模型选项已经存在，不覆盖本地价格
与 Ultrafast 适配。只补 Sol/Terra 的兼容别名，保留 `xhigh` 与 `max` 差别、显式 effort 优先和
thinking disabled 优先。Sol 的不支持档位仍走本项目原来的兼容归一化。Chat-only 账号实际映射目标
为 Sol/Terra 且携带工具时，三个兼容入口在上游调用前返回协议对应的 400，提示使用 Responses。
无工具请求及映射到其他模型的请求保留原路径。

Grok 目录拒绝 HTTP 200 的 error envelope、null/坏条目和不完整分页；接受标准对象和字符串 ID。
完整成功目录仍使用同一次有界请求、统一身份/出口路由、refresh generation、账号原子存储及 scheduler
outbox，不增加另一套 CAS 读改写循环。保存快照增加可选 `discovery_scope` 指纹，账号详情诊断只读当前
内存账号：没有 token refresh、HTTP、DNS 或额外数据库查询。列表与通用 mapper 不计算诊断。

诊断状态为 fresh/stale/source_changed/unknown。只有同来源且未过期的成功目录可声称 not_listed；
过期目录仅保留已有正向证据。通配符、未知自定义 ID、bare Grok 别名以及 runtime build ID 不能被误报为
目录缺失；图片、语音和视频标成独立能力。目录证据不参与调度、计费或白名单裁剪。

管理端新组件独立位于账号 feature，按已保存账号展示诊断；明确刷新后才发送目录请求，不使用未保存凭据。
Grok 白名单中的同步只提供可选项，保留已选自定义模型，不自动写入白名单；其它平台沿用原有同步行为。
切换账号或关闭组件后忽略迟到响应。中英文本同步，旧服务器没有新字段时显示兼容提示。

Grok Responses 将原始响应模型和冲突位带入已有可靠计费。现有渠道 response_model 模式继续只接受
不增加费用且无冲突的响应模型，默认价格来源保持原规则；不从 runtime build 名推断价格。
缓冲兼容响应复用已解码事件，仅 Grok 观察非终止模型；固定大小状态不会随 SSE 事件数增长。
模型名截断改为只扫描前 200 个 rune，并复制被截断的短串，避免新观察点为超长模型名分配整串 rune 数组。

## 平滑升级与回退

没有新增 SQL 迁移、配置开关、Wire/Ent 生成、后台任务或新增凭据。快照格式仍为 version 1，新增字段可选；
旧快照缺少 scope 时显示 unknown，手动或既有 worker 成功刷新后再显示 fresh。旧二进制忽略新增快照字段，
现有迁移 249 继续保护旧节点的陈旧 Extra 替换。普通编辑由行锁保留最新目录；身份改变时清除快照，沿用
本项目的账号身份隔离，而非复用旧来源的可用性结论。

滚动期间新 UI 请求旧节点时能处理缺失诊断字段；两版本不会因目录诊断改变选号或默认计费。需要新增
Chat-only 工具约束的客户端应改用支持 Responses 的账号。二进制回退无需 Down SQL，身份、持久 Key、
订阅和已经落库的用量保持原数据。

## 验证与证据

Docker 单元、集成、前端、生产构建、升级/回退及性能的实际结果集中在
[证据目录](../diagnostics/upstream-sync-20261027/)。命令、输入、逐字 stdout/stderr、退出状态和纠错记录
以 `VERIFICATION.txt` 和 `records/` 为准；性能是固定函数夹具，不能外推生产吞吐。

原始工作树不参与修改或回退。四个原生角色为 `MODIFIED_FILE.tar.gz`、`DIFF_FILE.patch`、
`VERIFICATION.txt` 与嵌入 pristine baseline 的可执行 `ROLLBACK.sh`；在独立副本上恢复并校验原始哈希，
使用相同命令/输入记录 BASELINE、MODIFIED、ROLLBACK，再重建修改副本。远端 PR/main CI 和发布镜像
另按实际 revision 跟踪。
