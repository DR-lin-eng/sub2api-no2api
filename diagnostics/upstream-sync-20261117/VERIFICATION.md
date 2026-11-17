# 验证记录

冻结主线 `73b58110a`，上游至 `15871f904056f548dd874a1d069ffc6cdf777b78`。
实际命令、退出码和日志摘要见 `records/`，生产源码与镜像身份分别记录于 `SOURCE_HASHES.json`、`IMAGES.json`。

| 门禁 | 结果 | 记录 |
| --- | --- | --- |
| Docker 计费/价格优先级、Fast policy、WS 契约回归 | PASS | `backend-focused.json` |
| Docker 全后端 unit / 官方 MCP Go OAuth | PASS，23710 用例/子用例，28 条件跳过 | `backend-unit.json`、`UNIT_SUMMARY.json` |
| 正式 Dockerfile 主线基线与候选构建 | PASS | `build-baseline-verified.json`、`build-candidate.json` |
| PostgreSQL/Redis 同库基线→候选→回退→恢复 | PASS，4 阶段 / 40 HTTP 场景 | `runtime-final.json`、`runtime/result.json` |
| 文档与后端文件布局 | PASS | `docs.json`、`source-layout.json` |
| 冻结补丁、3 份继承台账 | PASS，2 PR + 1 版本提交，266 个既有 PR 原始决定不变 | `verify_ledger.py` |
| Docker 匹配器性能 | PASS，各输入三次，均 0 B/op、0 allocs/op | `PERFORMANCE.json` |

四阶段都保持 317 条迁移及其校验和、持久身份/Key/订阅、历史用量字节摘要和余额等式。
已持久化的显式分组输入价格在全部阶段按同一价格结算；新兜底只在候选中生效，回退恢复旧缺价行为。
固定假上游报告每次 1000 输入 token，Embedding 8B/4B/0.6B 的新费用分别为
$0.00004/$0.00002/$0.00001；相似的未知 ID 保持缺价处理。HTTP JSON/SSE 下 Astra/Sol
每次 1000 输入和 100 输出 token，费用在全部版本分别为 $0.09/$0.018。
每次请求均只有一条用量记录。临时 Docker 容器和卷在验证后清理。

## 性能范围

同一 Docker linux/arm64 / Go 1.26.11，1 个 benchmark CPU，无网络。仅采样已小写模型 ID 的局部匹配器，
不含调度、整条价格查找、数据库或真实模型推理，不能推断生产吞吐。冻结上游源码摘要及抽取方法保留于 runner 和 JSON。

| 输入 | 上游中位 ns/op | 当前中位 ns/op | 两者分配 |
| --- | ---: | ---: | --- |
| Qwen Embedding 8B | 16.63 | 8.724 | 0 |
| Qwen Reranker 0-6B | 71.93 | 6.448 | 0 |
| 无关 GPT 模型 | 8.769 | 2.885 | 0 |

没有发现需要大规模重构的性能问题；精确白名单消除了误匹配并减少连续子串扫描。
采样期间也有其他测试容器运行，结果用于局部成本核对，不作为稳定吞吐承诺。

## 首次失败及修正

- 第一次 Docker 构建调用被文件沙箱拒绝访问 daemon；在允许访问 daemon 后同一基线构建成功，两个记录分别保留。
- 第一轮运行夹具在已被调度缓存的账号上用直接 SQL 改协议标记，假上游按 Responses 返回，而缓存仍按 Chat 解析，用量为零。夹具改用首次调度前完成配置的独立 Responses 账号；生产源码未因此修改。修正后全部四阶段通过。
- Python 官方价卡读取遇到宿主机 CA 查找失败，改用系统 curl 正常校验证书；不关闭 TLS 验证。五张价卡与未核实的 4B 重排页面均有响应摘要。

## 发布核验

推送后按 PR 的精确 head SHA 跟踪 CI；合并后另按 main 的精确 merge SHA 复查。远端状态以 GitHub Actions 实际结果为准。
