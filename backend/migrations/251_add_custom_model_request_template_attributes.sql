-- 模板属性：请求模板声明其适用模型在各模态下接受的输入字段、取值范围与默认值。
-- 绑定该模板的模型全部继承这些属性；空对象表示不约束，行为与改造前一致。
ALTER TABLE custom_model_request_templates
    ADD COLUMN IF NOT EXISTS attributes JSONB NOT NULL DEFAULT '{}'::jsonb;
