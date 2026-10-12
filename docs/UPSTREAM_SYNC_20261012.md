# 上游主线同步审查（2026-10-12）

本轮从下游 `73469a9062b3e7f8df31d22b42c1507a843b2ae9` 出发，冻结上游
`ab80abc6d1f929a6864c8804fd077b509d21fda2`。审查在独立可写副本完成，保留当前
`transport -> application -> domain -> infrastructure` owner；上游旧目录不会覆盖本项目的模块边界。

## PR 台账

| 上游 PR | 决定 | 当前 owner / 说明 |
| --- | --- | --- |
| #7982 | 已覆盖 | 当前嵌入式前端测试已经使用发布资源路径；没有重复引入旧 `internal/web` 测试树。 |
| #7983 | 已移植 | `admin-accounts` 的 `AccountGroupsCell` 与共享 `GroupBadge` 支持完整名称换行，默认截断行为保留。 |
| #7974 | 已移植 | Gemini OAuth composable 通过 `extractApiErrorMessage` 保留服务端错误，兼容旧错误回退。 |
| #7973 | 已移植 | `channels-user/ChannelStatusPage` 的自动刷新也刷新非 7 天详情缓存；7 天路径仍只请求列表。 |
| #7972 | 已移植 | 分组用户倍率只接受有限且大于零的数字；清空输入仍表示删除覆盖值。 |
| #7970 | 已移植 | 共享 Select 在 IME composition 期间让输入法处理按键。 |
| #7969 | 已移植 | 运维自定义时间范围使用本地时间控件，提交时只做一次 UTC 转换。 |
| #7933 | 已移植并重构 | 运维请求明细在已有 success/error CTE 中投影 `account_name`，避免前端分页拉取全量账号或逐行请求；移动端和桌面端均显示回退的 `#account_id`。 |
| #7738 | 暂缓 | 当前 `admin-channel-monitor` 没有上游 v2 平台配置 owner；启用前需要明确配置快照、模型目录和滚动升级契约。 |
| #7717 | 已移植 | 支付倒计时以 deadline 计算，后台切换或定时器漂移不会延长订单有效期。 |
| #7696 | 已移植 | 账号用量倒计时在页面重新可见时刷新；不增加后台轮询。 |
| #7711 | 已移植并重构 | 审核分组写入按 group 行再 settings 行加锁，删除在同一事务清理 `group_ids`；旧的非法 JSON 设置保持可删除。 |
| #7649 | 已移植 | Codex hosted image bridge 仅对 OpenAI 平台账号开启，Kimi/DeepSeek/MiniMax 原生 Responses 保持原请求。 |
| #8002 | 已移植 | RPM 覆盖列表使用请求版本号丢弃关闭弹窗或切换分组后的旧响应。 |
| #8036 | 已移植并重构 | GPT-6.1 Sol 的模型长上下文附加费只对 OAuth/Setup Token 按最终发送模型豁免；API Key 和分组自定义区间价格保持原规则。 |
| #8033 | 暂缓 | 当前 owner 是上游 manifest 透传与账号映射，没有旧树的静态 `configuredCodexModelDescriptor`；新增 500k 默认值需先建立目录快照、显式元数据优先级和回退矩阵。 |
| #7960 | 已移植并重构 | 分组名称/描述多语言进入 domain/application/transport owner，迁移使用本项目下一个编号 `248`；管理端保留 `i18n`，用户端按 `Accept-Language` 输出，缺失译文回退默认字段。 |

已移植差异写入 `diagnostics/upstream-sync-20261012/UPSTREAM_PRS.json`，原始 diff SHA 和重开条件保存在同一台账。#7618、#7939 以及此前的搜索历史、工具链和安全差异沿用 2026-10-09 的关闭记录，不重复重做；只有发生新的上游行为或当前 owner 出现回归时才重开。

## 性能与兼容边界

账号名称由请求明细的已有 SQL CTE 一次投影，复用已有关联，只增加账号名称列的投影，不引入 N+1 请求。多语言只在传输输出阶段做有界 map 查找，不参与调度、计费、搜索或分组唯一性。长上下文豁免只传递一个布尔策略到已有计费入口，不重新解析定价。RPM 版本号和支付 deadline 都是 O(1) 的请求状态检查。

相同 Docker Go 1.26.9、linux/arm64、GOMAXPROCS=2，常规计费解析 benchmark 三轮中位数从 `732.5ns` 到 `659.5ns`，均为 `209B / 9 allocs`；未观察到分配增长或大幅时间回退，不据此宣称全网关吞吐提升。订阅豁免判断中位数 `19.05ns / 0B / 0 allocs`。36 个实际 RecordUsage 场景验证 OAuth/Setup Token/API Key、分组与账号开关、priority、缓存 token 和扣款；另验证管理员自定义区间价格不被豁免改变。

分组迁移只增加 带空对象默认值的 JSONB 列，使用 `IF NOT EXISTS`；旧节点忽略新列，新的读路径对缺失译文回退原字段。审核配置写入和组删除使用相同的 group → settings 锁顺序，避免跨实例竞态；非法历史 JSON 不会阻塞组删除。

## Docker 与回退验证

候选与基线均从完整源码树构建。基线生产镜像构建通过；候选前端 builder 和完整生产镜像在最终提交前重建。Docker Go 聚焦测试覆盖多语言规范化、GPT-6.1 Sol 凭据类型边界、非 OpenAI image bridge、审核分组和运维账号投影。

前端候选关键目录测试为 `843/844`，唯一失败是现有 `admin-accounts/presentation/widgets/CreateAccountDialog.vue` 的 1512 行维护门禁；同一测试在干净基线也是 `1512 > 1500`。后端布局检查同样在基线和候选都报告既有 `payment_fulfillment_test.go` 1201 行、`pricing_service.go` 1238 行。本轮没有为无关门禁扩大范围。候选生产构建、全量 Go unit、前端 lint 和审核分组 integration 测试通过。升级/回退使用同一 PostgreSQL/Redis 数据快照，验证旧镜像启动、候选镜像启动、回到旧镜像以及再次升级后的登录、API Key、旧订单和已存在的迁移不会减少，新增迁移只应用一次；详细 stdout/stderr、退出码和哈希见 `diagnostics/upstream-sync-20261012/VERIFICATION.txt`。

本轮不删除现有数据库 CHECK 约束，不改公开协议必填字段，不把上游 tracking merge 当作新平台已实现。重新启用 #7738 或 #8033 前应先补齐对应 owner、快照缓存和混合版本部署矩阵。

浏览器 fixture 在 320、375、1440 宽度与中英文下复现基线的名称截断和 `+2` 被裁切；候选完整换行且按钮可见，无水平溢出。独立回退副本重现基线行为。候选译文编辑保持默认名称，并将既有 `data-tour` 属性透传到真实输入控件。

最终复核把普通删除从共享锁升级方式改成直接 `FOR UPDATE`，真实 PostgreSQL 并发删除与“删除期间写配置”都通过；分组译文保存、旧字段编辑保留译文和空 map 清除测试通过。文档已加入 `.gitignore` 例外，主线索引不再指向仅存在于本地的文件。源码归档以固定初始 SHA 重建，并独立执行补丁应用、回退、哈希比较与重施。

本项目单条订阅进度接口也应用相同的名称译文，不新增查库；进度值从已加载分组带出仅内部使用的 map，JSON 不输出译文表。Docker 混合版本矩阵验证单条进度与分组列表均为“旧版本默认名 → 新版本英文名 → 回退默认名 → 再升级英文名”，旧版本修改默认描述不会清除已有译文。
