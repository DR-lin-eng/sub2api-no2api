# 2026-11-20 上游同步验证

冻结范围、能力边界和滚动部署顺序见[审查说明](../../docs/UPSTREAM_SYNC_20261120.md)。`verify_ledger.py` 检查当前首父链、文件列表、固定净补丁、历史台账及当前实现路径。`benchmark.py` 直接读取冻结上游及本项目的实际函数，三次采样见 `performance.txt`。

Docker 前端专项覆盖透传批改、账号用量、设置及模型广场，四个文件共 157 个测试通过。Docker Go 1.26.11 的 DeepSeek 基准以原始源文件进行同输入比较。候选生产镜像通过项目根目录的 Dockerfile 构建。

[四阶段记录](runtime.json)保存 60 个 HTTP 场景的费用和协议结果。四次使用同一临时 PostgreSQL 与 Redis，核对 321 条迁移摘要、用户、密钥、订阅、定价、历史用量及余额等式。TypeSafe 返回的实际模型依次为 `jev-latest`、`mapped-fixture`、`jev-latest`、`mapped-fixture`。固定主线镜像的 OCI revision 与基线提交一致。

从仓库根目录复现同库测试。命令使用独立的 Compose 项目、回环端口 19586 和临时数据卷。

```sh
fixture_dir=$(mktemp -d)
cp -R diagnostics/upstream-sync-20261118/runtime/. "$fixture_dir/"
patch -d "$fixture_dir" -p1 < diagnostics/upstream-sync-20261120/runtime.patch
docker build -t sub2api:sync-20261120-local .
SYNC_TEST_CANDIDATE=sub2api:sync-20261120-local python3 "$fixture_dir/check.py"
```

本机早期全量 unit 在只读源码目录创建 Ent 临时文件失败，改用容器内可写副本后，Go 大包编译曾被虚拟机内存压力终止。CI 结果按本项目 PR #227 与合并后的提交查询。真实供应商凭据和生产吞吐不在这组夹具的覆盖内。
