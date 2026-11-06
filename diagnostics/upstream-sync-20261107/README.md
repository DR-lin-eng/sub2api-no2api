# 上游同步证据（2026-11-07）

范围和升级边界见 [审查文档](../../docs/UPSTREAM_SYNC_20261107.md)，验收见 [VERIFICATION.md](VERIFICATION.md)。

- [UPSTREAM_PRS.json](UPSTREAM_PRS.json)：冻结 SHA、原始 merge diff 摘要、owner、已关闭子项和明确暂缓能力。
- [SOURCE_HASHES.json](SOURCE_HASHES.json)：本轮源码、测试、构建配置的摘要。
- [PERFORMANCE.json](PERFORMANCE.json)：相同夹具、冻结主线与最终实现的 Docker 逐次结果及中位数。
- [PERFORMANCE_BEFORE_COMPLETION_GATE.json](PERFORMANCE_BEFORE_COMPLETION_GATE.json)：初次候选测量，保留 Read 工具参数继续优化的原因。
- [runtime/result.json](runtime/result.json)：同库 BASELINE → MODIFIED → ROLLBACK → RESTORED 的 30 个真实 HTTP 场景。
- [UI.json](UI.json)：真实组件在桌面/手机宽度、中英文、编辑与禁用确认状态的浏览器证据。截图保存在本地 `output/playwright/upstream-sync-20261107/`。
- [records](records/)：压缩验证输出。

从仓库根目录可运行：

```sh
python3 diagnostics/upstream-sync-20261107/run_integration.py
python3 diagnostics/upstream-sync-20261107/run_performance.py
python3 diagnostics/upstream-sync-20261107/runtime/check.py
```

集成与性能脚本使用 Go 1.26.10 Docker 镜像及预热的 `sub2api-gomod`/`sub2api-gocache` volume。
集成脚本创建一次性 PostgreSQL/Redis 网络，不挂载 Docker socket、不连接生产数据库；结束只清理自己创建的资源。
性能脚本从固定 Git SHA 临时提取基线，禁网并顺序测量。runtime 需要本轮基线/候选镜像，使用独立 Compose 项目
和 `127.0.0.1:18448`；随机认证材料仅在进程与临时容器中存在，结束清理临时数据库。

`ui/` 挂到前端测试镜像的 `/app/frontend/ui-fixture`，真实 `frontend/src` 挂到 `/app/frontend/src` 后，
运行 `pnpm exec vite ui-fixture --config ui-fixture/vite.config.ts`。API adapter 只提供夹具数据，没有远端写入。
