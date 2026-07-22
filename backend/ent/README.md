# Ent Generated Layer

除 `schema/` 和生成配置外，本目录由 Ent 生成，不手工拆分、重命名或格式化生成文件。

修改数据模型后运行：

```sh
make generate
```

数据库上线变更仍需在 `migrations/` 提供显式迁移，不能只依赖生成代码。
