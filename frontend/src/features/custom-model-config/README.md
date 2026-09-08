# Custom Model Config

本 feature 提供 `/admin/custom-model-config` 的自定义能力和请求模板管理。完整配置流程见 [自定义模型与媒体工坊](../../../../docs/CUSTOM_MODELS_AND_MEDIA.md)。

- [data/dtos](data/dtos/)、[data/datasources](data/datasources/)：管理协议、模型与模板 CRUD；`runtime=1` 查询启用后的能力视图。
- [domain/entities](domain/entities/)：能力与模板实体。
- [domain/services/modelCapabilityService.ts](domain/services/modelCapabilityService.ts)：纯能力查询，精确匹配优先，其次最长前缀。
- [modelCapabilityCache.ts](modelCapabilityCache.ts)：按需加载、60 秒缓存和请求去重。
- [presentation](presentation/)：列表、配置对话框与模板编辑/导入。

`custom_model_config_enabled` 控制运行时生效，不删除管理员已保存的配置。全应用启动不预取此管理接口；用户媒体模型列表由媒体工坊接口提供，不能改成普通用户请求管理员配置。能力声明不创建上游账号、授权或价格。

从 `frontend/` 执行 `pnpm exec vitest run src/features/custom-model-config src/features/media-studio`。后端 resolver、模板校验和 handler 另有同主题回归；修改模板需同时覆盖网关应用位置。
