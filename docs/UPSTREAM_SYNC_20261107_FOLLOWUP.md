# 上游主线后续增量审查（2026-11-07）

基线为本项目 `1e629a9a75e043ae0a888a9231cf7c88ad8f37e8`，冻结上游
`0eee80ba87aa4f3aff7282bf1695aa6dc0471c36..6b4b37db43443b5e88e424fe3c69602cc3810dfb`。
本轮覆盖 20 个新增合并 PR，赞助者和上游版本号保留本项目独立元数据。
上一轮已关闭的实现不重新移植，仍按当前模块 owner 适配。

| 上游 PR | 决定与边界 |
| --- | --- |
| #8563 | 补齐缓冲 Responses reasoning content；summary 优先，跨输出项用 builder 线性累积，保留签名、工具和用量。 |
| #8429 | 对齐 message JSON/SSE 的空 text、annotations、logprobs、refusal 和空 content 数组。 |
| #8531 | Antigravity 账号测试使用管理员提示词，缺省 hi，输出预算 256 token。 |
| #8521 | 图片 INTERNAL 500 只冷却实际模型；参数进入既有 config/defaults/env/validation，保留文本账号默认惩罚和暂停总开关。 |
| #8437 | 换号耗尽的 413 先匹配管理员透传规则，默认脱敏错误及换号预算由当前 owner 管理。 |
| #8430 | Gemini 3+ 的工具媒体放入 functionResponse.parts；继续复用本地单次解码，旧模型保留兼容布局。 |
| #8557 | 新增 2.6 Flare/Sunburst 原生 Images 路径、参数、GIF/SVG MIME、JSON 转完成事件及准确图片缓存计价；集中改写多图响应。 |
| #8457 | 在现有 chunk reader 上记录失败进度，区分 408、499、413、截断及解压错误；中间件和 handler 保留协议格式。 |
| #8522 | 增加 Antigravity 分组的真实 5h/周额度，1 MiB/32 组/每组 8 窗口上限、2 秒独立查询时限，失败回到既有模型额度；使用静态译文映射。 |
| #8553 | Anthropic setup-token 主动/被动展示复用持久周额度；429 和只有周窗口的响应头仍被采样，无额外 profile API 请求。 |
| #8441 | 修复直接/信封两种 Antigravity 响应形态，以字段探针后单次类型解码适配现有 typed sink。 |
| #8432 | 当前账号图片测试已不发送 response_format，关闭已覆盖差异。 |
| #8445 | 强制 Responses/Chat 模式不再发起无效能力探测。 |
| #8520 | ID-less token delta 为每个活跃 WS turn 独立记时，保留本项目 Codex 首事件策略和 response.created 计时起点。 |
| #8464 | HTTP bridge 已显式传递账号级 fingerprint 和 turn 快照，关闭已覆盖差异。 |
| #8471 | 无效 UTF-8 在缓存/数据库前拒绝；取消的鉴权查找返回 499，保留现有 bloom/L1/L2/singleflight。 |
| #8409 | Antigravity 的 responseJsonSchema 规范化并清理，已有 responseSchema 优先。 |
| #8420 | 仅精确 HTTPS Cloudflare account REST Messages 端点省略 beta query，统一正式/透传/测试构造器。 |
| #8380 | 导出实时搜索、显式 API Key 模式的原生图片能力及 CC Switch provider 配置。保留当前 env_key、兼容默认和 gpt-5.6-sol 默认模型，避免改变已有分组/客户端策略。 |
| #8365 | 加入冻结的 Terra 官方描述和启动期 prompt 解析，补齐订阅长上下文豁免及 API Key 的 Lite 修正；复用当前别名、价格、缓存/工具能力和 minimal→none 规范化。 |

[机器台账](../diagnostics/upstream-sync-20261107-followup/UPSTREAM_PRS.json) 固定 merge/head SHA、原始 diff 摘要、
当前 owner、关闭原因和重开条件。tracking merge 只登记审查边界，不代表暂缓能力已经实现。

## 当前实现与性能

原生图片仅接入本轮新增的两个 2.6 模型。旧图片模型继续沿用本项目已验证的 Responses 路径。
新路径使用所选账号、代理、workspace 路由和实际端点的现有鉴权/证明 owner；转发经过账号模型映射，
不在 404/405 后改用可能忽略所选图片模型的 Responses 工具。请求体替换时同步清除原压缩头和长度头。

支持显式 complexity、markup、GIF 帧和 SVG author 参数；缺省不注入 complexity。输出须包含真实图片，
不能把空 JSON、错误帧或无输出流计为成功。流式编辑遇到 JSON 结果时转换为完成事件，usage 只带一次；
后续空 usage 帧保留已经观察到的输入、缓存和输出计数；断连后继续使用当前用量排空和可靠结算，不引入新的总排空 deadline。

图片缓存只采用上游明确报告的 image cache 明细，限制在输入/缓存总量内；文本缓存不被推断成图片缓存。
缓存图片输入使用独立价卡及图片倍率，渠道显式缓存价格同时覆盖图文缓存。费用最终仍交给现有 PostgreSQL
幂等结算和持久队列，未增加丢弃、采样或无界账务队列。历史 usage log 字段、表结构和幂等键格式不变。

多图非流式输出按 data item 集中序列化，保留未知字段，避免逐图复制整份 base64 响应。
Terra 静态描述和 instructions 在进程初始化时解析；请求只借用静态值或获取独立描述副本。
额度摘要查询沿用既有管理额度缓存、singleflight 和账号出口，失败不清除旧的 per-model 结果。
性能夹具和实测边界见 [验证记录](../diagnostics/upstream-sync-20261107-followup/VERIFICATION.md)。

## 升级、混合版本与回退

本轮没有新增 SQL migration、Ent schema、Wire 图、运行依赖或鉴权缓存格式。
额度响应是可选字段，旧前端继续显示原模型额度。配置缺省保持文本账号的 30 分钟、2 小时、第三轮禁用策略；
图片错误仅对失败模型执行缺省 2 分钟冷却。持续时间支持 0（使用缺省）到 604800 秒，非法值在启动校验时拒绝。

应在所有网关节点升级后再对外启用 2.6 模型及新的 INTERNAL 500 自定义策略，旧节点不会实现这些行为。
无白名单分组应在滚动升级前临时设置只含旧模型的显式白名单，待全节点升级后再解除限制，避免新节点的
模型目录提前暴露 2.6。回退前同样先限制到旧模型；空白名单表示不限模型，不能作为撤下新模型的方式。
随后恢复原惩罚参数并等待已有模型冷却到期；二进制回退不会自动清除
持久冷却，也不会撤销账务记录。保留数据库，不执行 Down SQL。

Terra 的 Codex 描述补充只作为缺失字段的回退；显式上游 false/空数组和管理员覆盖仍优先。
本项目现有 API effort 规范化继续使用原 owner，未重新引入上游另一套请求拒绝策略。
Codex 生成配置只影响新导出的文件；原生图片仍需要分组权限、余额和实际上游支持。

## 继承的专项

#8484 共享 ingress/patch/异步图片预算、传输/SSE 预算，#8006/#8369 显式 Gemini cachedContents，以及
#8319/#8476 资源级管理员 Key 迁移继续继承原专项。#8457 只移植错误分类和诊断，未把这些尚未验收的
传输策略夹带进同步。上游中不存在于本项目的 handler 子项已明确登记，不新建平行 owner。

本轮测试使用本机 Docker、一次性数据库和假上游；未部署生产节点或调用真实模型账号。
