package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/internal/application/service"

	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
)

func (r *accountRepository) FindByExtraField(ctx context.Context, key string, value any) ([]service.Account, error) {
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.DeletedAtIsNil(),
			func(s *entsql.Selector) {
				path := sqljson.Path(key)
				switch v := value.(type) {
				case string:
					preds := []*entsql.Predicate{sqljson.ValueEQ(dbaccount.FieldExtra, v, path)}
					if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
						preds = append(preds, sqljson.ValueEQ(dbaccount.FieldExtra, parsed, path))
					}
					if len(preds) == 1 {
						s.Where(preds[0])
					} else {
						s.Where(entsql.Or(preds...))
					}
				case int:
					s.Where(entsql.Or(
						sqljson.ValueEQ(dbaccount.FieldExtra, v, path),
						sqljson.ValueEQ(dbaccount.FieldExtra, strconv.Itoa(v), path),
					))
				case int64:
					s.Where(entsql.Or(
						sqljson.ValueEQ(dbaccount.FieldExtra, v, path),
						sqljson.ValueEQ(dbaccount.FieldExtra, strconv.FormatInt(v, 10), path),
					))
				case json.Number:
					if parsed, err := v.Int64(); err == nil {
						s.Where(entsql.Or(
							sqljson.ValueEQ(dbaccount.FieldExtra, parsed, path),
							sqljson.ValueEQ(dbaccount.FieldExtra, v.String(), path),
						))
					} else {
						s.Where(sqljson.ValueEQ(dbaccount.FieldExtra, v.String(), path))
					}
				default:
					s.Where(sqljson.ValueEQ(dbaccount.FieldExtra, value, path))
				}
			},
		).
		All(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrAccountNotFound, nil)
	}

	return r.accountsToService(ctx, accounts)
}

// ListDueUpstreamBillingProbeAccounts bounds result hydration and network work
// to limit. New and migrated snapshots use an indexed Unix timestamp.
// Malformed snapshots are drained first and repaired by the next probe.
func (r *accountRepository) ListDueUpstreamBillingProbeAccounts(ctx context.Context, now time.Time, limit int) ([]service.Account, error) {
	if limit <= 0 {
		return []service.Account{}, nil
	}
	if r.sql == nil {
		return nil, errors.New("account repository SQL executor not configured")
	}

	rows, err := r.sql.QueryContext(ctx, `
		WITH legacy_candidates AS MATERIALIZED (
			SELECT
				id,
				extra #>> '{upstream_billing_probe,status}' AS probe_status,
				extra #>> '{upstream_billing_probe,next_probe_at}' AS next_probe_at
			FROM accounts
			WHERE deleted_at IS NULL
				AND status = 'active'
				AND platform = 'openai'
				AND type = 'apikey'
				AND extra @> '{"upstream_billing_probe_enabled": true}'::jsonb
				AND (
					jsonb_typeof(extra #> '{upstream_billing_probe,next_probe_unix}') = 'number'
					AND extra #>> '{upstream_billing_probe,next_probe_unix}' ~ '^[0-9]{1,19}$'
					AND extra #>> '{upstream_billing_probe,status}' IN ('ok', 'unsupported', 'failed')
				) IS NOT TRUE
		), legacy_parsed AS MATERIALIZED (
			SELECT
				id,
				probe_status,
				next_probe_at,
				next_probe_at ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$' AS rfc3339_shape,
				jsonb_path_query_first_tz(
					jsonb_build_object(
						'value',
						replace(regexp_replace(next_probe_at, 'Z$', '+00:00'), 'T', ' ')
					),
					'$.value.datetime()',
					'{}'::jsonb,
					true
				) #>> '{}' AS parsed_next_probe_at
			FROM legacy_candidates
		), legacy AS (
			SELECT id, 0 AS queue_priority, NULL::numeric AS next_probe_unix
			FROM legacy_parsed
			WHERE probe_status NOT IN ('ok', 'unsupported', 'failed')
				OR probe_status IS NULL
				OR next_probe_at IS NULL
				OR NOT rfc3339_shape
				OR parsed_next_probe_at IS NULL
				OR parsed_next_probe_at::timestamptz <= to_timestamp($1)
			ORDER BY id
			LIMIT $2
		), due AS (
			SELECT
				id,
				1 AS queue_priority,
				(extra #>> '{upstream_billing_probe,next_probe_unix}')::numeric AS next_probe_unix
			FROM accounts
			WHERE deleted_at IS NULL
				AND status = 'active'
				AND platform = 'openai'
				AND type = 'apikey'
				AND extra @> '{"upstream_billing_probe_enabled": true}'::jsonb
				AND jsonb_typeof(extra #> '{upstream_billing_probe,next_probe_unix}') = 'number'
				AND extra #>> '{upstream_billing_probe,next_probe_unix}' ~ '^[0-9]{1,19}$'
				AND extra #>> '{upstream_billing_probe,status}' IN ('ok', 'unsupported', 'failed')
				AND (extra #>> '{upstream_billing_probe,next_probe_unix}')::numeric <= $1
			ORDER BY (extra #>> '{upstream_billing_probe,next_probe_unix}')::numeric, id
			LIMIT $2
		)
		SELECT id
		FROM (
			SELECT * FROM legacy
			UNION ALL
			SELECT * FROM due
		) queued
		ORDER BY queue_priority, next_probe_unix NULLS FIRST, id
		LIMIT $2
	`, now.UTC().Unix(), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	ids := make([]int64, 0, limit)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []service.Account{}, nil
	}

	accounts, err := r.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]service.Account, 0, len(accounts))
	for _, account := range accounts {
		if account != nil {
			out = append(out, *account)
		}
	}
	return out, nil
}
