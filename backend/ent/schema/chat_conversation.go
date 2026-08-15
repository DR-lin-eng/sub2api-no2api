package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
)

// ChatConversation holds the schema definition for the ChatConversation entity.
//
// 每个用户与客服（全体管理员）之间只有一条长期会话，不按工单/会话轮次拆分。
type ChatConversation struct {
	ent.Schema
}

func (ChatConversation) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "chat_conversations"},
	}
}

func (ChatConversation) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (ChatConversation) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id").
			Unique().
			Comment("会话所属用户，一个用户仅有一条会话"),
		field.Time("last_message_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("最近一条消息时间，用于会话列表排序"),
		field.Int("unread_by_user").
			Default(0).
			Comment("用户未读消息数（客服发出待用户查看）"),
		field.Int("unread_by_admin").
			Default(0).
			Comment("客服未读消息数（用户发出待客服查看）"),
		field.Time("last_read_by_user_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("last_read_by_admin_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (ChatConversation) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("chat_conversation").
			Field("user_id").
			Unique().
			Required(),
		edge.To("messages", ChatMessage.Type),
	}
}

func (ChatConversation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("last_message_at"),
		index.Fields("unread_by_admin").
			StorageKey("idx_chat_conversations_unread_by_admin_active").
			Annotations(entsql.IndexWhere("unread_by_admin > 0")),
	}
}
