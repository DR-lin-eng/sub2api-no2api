package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ChatAsset stores small support-chat images in PostgreSQL so every instance
// sees the same authorized content during rolling upgrades.
type ChatAsset struct {
	ent.Schema
}

func (ChatAsset) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "chat_assets"}}
}

func (ChatAsset) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("scope").Values("message", "library", "sticker"),
		field.Int64("conversation_id").Optional().Nillable(),
		field.Int64("uploaded_by").Optional().Nillable(),
		field.String("name").MaxLen(255),
		field.String("mime_type").MaxLen(64),
		field.Int("size"),
		field.Bytes("data").Sensitive(),
		field.String("collection").MaxLen(100).Default(""),
		field.Bool("catalog_visible").Default(false),
		field.Time("created_at").Immutable().Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (ChatAsset) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("messages", ChatMessage.Type).
			Ref("assets").
			Through("message_assets", ChatMessageAsset.Type),
	}
}

func (ChatAsset) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("scope", "catalog_visible", "created_at", "id"),
		index.Fields("conversation_id", "created_at"),
	}
}
