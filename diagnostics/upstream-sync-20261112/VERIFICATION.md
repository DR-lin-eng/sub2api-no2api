# 验证记录

冻结本项目主线 `0f1836b5a3db2ccdf8d4ab985d199215e45f39a2`，上游至 `530c5e3a4f9c73ca2b0881191d0ef8e0d82c5484`。
逐项决定见 [台账](UPSTREAM_PRS.json)。本文件在发布前按最终输出补全，不把尚未完成的检查记作通过。

## 前端

Docker Node 24 / pnpm 11.17.0 的 ESLint、typecheck、关键路径集，以及受影响的四个 feature
119 文件 / 926 用例通过。冻结基线的 OpenAI 配置和 Codex 设置卡模板已有过期 golden；
[精确对照](FRONTEND_CONTRACTS.json) 证明源码及输出一致后，仅更新对应精确摘要。
Claude 5.2 配置增加 native-only 语义断言，Antigravity 保留原模型输出。

首次后端 lint 在 2200 MiB 容器中因内存限制退出 137；保留原记录，后续改为顺序执行与有界编译并发。


## 后端、运行态与兼容

Go 1.26.11 Docker 全包 unit、官方 MCP Go SDK OAuth 互操作检查通过，golangci-lint 2.9.0 为 **0 issues**。
完整 integration 的业务包（包括 PostgreSQL/Redis repository）通过；唯一首轮失败来自外部
`TestJA3Fingerprint` 探针的瞬时非 TLS 响应。冻结基线和候选单独重试均通过，候选实测 JA3/JA4 匹配；
本地全包复跑明确排除该外网探针，另在隔离 `prompt_audit` 数据库执行完整真实审计存储/队列/CAS 测试并通过。
原测试源码未修改。临时 PostgreSQL 初始化首次误命中临时 Unix socket，runner 改为 TCP readiness 后通过。

冻结基线与候选均按根 Dockerfile 正式构建。数据库四阶段 **BASELINE → MODIFIED → ROLLBACK → RESTORED**
42 个 HTTP 场景全部通过，见 [result.json](runtime/result.json)。迁移数为 **314 → 315 → 315 → 315**；
新索引有效，旧二进制继续读取相同数据库。身份/密码摘要、API Key、订阅、历史用量及余额结算等式保持一致。
每次成功模型请求只结算一条用量：Responses JSON/SSE 均为 1000/100 token、$0.004；图片两张为 $0.20。
新节点正确展示 Claude 5.2、保存读回 `0.140845` 并规范化 page context，回退节点恢复旧行为。
每个候选阶段 8 路并发刷新仅 1 成功 / 7 拒绝，顺序轮转与复用拒绝在四阶段均通过。

最终新增的极小正倍率边界单独复验：小于六位精度、会被容差误放行后保存成零的值必须拒绝；
`0.000001` 和最大有限浮点仍可保存读回，金额继续按币种精度结算。构建遇到一次 Docker Hub token EOF，
不改变依赖或 Dockerfile；环境重试记录单独保留。

## 性能

禁网 Go Docker、2 CPU 配额、benchmark 固定 1 CPU、300ms × 3，取中位数；同一 fixture 在冻结基线、
候选与回滚源码上运行。首轮与编译并行的样本保留为 `PERFORMANCE_INITIAL.json`；下表使用无其它重编译
时的 [PERFORMANCE.json](PERFORMANCE.json)。B/op 与 allocations 在三阶段完全一致。

| 固定夹具 | 基线 | 候选 | 回滚 | B/op |
| --- | ---: | ---: | ---: | ---: |
| 普通 1 MiB bootstrap | 299.804 μs | 275.976 μs | 282.570 μs | 0 |
| 大历史 Responses 会话种子 | 332.811 μs | 334.188 μs | 334.327 μs | 1752 |
| 大历史 Chat 会话种子 | 370.164 μs | 333.111 μs | 333.848 μs | 2288 |

普通 bootstrap 保持零分配，新增 page 功能未引入普通请求的完整 JSON 解码；种子路径未显示实质性能退化。
这些是局部 CPU 夹具，不能外推真实上游延迟或整站吞吐。

PostgreSQL 18.1 固定 30 万条、每条 1 KiB 宽 payload、10 Key 批量，查询包含 actual_cost 和四类 token。
基线/候选/移除索引对照中位执行时间 **1.972 / 0.491 / 1.656 ms**，结果摘要一致。
候选使用 Index Only Scan，Heap Fetches=0，shared hit blocks 从 691 降至 111；新增索引 **23,134,208 bytes**。
这是 VACUUM 后的固定内存命中场景；生产的可见性图、冷热缓存、写入成本、索引构建 I/O 与磁盘占用需独立评估。
运行态回滚保留索引；这里只在一次性性能库中移除索引以证明对照。

## 浏览器

真实正式镜像页面在 1280×800、390×844 上验收：保存区在视口底部可见；六位倍率中英文预览正确；
EasyPay 默认码 1、显式 0 的选择正确；刷新时 21 行保持且 `aria-busy=true`。
手机原有取消频率控件横向溢出已按当前组件补 `flex-wrap`，修复后 document width=382（含滚动条的 viewport=390）。
见 [UI.json](UI.json)、[手机截图](mobile-payment.jpg) 和 [桌面截图](desktop-payment.jpg)。
浏览器唯一捕获的错误来自主动重建临时服务/JWT 后的旧 session 自动刷新；重新登录后的验收正常。
设置检查只编辑草稿，不对生产写入。最终手机修复后 ESLint/typecheck、31 文件/283 关键测试和
21 文件/150 设置测试再次通过。

`make check-docs`、源码布局与空白门禁通过，未扩大任何 allowlist。前端与后端文件按职责拆分；
未修改 Ent/Wire 生成文件、锁文件或运行依赖。远端 CI 和 GHCR 另以精确推送/合并 SHA 的记录为准。
