# Test Utilities

| 文件 | 作用 |
| --- | --- |
| `fixtures.go` | 通用测试对象构造 |
| `httptest.go` | HTTP 测试辅助 |
| `stubs.go` | application 端口 stub |

生产代码不得导入本目录。优先保留小型显式 fixture，避免形成行为不透明的测试框架。
