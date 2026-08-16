package schema

import (
	"encoding/json"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ChatMessage holds the schema definition for the ChatMessage entity.
//
// Messages may contain validated text, image/sticker references, replies, or
// an administrator-created balance-transfer receipt.
type ChatMessage struct {
	ent.Schema
}

func (ChatMessage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "chat_messages"},
	}
}

func (ChatMessage) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("conversation_id"),
		field.Enum("sender_type").
			Values("user", "admin").
			Comment("发送者角色：user=会话所属用户，admin=任意管理员客服"),
		field.Int64("sender_id").
			Comment("发送者用户ID（管理员发送时为该管理员的用户ID）"),
		field.String("content").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			NotEmpty().
			MaxLen(10000),
		field.Enum("kind").
			Values("text", "image", "sticker", "balance_transfer").
			Default("text"),
		field.Int64("reply_to_id").Optional().Nillable(),
		field.JSON("metadata", json.RawMessage{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("idempotency_key").Optional().Nillable().MaxLen(128),
		field.Time("recalled_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("管理员撤回消息的时间；原始内容仅保留用于服务端审计"),
		field.Int64("recalled_by").
			Optional().
			Nillable().
			Comment("执行撤回的管理员用户 ID"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (ChatMessage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("conversation", ChatConversation.Type).
			Ref("messages").
			Field("conversation_id").
			Unique().
			Required(),
		edge.To("assets", ChatAsset.Type).
			Through("message_assets", ChatMessageAsset.Type),
	}
}

func (ChatMessage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("conversation_id", "created_at", "id"),
		index.Fields("reply_to_id"),
		index.Fields("sender_type", "sender_id", "idempotency_key").Unique(),
	}
}
