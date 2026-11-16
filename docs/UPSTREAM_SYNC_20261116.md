# 上游主线增量同步审查（2026-11-16）

本批已由 [PR #216](https://github.com/DR-lin-eng/sub2api-no2api/pull/216) 合入主线
`b617f7e37d8fac78cecb2251a472d8388b896a22`。本页补齐原索引缺失的审查摘要；
逐项决定和原始验证以[机器台账](../diagnostics/upstream-sync-20261116/UPSTREAM_PRS.json)
及[验证记录](../diagnostics/upstream-sync-20261116/VERIFICATION.md)为准。

## 冻结范围与关闭规则

本项目基线为 `d8d23aa16663e2262f51e9543062b90812c33c3c`，上游范围为
`530c5e3a4f9c73ca2b0881191d0ef8e0d82c5484..8ed701382cbc4eadecc0de0684698ad85cf7cce0`，
共 61 个首父链 PR：44 项适配、5 项复用、12 项保留本地契约或不采用。
[198 项历史台账](UPSTREAM_DEFERRED_STATUS.md)保持原样。

每项保存固定 merge SHA、净 diff SHA-256、当前 owner、决定和重开条件。
`diff_review_closed=true` 只关闭这份冻结差异的审查；16 项
`capability_complete=false` 不代表功能已实现。相同 PR 和补丁 hash 不重复移植；
新补丁、当前 owner 的已证实回归或明确的新能力任务才触发重开。

## 实现与性能

按当前 application / infrastructure / shared / feature owner 修复额度快照、
调度冷却到期恢复、批次查询隔离、分组倍率事务与协议转换。
新增 Gemini Hydrogen 的目录、思考能力与价卡；管理员显式映射继续优先。

OAuth 历史元数据采用局部修改后一次拼接，1000 条固定历史的 Docker 中位耗时
由 52.626 ms 降为 0.657 ms，分配由 126.7 MB 降为 0.46 MB。
500 个 Anthropic delta 的局部测试由 1.385 ms 降为 0.554 ms。
基线、候选和回退均有独立三次样本，见[性能记录](../diagnostics/upstream-sync-20261116/PERFORMANCE.json)。
这些结果衡量局部固定输入，不代表真实模型延迟或生产吞吐。

保留 PostgreSQL 持久幂等结算；未采用无容量上限的计费 lane。
调度继续隔离请求对象，不采用共享可变嵌套对象、20 秒旧快照宽限及缺少混合版本证明的 memo/coalesce。

## 升级与回退

本批不增加数据库 migration、Ent/Wire 生成结果或队列格式，不重算历史账单。
全节点升级后再依赖批量写入修复和新增模型能力；旧写入节点仍有原行为。
关闭 ping、新套餐识别和会话 hash 的效果以实际处理请求的节点版本为准。
回退使用原二进制和同一数据库，无需执行 Down SQL。

未采用上游新的默认 TCP/HTTP2 超时、region 自动回退及默认加价、自动 token 轮换重放、
原始 keepalive 旁路、首输出前额外重放和非 CAS 主动清冷却。
价卡 override_file、Seedance 独立任务与新身份 namespace 等能力边界详见机器台账。

## 验证与发布

原批次 Docker 全后端 unit、PostgreSQL/Redis integration、前端 2637 测试、lint/typecheck、
后端 lint 与 race 专项通过；正式镜像同库基线→候选→回退→恢复共 70 个 HTTP 场景通过，
各阶段 317 项 migration，身份、Key、订阅、历史账单摘要和余额等式保持一致。
命令、退出码、首次失败及最终日志均保留在[验证目录](../diagnostics/upstream-sync-20261116/README.md)。

主线合并 SHA 的 CI、Security Scan 和 Docker Image 工作流已于 2026-11-16 通过。
后续 7 个 PR 与一个直接提交独立记录在[后续批次](UPSTREAM_SYNC_20261116_FOLLOWUP.md)，
不改变本批已冻结决定。
