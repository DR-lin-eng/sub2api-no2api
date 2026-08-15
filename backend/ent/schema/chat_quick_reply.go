package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ChatQuickReply holds the schema definition for the ChatQuickReply entity.
type ChatQuickReply struct {
	ent.Schema
}

func (ChatQuickReply) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "chat_quick_replies"},
	}
}

func (ChatQuickReply) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("admin_id").
			Comment("所属管理员ID"),
		field.String("title").
			MaxLen(100).
			NotEmpty().
			Comment("快捷回复标题"),
		field.String("content").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			NotEmpty().
			Comment("快捷回复内容"),
		field.Int("sort_order").
			Default(0).
			Comment("排序序号"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (ChatQuickReply) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("admin_id"),
		index.Fields("admin_id", "sort_order"),
	}
}
