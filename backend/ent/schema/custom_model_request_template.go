package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CustomModelRequestTemplate stores reusable upstream request adaptation rules.
// The JSON payload is intentionally extensible so new protocol operations do not
// require a schema migration for every additional field.
type CustomModelRequestTemplate struct {
	ent.Schema
}

func (CustomModelRequestTemplate) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "custom_model_request_templates"},
	}
}

func (CustomModelRequestTemplate) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (CustomModelRequestTemplate) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(100).
			NotEmpty().
			Comment("模板名称"),
		field.String("description").
			MaxLen(500).
			Optional().
			Default("").
			Comment("模板说明"),
		field.JSON("request_adapter", map[string]any{}).
			Comment("请求适配规则"),
	}
}

func (CustomModelRequestTemplate) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").Unique(),
	}
}
