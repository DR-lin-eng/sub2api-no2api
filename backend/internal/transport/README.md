# Transport

传输层负责把外部协议转换为 application 调用，并把结果映射为稳定的客户端响应。

| 目录 | 作用 |
| --- | --- |
| `http/` | HTTP/WS handler、路由、鉴权和请求中间件 |
| `webassets/` | 嵌入后端二进制的前端构建产物与缓存 |

传输层不得直接导入 `infrastructure/repository`。
