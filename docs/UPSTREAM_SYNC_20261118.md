# 上游主线增量审查（2026-11-18）

本批冻结本项目主线 `b22b8c95d0388dd94682d92bf8b8986fb90c4ab1`，上游范围为
`15871f904056f548dd874a1d069ffc6cdf777b78..2af3f1e15c98c893a89255a037f551046883b8c0`。
8 个 PR 与 1 个版本提交的首父链、净 diff SHA-256、当前 owner、能力边界及重开条件保存在
[机器台账](../diagnostics/upstream-sync-20261118/UPSTREAM_PRS.json)。四份继承台账原始字节保持不变，
其 268 个历史 PR 不因本轮同步再次移植。

| 上游 | 当前处理 | 能力边界 |
| --- | --- | --- |
| #8906 | 复用本项目 #219 的 GPT-6.2 适配 | 保留现有目录优先级、基础指令和推理档位规范化，不引入另一套静态 descriptor |
| #8904 | 复用已重构的 DeepSeek 线性改写 | 本地没有该补丁针对的 Ollama 专用请求改写入口，不新增运行面 |
| #8903 | 在现有性能模式下按 Read 合并 SSE Flush | 普通模式、预输出暂存、断连排水、`include_usage` 保持原契约 |
| #8598 | 关闭不适用的测试补丁 | 本项目没有采用该 inflight reservation 系统，不新增余额预占 |
| #8914 | 保留当前渠道计价契约 | 独立渠道 Ultra Fast 倍率未引入，不重开此前未采用的 Fast/Flex/推理倍率字段 |
| #8577 | 单次 timer 按最近读取重新定时 | 原 idle timeout、语义输出与换号门禁继续生效 |
| #8174 | 适配缓存用量、TTL 快照及互斥计费桶 | 不新增渠道 TTL 单价或 OpenAI 30m/12h 计价；保留本地零占位和缓冲 EOF 兼容 |
| #8920 | idle failover 复用首输出的一次换号预算 | keepalive 可重试，已有语义输出或客户端断连时不重放 |
| `2af3f1e15` | 保留 fork 版本 | 不覆盖 VERSION，不创建发行标签 |

全部冻结差异均设置 `diff_review_closed=true`；只有按当前范围完整实现的项标记
`capability_complete=true`。同一 PR/hash 的关闭不代表功能对等，也不是以后重开的理由。

## 缓存与协议

Anthropic 的累计 usage 可能只给出部分 5m/1h 快照，后续补齐另一 TTL。
新 owner 合并缺失/null/零的区别，保留已收到的正聚合与缓存读取计数；沿用已关闭 #6132 的
delta 零值不覆盖正明细契约，不采用本补丁的零值清除行为。没有聚合时从累计明细推导；
有聚合时，矛盾明细仍由既有比例限幅处理，
聚合中未被明细覆盖的余量按 5m 计价。例：聚合 100、5m 20、1h 30，实际分配为 5m 70、1h 30。

provider 的 `prompt_tokens` 总量与缓存桶归一化为互斥输入，`prompt_cache_miss_tokens` 显式值优先。
HTTP JSON、SSE、原生 Anthropic 与 Chat/Responses 转换共享当前规则；客户端断开后仍排水并结算。
渠道/分组显式价格和账号统计的单一 cache-write 价格继续执行，不通过本补丁引入额外收费字段。
存量结算队列、请求幂等 ID、槽位、粘性会话及失败账号排除的 owner 不被替换。

## 性能审查

不重复替换已完成的 DeepSeek 局部 JSON 改写。raw usage 解析将不含 usage 的文本 delta 提前返回，
每个 usage 对象一次遍历，以值与 presence 标记合并计数，避免临时指针分配和重复 TTL 合并；
不新增外部 I/O 或无界内存集合。
缓存快照每次只更新固定计数。SSE 性能模式在上游下一次读取前刷新，既合并同批已到达事件，
也不等待未来事件才发送首个输出；原单元夹具从 4 次 Flush 降为 1 次，普通模式仍为 4 次。
idle timeout 从 ticker 改为一个 timer，不增加每请求 worker 或 timer 数量。

固定 Docker 工具链、同 CPU 预算的实测见[性能记录](../diagnostics/upstream-sync-20261118/PERFORMANCE.json)。
同一 linux/arm64、Go 1.26.11、1 CPU，每组五次采样的中位数如下：

| 实际 raw usage parser 输入 | 主线 | 候选 | 两者 B/op / allocs/op |
| --- | --- | --- | --- |
| 256 字符文本 delta，无 usage | 748.5 ns/op | 227.4 ns/op | 0 / 0 |
| 含聚合与两个 TTL 的 usage 快照 | 799.8 ns/op | 646.5 ns/op | 0 / 0 |

初始适配的缓存快照出现 16 B/op、2 allocs/op，最终以固定值快照和单次遍历去除；
初始及最终原始样本均保留。计费正确性没有用采样或丢弃 usage 换取性能。
采样只衡量实际 usage 解析函数，不代表生产网络吞吐。完整验证与失败修正记录见
[证据目录](../diagnostics/upstream-sync-20261118/README.md)。

## 平滑升级与回退

不新增数据库 migration、Ent/Wire 生成结果、Redis key、鉴权快照版本或队列封装版本。
新增的请求内 TTL 投影使用已有用量明细列；旧二进制能忽略 JSON 中新增的可选明细。
历史费用不重算。源码生产镜像验证仍包含嵌入式前端构建。

缓存解析和聚合余量修复会纠正新请求的费用，不能保证混合版本缺省收费完全一致。
先升级处理相关 Anthropic/兼容流量与异步结算的节点，再恢复这些流量的跨节点分发。
若必须同时运行旧节点，将受影响的流量和结算任务保持在修复后的节点上；单纯设置相同单价
不能补回旧 parser 漏掉的 token。回退前同样需要保持这一路由隔离或暂停受影响流量，
不执行 Down SQL，不重放已结算请求。

性能模式保持原管理员设置；新旧节点只在 Flush 粒度及实际 idle deadline 上不同。
空闲超时换号仍受现有安全重放判断和一次换号预算约束。生产上游权限、真实并发吞吐和实际
多机滚动发布不属于临时假上游夹具的验证范围。

同库夹具执行基线→候选→回退→恢复，核对身份、API Key、订阅、显式价格、迁移摘要、历史账单
及余额等式。实际结果与精确 SHA 的 CI 链接见[验证记录](../diagnostics/upstream-sync-20261118/VERIFICATION.md)。
