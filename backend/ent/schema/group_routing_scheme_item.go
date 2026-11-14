package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/domain"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// GroupRoutingSchemeItem binds one group to a routing scheme. Candidates are
// distributed by the backend-computed composite score, so no manual weight is
// stored here.
type GroupRoutingSchemeItem struct {
	ent.Schema
}

func (GroupRoutingSchemeItem) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "group_routing_scheme_items"},
	}
}

func (GroupRoutingSchemeItem) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (GroupRoutingSchemeItem) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("scheme_id"),
		field.Int64("group_id"),
		field.Int("position").
			Default(0),
		field.JSON("rules", []domain.RoutingSchemeRule{}).
			Default([]domain.RoutingSchemeRule{}).
			Comment("Declarative failover rules for this group inside the scheme"),
	}
}

func (GroupRoutingSchemeItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("scheme", GroupRoutingScheme.Type).
			Ref("items").
			Field("scheme_id").
			Unique().
			Required(),
	}
}

func (GroupRoutingSchemeItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("scheme_id", "group_id").Unique(),
		index.Fields("scheme_id", "position"),
		index.Fields("group_id"),
	}
}
