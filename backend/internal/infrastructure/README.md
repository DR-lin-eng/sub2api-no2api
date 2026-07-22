# Infrastructure

基础设施层实现 application 定义的持久化、缓存和外部资源端口。

| 目录 | 作用 |
| --- | --- |
| `repository/` | PostgreSQL/Ent、Redis、缓存和上游数据访问实现 |

本层可以依赖 application 的接口和数据结构；application 不得反向依赖本层。具体绑定集中在 Wire 装配代码。
