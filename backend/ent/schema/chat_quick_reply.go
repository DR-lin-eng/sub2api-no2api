package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
)

// ChatQuickReply stores a reusable reply owned by one administrator.
type ChatQuickReply struct {
	ent.Schema
}

func (ChatQuickReply) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "chat_quick_replies"}}
}

func (ChatQuickReply) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}}
}

func (ChatQuickReply) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("admin_id"),
		field.String("title").NotEmpty().MaxLen(100),
		field.String("content").NotEmpty().MaxLen(10000),
		field.Int("sort_order").Default(0),
	}
}

func (ChatQuickReply) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("admin_id", "sort_order", "id"),
	}
}
