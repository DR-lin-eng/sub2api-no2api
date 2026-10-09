# 用量记录布局修复

当前 main 将“输出 TPS”的标签和数值插在“总耗时”标签与其耗时数值之间，导致截图中的错位。`cell-latency` 现在用独立 `dt`/`dd` 行保存每项指标，并通过 subgrid 共用两列。首字、总耗时、输出 TPS、速度及管理员的本地/引擎首字均保持各自的标签和值配对。

发布基线为 `9312e56c31bc5005ab7ee58a261bd1f635c3b9c7`，保留该 main 的其他修改。Docker 下 86 项用量回归测试、类型检查、lint 与每阶段 46 个 Chromium 布局场景通过。原生 BASELINE 与 ROLLBACK 均复现“总耗时 → 输出 TPS”，MODIFIED 显示“总耗时 → 52.07s”和“输出 TPS → 36.5 tok/s”；发布回退哈希与 `PUBLICATION_BASELINE.vue` 一致。

初次工作区比远端 main 旧，原始验证记录保留在 `baseline/`、`modified/`、`rollback/`；最新 main 的原生验证位于 `publication/`。完整原始字节和初次候选仍保存在 `ORIGINAL_FILE.vue` 与 `INITIAL_MODIFIED_FILE.vue`。

四个角色保持原来的文件名：`MODIFIED_FILE.vue`、`DIFF_FILE.patch`、`VERIFICATION.txt`、`ROLLBACK.sh`。`DIFF_FILE.patch` 从最初原始文件重建当前候选；`PUBLICATION_DIFF.patch` 只包含针对最新 main 的布局改动。`ROLLBACK.sh TARGET_COPY` 恢复初次原始字节；`ROLLBACK.sh --publication TARGET_COPY` 恢复发布基线字节。
