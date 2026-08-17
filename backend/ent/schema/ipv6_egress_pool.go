package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type IPv6EgressPool struct {
	ent.Schema
}

func (IPv6EgressPool) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "ipv6_egress_pools"}}
}

func (IPv6EgressPool) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}}
}

func (IPv6EgressPool) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").MaxLen(100).NotEmpty().Unique(),
		field.String("cidr").MaxLen(64).NotEmpty().Unique(),
		field.String("node_id").MaxLen(128).Optional().Nillable(),
		field.String("status").MaxLen(20).Default("active"),
		field.Bool("is_default").Default(false),
		field.Int64("allocation_version").Default(1),
	}
}

func (IPv6EgressPool) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("bindings", AccountEgressBinding.Type),
	}
}

func (IPv6EgressPool) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("node_id"),
		index.Fields("is_default").Unique().Annotations(entsql.IndexWhere("is_default = true")),
	}
}
