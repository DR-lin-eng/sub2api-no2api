# PR #153 渠道监控同步审查

审查范围固定为 [PR #153](https://github.com/DR-lin-eng/sub2api-no2api/pull/153)，基线为
`1aebebaead38cb5659498a720e2951497e339070`，原 PR 为
`b7eecf33d8559f5ba4c0b784f6fec9cb26c11736`。不引入其他上游 PR，也不重放已关闭的差异。
机器台账和原始验证输出位于 [diagnostics](../diagnostics/pr153-channel-monitor-20261022/)。

## 审查结论与修正

- 保留时间线柱数设置、登录页供应商筛选，以及同供应商主动／被动监控合卡。仍由现有 application、transport 和 feature owner 负责。
- 恢复已发布的 `235_media_studio_api_key_identity.sql` 原始字节，排除无关的 `240_media_studio_api_key_legacy_identity.sql`。原 235 已完成历史 Key 身份迁移；新增 240 会再次重命名之后创建的规范名称 Key。最终 SQL 迁移相对基线零差异。
- 补齐管理员 GET 的 `channel_monitor_timeline_points` 映射，避免保存后读回 0；同步设置写入及 API 契约断言，旧设置摘要保持不变。
- 前端统一限制为 1–60 的整数，缺失、无效及非有限缓存值回退到 20，避免小数写入 Go 整数字段失败或无界柱数渲染。
- 刷新后供应商消失时回到全部供应商。筛选仅处理已授权返回的数据，不增加 HTTP 请求。

该设置仅影响列表卡片显示，不改变采样频率、数据保留、7/15/30 天统计窗口、计费、调度或网关转发。

## Docker 与升级验证

Docker Go 1.26.10 的全量 unit 测试通过；设置 GET/PUT、部分保存、公开设置、契约和渠道监控定向测试通过。
前端相邻的 183 项测试、仓库固定关键集的 248 项测试、全量 ESLint、类型检查、生产前端及完整 Docker 镜像构建通过。

同一页面输入的实际观察：

| 状态 | 卡片数 | 行数 | 每行柱数 | 筛选 OpenAI 后 |
| --- | ---: | ---: | --- | --- |
| BASELINE | 3 | 3 | 20/20/20 | 无筛选控件，仍为 3 行 |
| MODIFIED | 2 | 3 | 35/35/35 | 1 张卡片、2 行 |
| ROLLBACK | 3 | 3 | 20/20/20 | 无筛选控件，仍为 3 行 |

同一 PostgreSQL/Redis 数据库从已发布基线镜像升级、保存设置、回滚、再升级，各阶段 `/ready` 均为 200，迁移数始终为 306。
用户身份、余额、API Key 名称与状态、分组及订阅快照的 SHA256 相同；旧二进制保留新设置，再升级后读回 35。
这是隔离测试环境的观测，不代表已对生产实例执行部署。

源码回滚脚本在独立副本上恢复了全部 7,596 个基线文件的哈希；再应用补丁后与 38 个修改文件一致，重新生成的修改归档 SHA256 也一致。
`MODIFIED_FILE.tar.gz` 与 `DIFF_FILE.patch` 覆盖源码、回归测试及 owner README；本审查文档、台账、截图与执行记录作为补充证据。

## 性能与界面

固定 2 CPU／4 GiB Docker，360 个监控项、每项 60 个历史点，预热 4 次后记录 12 次。
时间线 DOM 被替换，保留实际分组、健康排序与卡片渲染，因此数据仅代表该界面路径，不代表网关吞吐或生产延迟。

| 状态 | 中位耗时 | P90 |
| --- | ---: | ---: |
| BASELINE | 152.58 ms | 160.91 ms |
| MODIFIED | 143.63 ms | 171.59 ms |
| ROLLBACK | 144.56 ms | 152.43 ms |

修改版相对原始基线的中位耗时为 -5.87%，P90 为 +6.64%；回滚结果也表明存在测量波动。未观察到需要重设计的大幅性能回退。
供应商分组保持 O(n)，设置仍通过既有批量查询读取；没有新增网关请求数据库查询、后台轮询、重试或无界队列。

使用实际浏览器检查生产前端构建，API 数据为明确的本地 fixture。1440px 桌面和 390px 手机宽度均无横向溢出，三行各显示 35 柱，主动／被动分区和状态文案正确。
截图与测量见 [UI_RESULT.json](../diagnostics/pr153-channel-monitor-20261022/UI_RESULT.json)。

## 继承状态与关闭条件

14 份既有上游关闭台账逐字节保留，摘要见 [INHERITED_CLOSURES.json](../diagnostics/pr153-channel-monitor-20261022/INHERITED_CLOSURES.json)。
本 PR 的采纳、已覆盖与有意差异均登记为关闭；仅在对应行为变化或当前 owner 出现可复现回归时重开。

扩展前端测试发现主分支已有的 `settingsGatewayResilienceModularization.spec.ts` Codex 模板摘要过期：期望 `c78190eb…`，实际 `f72f76ef…`。
原版和修改版表现相同，相关模板与测试相对主分支均未修改；此项单独记录为继承测试债务，不计入本 PR 的通过项，也未为本次审查重新处理。

源码布局检查同样保留两项基线债务：`openai_gateway_forward.go` 和 `payment_fulfillment_test.go` 各 1,201 行，超过 1,200 行上限；两者相对基线均无改动，未扩充 allowlist。

发布阶段必须另行核对修正后的 PR 精确 SHA、合并后的 main CI、Security Scan、Docker Image，以及 GHCR 的 amd64/arm64 revision。结果写入外部发布记录，避免把待运行结果当作已完成验证。
