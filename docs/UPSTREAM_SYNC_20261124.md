# 上游主线增量审查（2026-11-24）

本项目基线为 `223901a83`，上游增量为 `328db26f4..0f6671457`。首父链包含 #9008、#9045、#9052 和一个版本提交。每项的固定提交、净 diff SHA-256、当前 owner、关闭状态和重开条件见[机器台账](../diagnostics/upstream-sync-20261124/UPSTREAM_PRS.json)。台账继承 2026-11-20 的历史摘要，已处理范围不重复扫描。

**已合入的本地适配。** #9045 的当前 owner 已经覆盖有界透传 failover、`invalid_encrypted_content` 续接、compact SSE 失败事件、规则匹配和错误净化。本批补齐 Responses `response.error` 在非流式 400 错误中的 message、type、code、param 保留。错误路径只增加有限字段读取，不触及成功请求热路径。上游分支重复携带的 gorilla/websocket 安全版本只合入一次，当前项目从 `v1.5.3` 升到修复 GO-2026-6278 的提交版本。

**保留本地契约。** #9008 把上游分组名称和描述写回账号名称/备注，并增加自动同步和编辑器预览。本站的探测、账号 Extra 和 `admin-accounts` feature 采用不同 owner。自动写回会增加周期性上游请求、持久化竞争和管理员可见名称变化，需先定义 opt-in、CAS 和回退边界。#9052 把请求头身份变量、错误记录身份头和 WebSocket 身份分桶作为一个整体引入。该功能会改变存量覆写值的解释、错误日志敏感信息和连接池容量，等待独立契约后再重开。上游 `0.2.25` 版本号继续由上游仓库管理，本项目保留自己的版本解析。

**性能与升级。** 本批没有新增 migration、Redis key、队列格式或持久账单字段。滚动升级可以先替换网关节点，再恢复流量。回退不执行 Down SQL，也不需要恢复账号 Extra。详情见[验证说明](../diagnostics/upstream-sync-20261124/README.md)和[性能记录](../diagnostics/upstream-sync-20261124/performance.md)。

**验证。** `verify_ledger.py` 校验上游首父提交、固定文件列表、净 diff 摘要和当前 owner。Docker Go 1.26.11 中已通过 application service 全量测试，以及 HTTP handler、admin handler、DTO、server middleware 和 server/routes 测试。真实上游凭据、生产数据和生产吞吐未验证。
