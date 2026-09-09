package repository

import (
	"fmt"
	"time"

	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	dbpredicate "github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/codexsimulation"

	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
)

const codexPrewarmContinuationEnabledAccountSQLPerAccount = `(a.platform = 'openai'
		AND a.type = 'oauth'
		AND COALESCE(a.extra, '{}'::jsonb) @> '{"codex_prewarm_continuation_enabled":true}'::jsonb)`

const codexPrewarmContinuation429TempReasonSQL = `COALESCE(a.temp_unschedulable_reason, '') ~ '"status_code"[[:space:]]*:[[:space:]]*429[[:space:]]*([,}])'`

func codexPrewarmContinuationEnabledAccountSQL() string {
	if codexsimulation.PrewarmContinuationEnabled() {
		return `(a.platform = 'openai' AND a.type = 'oauth')`
	}
	return codexPrewarmContinuationEnabledAccountSQLPerAccount
}

func accountSchedulableRateLimitSQL() string {
	return `(a.rate_limit_reset_at IS NULL
		OR a.rate_limit_reset_at <= NOW()
		OR ` + codexPrewarmContinuationEnabledAccountSQL() + `)`
}

func accountSchedulableTempUnschedulableSQL() string {
	return `(a.temp_unschedulable_until IS NULL
		OR a.temp_unschedulable_until <= NOW()
		OR (` + codexPrewarmContinuationEnabledAccountSQL() + `
			AND ` + codexPrewarmContinuation429TempReasonSQL + `))`
}

func codexPrewarmContinuationEnabledAccountPredicate() dbpredicate.Account {
	if codexsimulation.PrewarmContinuationEnabled() {
		return dbaccount.And(
			dbaccount.PlatformEQ(service.PlatformOpenAI),
			dbaccount.TypeEQ(service.AccountTypeOAuth),
		)
	}
	return dbaccount.And(
		dbaccount.PlatformEQ(service.PlatformOpenAI),
		dbaccount.TypeEQ(service.AccountTypeOAuth),
		dbpredicate.Account(func(s *entsql.Selector) {
			s.Where(sqljson.ValueEQ(
				dbaccount.FieldExtra,
				true,
				sqljson.Path(service.CodexPrewarmContinuationExtraKey),
			))
		}),
	)
}

func codexPrewarmContinuationDisabledAccountPredicate() dbpredicate.Account {
	if codexsimulation.PrewarmContinuationEnabled() {
		return dbaccount.Or(
			dbaccount.PlatformNEQ(service.PlatformOpenAI),
			dbaccount.TypeNEQ(service.AccountTypeOAuth),
		)
	}
	return dbaccount.Or(
		dbaccount.PlatformNEQ(service.PlatformOpenAI),
		dbaccount.TypeNEQ(service.AccountTypeOAuth),
		dbpredicate.Account(func(s *entsql.Selector) {
			extraColumn := s.C(dbaccount.FieldExtra)
			s.Where(entsql.ExprP(fmt.Sprintf(
				`NOT (COALESCE(%s, '{}'::jsonb) @> '{"%s":true}'::jsonb)`,
				extraColumn,
				service.CodexPrewarmContinuationExtraKey,
			)))
		}),
	)
}

func codexPrewarmContinuation429TempReasonPredicate() dbpredicate.Account {
	return dbpredicate.Account(func(s *entsql.Selector) {
		reasonColumn := s.C(dbaccount.FieldTempUnschedulableReason)
		s.Where(entsql.ExprP(fmt.Sprintf(
			`COALESCE(%s, '') ~ '"status_code"[[:space:]]*:[[:space:]]*429[[:space:]]*([,}])'`,
			reasonColumn,
		)))
	})
}

func schedulableRateLimitPredicate(now time.Time) dbpredicate.Account {
	return dbaccount.Or(
		dbaccount.RateLimitResetAtIsNil(),
		dbaccount.RateLimitResetAtLTE(now),
		codexPrewarmContinuationEnabledAccountPredicate(),
	)
}

func schedulableTempUnschedulablePredicate() dbpredicate.Account {
	return dbaccount.Or(
		tempUnschedulablePredicate(),
		dbaccount.And(
			codexPrewarmContinuationEnabledAccountPredicate(),
			codexPrewarmContinuation429TempReasonPredicate(),
		),
	)
}

func activeHardTempUnschedulablePredicate() dbpredicate.Account {
	return dbaccount.And(
		dbaccount.TempUnschedulableUntilNotNil(),
		dbpredicate.Account(func(s *entsql.Selector) {
			column := s.C(dbaccount.FieldTempUnschedulableUntil)
			s.Where(entsql.GT(column, entsql.Expr("NOW()")))
		}),
		dbaccount.Or(
			codexPrewarmContinuationDisabledAccountPredicate(),
			dbaccount.Not(codexPrewarmContinuation429TempReasonPredicate()),
		),
	)
}
