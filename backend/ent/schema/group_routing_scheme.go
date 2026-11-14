package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/domain"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// GroupRoutingScheme is an admin-defined, reusable scheduling scheme for one
// platform. Keys opt into a scheme with api_keys.routing_scheme_id.
type GroupRoutingScheme struct {
	ent.Schema
}

func (GroupRoutingScheme) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "group_routing_schemes"},
	}
}

func (GroupRoutingScheme) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (GroupRoutingScheme) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(100).
			NotEmpty(),
		field.String("description").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("platform").
			MaxLen(50).
			Default(domain.PlatformAnthropic),
		field.String("status").
			MaxLen(20).
			Default(domain.RoutingSchemeStatusActive),
		field.Int("sort_order").
			Default(0),
		field.String("sort_mode").
			MaxLen(20).
			Default(domain.RoutingSortScore).
			Comment("Per-request candidate order: score / ttft / error_count"),
	}
}

func (GroupRoutingScheme) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("items", GroupRoutingSchemeItem.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (GroupRoutingScheme) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("platform", "status"),
		index.Fields("deleted_at"),
	}
}
