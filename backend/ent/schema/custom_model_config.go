package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CustomModelConfig holds the schema definition for the CustomModelConfig entity.
type CustomModelConfig struct {
	ent.Schema
}

func (CustomModelConfig) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "custom_model_configs"},
	}
}

func (CustomModelConfig) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (CustomModelConfig) Fields() []ent.Field {
	return []ent.Field{
		field.String("model_name").
			MaxLen(255).
			NotEmpty().
			Comment("模型名称"),
		field.Bool("prefix_match").
			Default(false).
			Comment("是否按模型名前缀匹配"),
		field.JSON("capabilities", []string{}).
			Comment("模型能力列表，如 [\"image\", \"video\", \"audio\"]"),
	}
}

func (CustomModelConfig) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("model_name").Unique(),
	}
}
