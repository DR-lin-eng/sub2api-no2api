package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AccountEgressBinding struct {
	ent.Schema
}

func (AccountEgressBinding) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "account_egress_bindings"}}
}

func (AccountEgressBinding) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}}
}

func (AccountEgressBinding) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("account_id").Unique(),
		field.Int64("pool_id"),
		field.String("source_ipv6").MaxLen(45).NotEmpty().Unique(),
		field.String("status").MaxLen(20).Default("active"),
		field.Int64("version").Default(1),
		field.Time("rotated_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (AccountEgressBinding) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("account", Account.Type).
			Ref("egress_binding").
			Field("account_id").
			Unique().
			Required().
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.From("pool", IPv6EgressPool.Type).
			Ref("bindings").
			Field("pool_id").
			Unique().
			Required().
			Annotations(entsql.OnDelete(entsql.Restrict)),
	}
}

func (AccountEgressBinding) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("pool_id"),
		index.Fields("status"),
	}
}
