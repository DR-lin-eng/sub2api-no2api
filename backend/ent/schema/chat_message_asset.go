package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ChatMessageAsset is the edge schema between messages and reusable assets.
type ChatMessageAsset struct {
	ent.Schema
}

func (ChatMessageAsset) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "chat_message_assets"},
		field.ID("message_id", "asset_id"),
	}
}

func (ChatMessageAsset) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("message_id"),
		field.Int64("asset_id"),
		field.Int("sort_order").Default(0),
	}
}

func (ChatMessageAsset) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("message", ChatMessage.Type).Unique().Required().Field("message_id"),
		edge.To("asset", ChatAsset.Type).Unique().Required().Field("asset_id"),
	}
}

func (ChatMessageAsset) Indexes() []ent.Index {
	return []ent.Index{index.Fields("asset_id")}
}
