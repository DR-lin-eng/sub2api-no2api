# 验证与发布记录

本轮最终源码输入摘要见 `SOURCE_HASHES.json`，镜像 ID 见 `IMAGES.json`；
`LOCAL_VERIFICATION.json` 汇总实际退出码，原始命令和 gzip 日志保存在 `records/`。

| 门禁 | 实际记录 | 结果 |
| --- | --- | --- |
| Docker 全后端 unit + MCP SDK | `backend-unit-release` | PASS，24926 个测试条目（含子测试） |
| Docker 全后端 lint / 最终改动包 lint | `backend-lint` / `backend-lint-final` | PASS，0 issues |
| Docker 全包 integration + 独立审计数据库 | `integration-final` | PASS |
| 冻结主线 / 最终候选生产镜像 | `build-baseline` / `build-final` | PASS |
| 同库基线→候选→回退→恢复 | `runtime-final` | PASS，56 个 HTTP 场景 |
| 同工具链 parser benchmark | `performance-optimized` | PASS，最终两组均 0 B/op、0 allocs/op |
| 文档 / 源码布局 / 源码摘要 | `docs` / `source-layout` / `source-hashes` | PASS |
| 8 PR + 版本提交关闭台账 | `ledger` / `ledger-ancestry` | PASS，268 个继承 PR 台账字节不变，tracking merge 树内容不变 |

同库各阶段保持 317 条迁移摘要、用户身份、API Key、订阅、明确配置的价格、历史账单摘要及
余额等式。真实生产数据、上游 OAuth 权限和生产并发吞吐不属于夹具验证范围。

夹具复现：缺少聚合的 TTL 用量在旧版通用 Messages 路径中记录为 0；候选的非流式、流式
分别合并为 30、50。回退重新呈现原漏记，恢复后重新得到修复结果；历史记录不被修改。
因此二进制能共库并不代表混合版本对新请求的缺省收费一致，部署顺序见审查文档。

初始编译/夹具失败保留。移植测试按本地函数签名、unit build tag、include_usage、缓冲 EOF、
渠道单一 cache-write 单价和已关闭 #6132 的 delta 零值不覆盖契约调整；未删除原有零值回归断言。
内部 presence 标记的两个结构体期望已更新。初始指针临时分配、假上游 query 路径与基线期望
修正均有独立记录，最终成功不覆盖初始失败。

推送后按 PR 的精确 head SHA 跟踪 CI，合并后另按 main 的精确 merge SHA 跟踪 CI、
安全扫描和 Docker 发布；远端最终状态及运行链接以本次 PR 和任务完成报告为准。
