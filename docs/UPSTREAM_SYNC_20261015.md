# 上游主线同步审查（2026-10-15）

本轮基线为本项目 main `ec1ead14315d303db19cf3116754eb89a853838c`，沿用前轮审查边界 `4d9ac3e482106d2da3a39fe3e546d8d6801f1cc1`，冻结上游至 `698d142468ab15374bc22bd5247759e5b7352006`。初始范围为 9 个新合并 PR；两个赞助更新提交不进入本项目 README 或资源。本项目版本、模块归属、现有依赖和迁移编号保持独立。执行期间上游另合入 #8120，本轮补充审查并冻结至 `1b2243a4f50621508e79aef4b498a2937a6238ee`；原始冻结输入保留，补充记录不覆盖它。

## 逐 PR 决定

| PR | 决定 | 当前实现和边界 |
| --- | --- | --- |
| [#8106](https://github.com/Wei-Shaw/sub2api/pull/8106) | 适配并关闭 | Anthropic 原生构造器拒绝其它供应商的 OAuth/setup-token；count_tokens 在取凭据前返回兼容的 404。API Key 的多协议兼容保留。当前组合路由已经使用显式路由快照和 detector，不引入上游逐请求账号列表查询。 |
| [#8087](https://github.com/Wei-Shaw/sub2api/pull/8087) | 适配并关闭 | 智谱仅返回 CREDIT_LIMIT 时也按 unit 区分 5h/weekly，避免周窗口先重置时标反；TOKENS_LIMIT 优先和未知 unit 的回退保持。 |
| [#8060](https://github.com/Wei-Shaw/sub2api/pull/8060) | 适配并关闭 | Vertex 无错误明细的共享容量 429 不暂停账号，继续受原有有界重试和换号约束；项目配额和其它 service-account 429 无重置时间时使用一分钟兜底。 |
| [#8076](https://github.com/Wei-Shaw/sub2api/pull/8076) | 适配并关闭 | 原生 Gemini、Messages 和 Chat 三条路径对同一个上游响应只执行一次错误策略；固定 Google token 端点返回明确的 SA account-not-found 时停止该凭据并交给原换号流程。其它 token 错误不扩大为永久失效。 |
| [#8078](https://github.com/Wei-Shaw/sub2api/pull/8078) | 适配并关闭 | 补充 Argon/Cyber 目录和硬编码回退价；思考等级共用基础价，显式等级价仍优先，Argon 不新增长上下文加价。 |
| [#8093](https://github.com/Wei-Shaw/sub2api/pull/8093) | 兼容部分关闭，两个子项专项暂缓 | 补齐 Gemini 4 Argon、Antigravity 工具图片、函数签名、非流式 parts 顺序、图片测试提示、付费档位和 Hub UA。发现模型使用生成请求相同的端点，保留当前账号完整代理/IPv6 上下文，不回退或重定向到其它端点。Claude 5.5 注册和 descriptor/Codex 元数据扩展仍暂缓，见下文。 |
| [#8094](https://github.com/Wei-Shaw/sub2api/pull/8094) | 适配并关闭 | 在当前配置和转换 owner 中加入 `gateway.min_output_tokens`，默认 128、最小 16；覆盖本项目已有 Chat/Anthropic 转换和原生 Chat 回退。原生 Responses/WS 不另行引入上游旧入口改写器。 |
| [#8079](https://github.com/Wei-Shaw/sub2api/pull/8079) | 适配并关闭 | 原生和透传 Messages/count_tokens 使用协议专属 Anthropic base URL；保留本项目既有鉴权方式和 max_tokens 规则。 |
| [#8088](https://github.com/Wei-Shaw/sub2api/pull/8088) | 适配并关闭 | Codex 导入按长度编码的 workspace/account 与 user 组合匹配，避免同用户跨 workspace 误合并；access-token-only 指纹、account fallback、批内去重和存量更新继续使用当前索引。 |
| [#8120](https://github.com/Wei-Shaw/sub2api/pull/8120) | 有意保留本地布局并关闭冻结差异 | 这是上游账号筛选的纯布局回退。本项目保留已有可访问的折叠式次级筛选及 OAuth 额度/Excel BPS 控件；不复制会改变现有交互并丢失本地筛选的旧模板。当前 owner 与基线字节一致。 |

[机器台账](../diagnostics/upstream-sync-20261015/UPSTREAM_PRS.json) 记录每个冻结 merge SHA、原始 diff SHA-256、当前 owner、已关闭子项和重开条件。8 个 PR 适配并完整关闭，#8120 的有意布局差异关闭，#8093 的兼容子项关闭，开放子项不因 ancestry tracking merge 变成已实现。此前台账不改写；已处理差异只在上游行为变化或当前 owner 出现回归时重开。

## 仍开放的子项

Claude 5.5 注册沿用 #7957 的前置条件：当前内置目录没有 Opus/Sonnet 5.5 价格卡，签名历史、adaptive/disabled thinking、强制工具和采样矩阵也没有完整闭合。本轮不公开上游新增的六个 Claude 5.5 等级 ID。管理员既有自定义模型和价格配置仍按原契约处理。

上游 descriptor/Codex 元数据扩展依赖当前尚未建立的目录持久化和优先级 owner，与前轮 #8033 暂缓相关。现有上游模型同步返回 ID 列表，不能直接换成带新持久状态的旧目录实现。后续需先定义元数据优先级、有界刷新/查询和混合版本 writer 的升级回退矩阵；本轮已关闭的生成、图片和端点修复无需重做。

## 性能与兼容

没有数据库迁移、Ent/Wire 修改或依赖升级。成功请求不新增数据库/Redis 往返、后台 worker、无界 goroutine 或额外重试。组合路由继续读取不可变缓存；错误策略复用结果只作用于同一响应，下一次尝试重新清空该结果。

上游工具结果图片方案会在已解析文字后再次解码完整结果，包括大段 base64。本项目改为一次遍历同时取得文字和 inline media，二进制数据不进入 functionResponse 的文字字段，不新增远程图片抓取。原生 Gemini 非流式结果按原顺序收集全部 parts；不同签名的文字片段分别保留，避免合并时覆盖签名。定价别名规范化归入独立 owner，保留精确等级价格优先级。

性能对比使用相同 Docker Go 工具链、linux/arm64、CPU/内存限制和输入，分别记录基线、上游双解码参考、候选及回退结果。数字和分配量以 [VERIFICATION.txt](../diagnostics/upstream-sync-20261015/VERIFICATION.txt) 为准，不将微基准解释为全网关吞吐。


同条件三轮中位数如下；时间单位为 ms/op：

| 1 MiB 工具结果 | 基线 | 上游双解码参考 | 本项目候选 | 独立源码回退 |
| --- | ---: | ---: | ---: | ---: |
| 文字 | 16.886 | 20.334 | 16.403 | 16.596 |
| 图片 | 16.009 | 19.882 | 15.420 | 15.670 |

1 MiB 图片结果的分配由双解码参考的 3,157,482 B / 73 次降为候选 2,108,324 B / 59 次，分配字节约少 33%，时间约少 22%。与会丢弃图片的基线相比，候选多 226 B / 3 次分配；1 KiB 图片路径约 `20.426 → 22.531µs`，如实保留新 inline parts 的小对象开销。文字路径分配次数保持 43。原始样本和环境见 [PERFORMANCE.json](../diagnostics/upstream-sync-20261015/PERFORMANCE.json)。

`gateway.min_output_tokens` 缺省保持 128；环境变量为 `GATEWAY_MIN_OUTPUT_TOKENS`。滚动升级先保持默认配置，全部节点更新后再降低或提高该值。旧节点不识别新配置，会继续使用 128；回退前恢复默认值可避免混合节点的转换上限差异。账号字段、余额、订阅、API Key 和订单不需要数据转换。

## 验证和回退

执行记录保存 Docker 单元/集成、并发 race、lint、前端必需检查、相关账号用例、生产镜像和同 PostgreSQL/Redis 的 baseline → upgraded → rollback → restored 四阶段结果。具体通过项、原始失败、命令、输入、stdout/stderr、退出码和哈希均在 [VERIFICATION.txt](../diagnostics/upstream-sync-20261015/VERIFICATION.txt)，已有门禁债务单列在 [KNOWN_BASELINE_FAILURES.json](../diagnostics/upstream-sync-20261015/KNOWN_BASELINE_FAILURES.json)。前端创建表单 1512 行的既有维护用例在精确基线同样失败；本轮不扩大 allowlist 或修改该表单。后端未触及的支付测试 1201 行也保留为原有门禁结果。

四个可重建角色为 [MODIFIED_FILE.tar.gz](../diagnostics/upstream-sync-20261015/MODIFIED_FILE.tar.gz)、[DIFF_FILE.patch](../diagnostics/upstream-sync-20261015/DIFF_FILE.patch)、[VERIFICATION.txt](../diagnostics/upstream-sync-20261015/VERIFICATION.txt) 和 [ROLLBACK.sh](../diagnostics/upstream-sync-20261015/ROLLBACK.sh)。源码回退脚本接受独立副本，从 sibling baseline 归档恢复原字节并删除本轮新增路径；验证同时核对全部原始源码哈希和补丁重施。原始 7d88 工作区不参与修改或回退。
