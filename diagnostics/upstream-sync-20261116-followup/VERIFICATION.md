# 验证记录

冻结主线 `b617f7e37d8fac78cecb2251a472d8388b896a22`，上游至 `9cd1a1eb21e84707685e1998dcb5a3d1ceafae49`。
最终本地门禁全部通过，实际命令、退出码和日志摘要集中于 `LOCAL_VERIFICATION.json` 与 `records/`。

| 最终门禁 | 结果 | 记录 |
| --- | --- | --- |
| Docker 全后端 unit / 官方 MCP Go OAuth | PASS，23689 用例/子用例，28 条件跳过，无失败 | `backend-release-unit.json`, `UNIT_RELEASE_SUMMARY.json`; 最终关键字边界见 `backend-keyword-contracts.json` |
| PostgreSQL/Redis 全包 integration / 独立审计库 | PASS | `integration.json` |
| 全前端 / 最终相邻表单与额度用例 | PASS，427 files / 2642 tests；布局后 4 files / 140 tests | `frontend-final.json`, `frontend-layout-tests.json` |
| 后端 lint | PASS，0 issues | `backend-lint-release.json` |
| 前端 lint / 类型 / 正式 Vite 构建 | PASS；最终布局通过 `vue-tsc -b` 和正式构建 | `frontend-final.json`, `build-layout-final.json` |
| 正式根 Dockerfile 基线 / 最终候选 | PASS | `build-baseline.json`, `build-keyword-final.json` |
| 最终镜像同库基线→候选→回退→恢复 | PASS，94 HTTP 场景，每阶段 317 migrations | `runtime-release.json`, `runtime/result.json` |
| 冻结补丁 / 259 项继承决定 | PASS，7 PR + 1 直接提交，原台账字节不变 | `verify_ledger.py --require-ancestry` |
| 浏览器桌面 1280×720 / 窄屏 390×844 | PASS，默认关闭、说明完整显示、手动开启恢复模型限制 | `UI_QA.json`, `ui-desktop.jpg`, `ui-mobile.jpg` |
| 正则步骤性能 / 原始上游重复对照 | PASS，固定输入各三次，含源码 hash | `PERFORMANCE.json`, `UNICODE_GUARD.json` |

`SOURCE_HASHES.json` 核对最终构建的改动运行输入，未变输入来自冻结基线；镜像标识见 `IMAGES.json`。
完整前端测试和布局后相邻补验分开保留，不把旧截图当作最终布局证明。

同库测试保持身份、Key、订阅、历史账单字节摘要和余额等式。新场景还验证：旧节点忽略筛选开关；
新节点拒绝名单外模型且不结算；来源模型名不被映射目标改写；正则修复在 JSON/SSE 两条路径成功且各结算一次；
Grok 扁平 400 不结算；Carbon JSON/SSE 各 1000/100 token，标准费用 $0.003。
临时 PostgreSQL/Redis、假上游和 UI 容器在检查后清理；不使用真实 OAuth、模型、支付账户或生产数据。

## 性能范围

固定 Go 1.26.11 linux/arm64、无网络、1 个 benchmark CPU、1 MiB 提示词，各输入三次并重复上游对照。

| 正则兼容步骤 | 上游中位耗时 | 适配中位耗时 | 上游重复对照 | 上游/适配分配次数 |
| --- | ---: | ---: | ---: | ---: |
| 无工具、无修改 | 0.451 ms | 0.034 ms | 0.448 ms | 0 / 0 |
| 100 个工具 | 5.013 ms | 1.678 ms | 5.030 ms | 112 / 12 |
| 1000 个工具 | 5.657 ms | 2.062 ms | 5.521 ms | 1015 / 15 |
| 混合 Unicode、无工具 | 1.375 ms | 0.632 ms | 1.370 ms | 0 / 0 |

1000 工具的 B/op 为上游约 1.176 MB、本地约 1.184 MB，不声称内存字节数大幅下降。
首次适配结果保留于 `PERFORMANCE_INITIAL.json`：gjson.GetBytes 曾为大 input 字符串复制整段数据；
最终复用项目既有只读 JSON 视图，字段不跨扫描留存。耗时有并行容器负载波动，局部数据不代表生产吞吐。

## 首次失败与修正

- 首轮编译暴露旧 import 路径和上游独立运行面与当前 owner 的接口差异；夹具按当前错误分类、Grok Chat 直转与 repository 接口适配，不重建旧运行面。
- 首轮前端发现批量编辑模块超过维护上限，静态错误码选项已移至现有纯表单策略 owner，原事件与状态所有权保持一致。
- 首次并行 full unit/lint 的 service 编译进程收到 SIGKILL，Docker engine 总内存约 7.65 GiB。并行构建结束后，以 6 GiB 容器单独复验全部通过；不把首次终止称为断言失败或通过。
- 布局补验在人为收紧的 1536 MiB Node 堆下遇到 vue-tsc 内存上限并退出 134；原 3 GiB 堆的全量检查及最终正式构建通过，相邻 140 用例独立通过。
- 浏览器发现新说明在横向开关行中被压至约 131px；创建/编辑表单已改为独立一行。最终标签在窄屏为 332px，桌面为 838px，截图与实际像素记录均保留。

## 发布核验

同步 PR 为 [#217](https://github.com/DR-lin-eng/sub2api-no2api/pull/217)。
PR 和 main 工作流分别按精确 head/merge SHA 核对，状态以 GitHub Actions 的实际记录为准；本地通过不替代远端门禁。


最终补查发现广泛匹配 Unicode 转义会解码没有工具的整段字符串。关键字预检改为只识别
`pattern` 前缀可能使用的 `\u006x` / `\u007x`，回归同时保留转义 schema 的修复能力。
固定约 1 MiB 中文、Latin-1、emoji 混合提示词与冻结初稿对照：约 3.345 ms / 1,441,792 B/op
降为 0.634 ms / 0 B/op（`UNICODE_GUARD.json`）。此前纯中文样本及两轮矩阵保留，样本不混算。
最终关键字谓词的定向 Docker 回归见 `backend-keyword-contracts.json`；全后端 unit / MCP 的最后完整记录为
`backend-release-unit.json`，随后只收窄该预检并增加混合 Unicode 输入，没有改动持久化或其它业务 owner。
