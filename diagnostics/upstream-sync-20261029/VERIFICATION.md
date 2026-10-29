# 上游同步验证（2026-10-29）

基线：c050e6cbf4d8c8fee4609f15171181b3e521714d。上游冻结：a3298b46ebe239fb0720b068a1eb3383955bdac2。

使用 Docker Go 1.26.10 与按本项目锁文件更新的 Node 24 / pnpm 11.17.0 环境。
精确命令、起止时间、完整压缩输出、退出码和 stdout SHA-256 由 run_check.py 保存到 records。
不同资源/早期候选的结果不得代替最终候选。

早期验证修正了 OAuth 非流式聚合的 x-request-id 保留。旧 service 日志中的业务错误是故障注入，
不能直接当成测试失败依据；后续使用完整日志和 --- FAIL 名称定位。一次编译被系统杀死，改为
限制容器/编译并发后验证。没有跳过失败用例作为通过依据。

针对性、完整单元、集成、lint、前端关键用例/构建、性能、生产镜像、同库升级回退和远端发布
各自按 records 中的最终退出状态记录。文档与源码差异检查通过；后端 layout 门禁报告的两处
1201/1218 行超限与基线一致，未扩充 allowlist。

## Final recorded results

- `backend-unit-final`: exit 0; full `go test -tags=unit ./... -count=1`, including application service 229.119s and HTTP handler 57.619s.
- `frontend-final`: exit 0; lint, typecheck, 39 affected tests and production build completed in 2m25s.
- `production-image`: exit 0; `sub2api:upstream-sync-20261029`, OCI image revision label `5db956c21c5de54b0a4054e0161649967ffc81c4`.
- `runtime-upgrade-rollback`: exit 0; baseline → modified → rollback → restored. Migrations remained 307, persisted identity/API key/subscription snapshot and historical usage stayed unchanged, and balance settlement identity passed. Modified/restored behavior was: admin guard HTTP 400 with `ADMIN_DISABLE_FORBIDDEN`, channel OpenAI Web Search dimension preserved, declared Gemini models returned from one metadata account lookup, and OAuth incomplete Responses returned JSON with one usage row.
- `benchmark-final`: exit 0; existing completion-only extraction median was about 85.8–88.6 µs, unified terminal extraction about 43.3–47.8 µs for the same fixture, both 156,792 B/op and 262 allocs/op.
- `shell-final`: exit 0; documentation, deployment scripts, runtime resource and Caddy checks passed.
- `ui-fixture`: exit 0; disposable Docker admin UI showed the OpenAI Web Search switch at desktop and 390×844 mobile width. Browser evaluation reported no horizontal overflow at either 1488 px desktop or 390 px mobile width.

Remote PR #168 head `5db956c21c5de54b0a4054e0161649967ffc81c4` passed Security Scan, frontend, shell, backend unit/integration, golangci-lint and Docker Image checks. A final evidence commit is pushed after these records; its CI must be checked separately before merge.
