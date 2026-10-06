# TypeSafe 与充值阶梯专项适配（2026-10-06）

基线为本项目 main `eb107c2b48e1e8c471713cc41817fec3811fd64e`，保留 PR #108 的贡献者修复。本轮只完成前次台账暂缓的三项，冻结上游范围仍为 `b8dece9000c68815a5b867ca5a1e6f236e173905`。不重复处理 2026-10-05 已关闭的差异，不覆盖上游旧目录、版本号或生成文件。

| 上游 PR | 本项目 owner 与结果 |
| --- | --- |
| [#7425](https://github.com/Wei-Shaw/sub2api/pull/7425) / `34fe845f5ab1ae29d541061c175c2a397f2028e3` | `modules/typesafe` 独立协议模块；application/transport 复用账号出口、审计、调度、可靠结算；平台、配额和管理端完整接入 |
| [#7813](https://github.com/Wei-Shaw/sub2api/pull/7813) / `3040209f205472038c1ba745a1bedd2edd9053b1` | 仅 API-key TypeSafe 账号可探测；前后端资格一致；官方 typesafe.ai 域名不探测 fork 专用 `/v1/sub2api/billing`，自定义 relay 使用现有探测与出口机制 |
| [#7802](https://github.com/Wei-Shaw/sub2api/pull/7802) / `5637f469fa85e30072c5635a7072087a8e25d6bf` | `modules/payment/recharge_bonus.go` 负责纯报价；应用层保存订单快照；管理端阶梯和用户端预览、活动文案、订单赠送展示 |

[机器台账](../diagnostics/typesafe-recharge-20261006/UPSTREAM_PRS.json) 保存原始 diff 哈希、关闭标志和验证入口；前次台账中的三项同步关闭。以后只在上游行为变化或出现回归证据时重开。

## 原生 TypeSafe

`POST /v1/systemone` 接受 Jev `noul`、`choice`、`score` 的原生请求，保留成功响应 JSON 与扩展字段，拒绝 `stream:true`。TypeSafe 分组不进入 Messages、Chat Completions、Responses 或 Codex 协议。原生账号只接受 API Key；默认模型 `jev-latest`，默认地址 `https://api.typesafe.ai`，账号自定义 `/v1` 后缀不会重复拼接。

有序多分组 Key 继续遵循本项目同平台绑定限制；原生入口在认证计费前按原顺序保留 TypeSafe/Composite 分组，只修改请求级副本。运行验证使用两个同为 TypeSafe 的分组，第一组无账号时按顺序使用第二组。通用模型目录可展示 Jev，Codex 专用清单继续限定 OpenAI 分组。原生转发与账号 SSE 连接测试均使用账号的完整代理/IPv6 路由；代理优先级、请求头保护、上下文取消和槽位释放沿用本项目规则。400/413/422 不惩罚账号，其他失败遵循现有失败转移及账号错误策略。网关上游错误正文不进入通用日志或客户端错误，防止回显输入。

计费继续走本项目可靠队列和同步兜底，异步闭包在提交前快照请求字段，不读取结束后被复用的 Gin context。Jev 回退价格输入 $0.042/百万 token、输出免费，来自 [TypeSafe 官方发布说明](https://typesafe.ai/blog/introducing-system-one-models-and-jev)；渠道自定义价格和倍率继续由现有系统负责。

## 充值金额与升级

`RECHARGE_BONUS_ENABLED` 默认为 false。即使已有阶梯 JSON，没有明确开启也按旧规则创建订单；旧客户端不传新设置字段时保持当前值。每笔新订单快照 `amount`（到账 USD）、`pay_amount`（实际支付币种与手续费）和 `bonus_amount`（免费到账 USD）。免费额度从推广返利基数中扣除；订阅订单不参加阶梯。

例如倍率为 1、手续费为 0 时，100 元档位赠送 20%：实付 100、到账 120、赠送 20、返利基数 100；退款一半到账额度会退还实付 50，并撤回 60 余额。20% 折扣则实付 80、到账 100、免费部分 20。修改配置不改变已有订单，退款按快照比例计算，跨币种精度沿用支付模块。

迁移追加 `246_add_typesafe_platform.sql` 和 `247_add_payment_order_bonus_amount.sql`，运行在本项目 OpenCode 迁移 244 后面，不使用上游冲突的 241 编号。平台约束只扩大已有集合；订单新列为 `DECIMAL(20,2) NOT NULL DEFAULT 0`，旧行和旧 binary INSERT 无须回填代码。

滚动升级顺序：先升级所有节点并确认迁移完成，再创建原生 TypeSafe 账号/分组或开启充值优惠。回退前关闭充值优惠，等待或处理完所有优惠订单，并停止原生流量路由；旧 binary 不具备优惠返利或 System One 处理能力。加法迁移不需要回退删除列。默认关闭阶段已经验证旧 binary 可以继续读取和写入旧订单。

最多 20 档阶梯，设置通过批量读取加载，不新增网关热路径数据库查询、外部请求或无界队列。活动 Markdown 复用既有支付帮助文案清洗 owner，并由动态 HTML 门禁覆盖。

## 性能与验证

Linux arm64、Docker Go 1.26.6，`GOMAXPROCS=2`，相同有效请求，100 次/样本、3 次测量：根对象单次解析将 256 KiB state 校验中位耗时从 3.078 ms 降至 2.319 ms（约 25%），分配从 1,855,288 降至 1,584,435 bytes/op（约 15%）。1 KiB 样本约 30.9→14.2 μs。该结果是协议校验基准，不是网络端到端吞吐承诺。

完整 Docker 单元/集成、Go lint、前端 CI 门禁、相关 feature 测试、生产镜像构建、PostgreSQL 升级/默认关闭回退/再升级及原生路由结算记录见 [验证记录](../diagnostics/typesafe-recharge-20261006/VERIFICATION.txt)。真实 PostgreSQL 验证了旧 writer、原平台约束、原生配额以及带赠送订单的重复履约/回调不会重复加款；退款和返利快照有专门回归测试。

浏览器在 1280×900 与 390×844 使用实际 Vue 组件验证赠金、折扣切换、金额预览和原生使用说明；fixture 不调用真实支付或 TypeSafe 服务。截图和 harness 在诊断目录。

扩大到整个 admin-settings 测试目录时发现一条原有 Codex 卡片模板哈希断言：基线 main 与当前文件完全相同，但测试期待旧哈希。本轮没有修改无关卡片或断言；[基线证据](../diagnostics/typesafe-recharge-20261006/PREEXISTING_TEST.txt) 单独记录。全部 CI 必需门禁仍完整执行。
