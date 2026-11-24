# 2026-11-24 上游同步验证

本批以项目 `origin/main` 的 `223901a83` 为基线，审查上游 `328db26f4..0f6671457` 的三个主线 PR 和一个版本提交。机器台账记录每个首父补丁的固定 SHA、净 diff SHA-256、当前 owner、关闭理由和重开条件。

#9008 的上游分组显示同步保留本地计费探测和账号名称契约。#9045 的本地 owner 已补齐 Responses `response.error` 在非流式 400 错误中的结构化字段保留，并复用已有透传错误净化。#9052 的身份上报变量暂缓，等待独立的变量授权、错误日志和 WebSocket 连接容量设计。上游重复携带的 gorilla/websocket 安全升级只写入一次 `backend/go.mod` 和 `backend/go.sum`。

验证台账：

```sh
python3 diagnostics/upstream-sync-20261124/verify_ledger.py
```

Docker 验证使用 Go 1.26.11。定向 service 回归、全部 application service、HTTP handler 和 server/routes 测试均在隔离容器中运行。真实供应商、生产数据和生产吞吐不在本批范围。

候选镜像 `sub2api:upstream-sync-20261124` 已按仓库根目录 Dockerfile 构建成功，并通过以下运行时 smoke check：

```sh
docker run --rm --entrypoint /app/sub2api sub2api:upstream-sync-20261124 --version
```

输出版本为项目现有 `0.1.250`，提交标记为本同步分支，未采用上游 `0.2.25`。
