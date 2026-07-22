package repository

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	dbaccountgroup "github.com/Wei-Shaw/sub2api/ent/accountgroup"
	dbpredicate "github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
	"github.com/lib/pq"

	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
)

func (r *accountRepository) List(ctx context.Context, params pagination.PaginationParams) ([]service.Account, *pagination.PaginationResult, error) {
	return r.ListWithFilters(ctx, params, "", "", "", "", 0, "")
}

func (r *accountRepository) accountListFilteredQuery(platform, accountType, status, search string, groupID int64, privacyMode string) *dbent.AccountQuery {
	q := r.client.Account.Query()

	if platform != "" {
		q = q.Where(dbaccount.PlatformEQ(platform))
	}
	if accountType != "" {
		q = q.Where(dbaccount.TypeEQ(accountType))
	}
	if status != "" {
		switch status {
		case service.StatusActive:
			q = q.Where(
				dbaccount.StatusEQ(status),
				dbaccount.SchedulableEQ(true),
				dbaccount.Or(
					dbaccount.RateLimitResetAtIsNil(),
					dbaccount.RateLimitResetAtLTE(time.Now()),
				),
				dbpredicate.Account(func(s *entsql.Selector) {
					col := s.C("temp_unschedulable_until")
					s.Where(entsql.Or(
						entsql.IsNull(col),
						entsql.LTE(col, entsql.Expr("NOW()")),
					))
				}),
			)
		case "rate_limited":
			q = q.Where(
				dbaccount.StatusEQ(service.StatusActive),
				dbaccount.RateLimitResetAtGT(time.Now()),
				dbpredicate.Account(func(s *entsql.Selector) {
					col := s.C("temp_unschedulable_until")
					s.Where(entsql.Or(
						entsql.IsNull(col),
						entsql.LTE(col, entsql.Expr("NOW()")),
					))
				}),
			)
		case "temp_unschedulable":
			q = q.Where(
				dbaccount.StatusEQ(service.StatusActive),
				dbpredicate.Account(func(s *entsql.Selector) {
					col := s.C("temp_unschedulable_until")
					s.Where(entsql.And(
						entsql.Not(entsql.IsNull(col)),
						entsql.GT(col, entsql.Expr("NOW()")),
					))
				}),
			)
		case "unschedulable":
			q = q.Where(
				dbaccount.StatusEQ(service.StatusActive),
				dbaccount.SchedulableEQ(false),
				dbaccount.Or(
					dbaccount.RateLimitResetAtIsNil(),
					dbaccount.RateLimitResetAtLTE(time.Now()),
				),
				dbpredicate.Account(func(s *entsql.Selector) {
					col := s.C("temp_unschedulable_until")
					s.Where(entsql.Or(
						entsql.IsNull(col),
						entsql.LTE(col, entsql.Expr("NOW()")),
					))
				}),
			)
		default:
			q = q.Where(dbaccount.StatusEQ(status))
		}
	}
	if search != "" {
		q = q.Where(dbaccount.NameContainsFold(search))
	}
	if groupID == service.AccountListGroupUngrouped {
		q = q.Where(dbaccount.Not(dbaccount.HasAccountGroups()))
	} else if groupID > 0 {
		q = q.Where(dbaccount.HasAccountGroupsWith(dbaccountgroup.GroupIDEQ(groupID)))
	}
	if privacyMode != "" {
		q = q.Where(dbpredicate.Account(func(s *entsql.Selector) {
			path := sqljson.Path("privacy_mode")
			switch privacyMode {
			case service.AccountPrivacyModeUnsetFilter:
				s.Where(entsql.Or(
					entsql.Not(sqljson.HasKey(dbaccount.FieldExtra, path)),
					sqljson.ValueEQ(dbaccount.FieldExtra, "", path),
				))
			default:
				s.Where(sqljson.ValueEQ(dbaccount.FieldExtra, privacyMode, path))
			}
		}))
	}

	return q
}

func (r *accountRepository) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]service.Account, *pagination.PaginationResult, error) {
	q := r.accountListFilteredQuery(platform, accountType, status, search, groupID, privacyMode)
	// Clone before Count so interceptor-appended predicates (SoftDeleteMixin's
	// deleted_at IS NULL) don't accumulate on the shared builder and pollute the
	// subsequent list query. Same pattern used in group_repo/promo_code_repo/user_repo
	// (P1-03 audit fix, commit 2588fa6a).
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	accountsQuery := q.
		Offset(params.Offset()).
		Limit(params.Limit())
	for _, order := range accountListOrder(params) {
		accountsQuery = accountsQuery.Order(order)
	}

	accounts, err := accountsQuery.All(ctx)
	if err != nil {
		return nil, nil, err
	}

	outAccounts, err := r.accountsToService(ctx, accounts)
	if err != nil {
		return nil, nil, err
	}
	return outAccounts, paginationResultFromTotal(int64(total), params), nil
}

func (r *accountRepository) ListAllWithFilters(ctx context.Context, platform, accountType, status, search string, groupID int64, privacyMode string) ([]service.Account, error) {
	accounts, err := r.accountListFilteredQuery(platform, accountType, status, search, groupID, privacyMode).All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) ListOpsAccountsForStats(ctx context.Context, platformFilter string, groupIDFilter *int64) ([]service.Account, error) {
	if r == nil || r.client == nil {
		return []service.Account{}, nil
	}

	q := r.client.Account.Query()
	if platformFilter = strings.TrimSpace(platformFilter); platformFilter != "" {
		q = q.Where(dbaccount.PlatformEQ(platformFilter))
	}
	if groupIDFilter != nil && *groupIDFilter > 0 {
		q = q.Where(dbaccount.HasAccountGroupsWith(dbaccountgroup.GroupIDEQ(*groupIDFilter)))
	}

	accounts, err := q.
		Select(
			dbaccount.FieldID,
			dbaccount.FieldName,
			dbaccount.FieldPlatform,
			dbaccount.FieldConcurrency,
			dbaccount.FieldLoadFactor,
			dbaccount.FieldStatus,
			dbaccount.FieldErrorMessage,
			dbaccount.FieldSchedulable,
			dbaccount.FieldRateLimitResetAt,
			dbaccount.FieldOverloadUntil,
			dbaccount.FieldTempUnschedulableUntil,
		).
		Order(dbent.Asc(dbaccount.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func accountListOrder(params pagination.PaginationParams) []func(*entsql.Selector) {
	sortBy := strings.ToLower(strings.TrimSpace(params.SortBy))
	sortOrder := params.NormalizedSortOrder(pagination.SortOrderAsc)
	if sortBy == "upstream_billing_rate" {
		direction := "ASC"
		tieOrder := entsql.Asc
		if sortOrder == pagination.SortOrderDesc {
			direction = "DESC"
			tieOrder = entsql.Desc
		}
		return []func(*entsql.Selector){func(s *entsql.Selector) {
			extra := s.C(dbaccount.FieldExtra)
			expression := upstreamBillingRateSortExpression(extra)
			s.OrderExpr(entsql.Expr(expression + " " + direction + " NULLS LAST"))
			s.OrderBy(tieOrder(s.C(dbaccount.FieldID)))
		}}
	}

	field := dbaccount.FieldName
	defaultOrder := true
	lastUsedSort := false
	switch sortBy {
	case "", "name":
		field = dbaccount.FieldName
	case "id":
		field = dbaccount.FieldID
		defaultOrder = false
	case "status":
		field = dbaccount.FieldStatus
		defaultOrder = false
	case "schedulable":
		field = dbaccount.FieldSchedulable
		defaultOrder = false
	case "priority":
		field = dbaccount.FieldPriority
		defaultOrder = false
	case "rate_multiplier":
		field = dbaccount.FieldRateMultiplier
		defaultOrder = false
	case "last_used_at":
		field = dbaccount.FieldLastUsedAt
		defaultOrder = false
		lastUsedSort = true
	case "expires_at":
		field = dbaccount.FieldExpiresAt
		defaultOrder = false
	case "created_at":
		field = dbaccount.FieldCreatedAt
		defaultOrder = false
	}

	// “从未使用”(NULL) 早于任何实际时间：升序置顶，降序沉底。
	if sortOrder == pagination.SortOrderDesc {
		if lastUsedSort {
			return []func(*entsql.Selector){
				entsql.OrderByField(field, entsql.OrderDesc(), entsql.OrderNullsLast()).ToFunc(),
				dbent.Desc(dbaccount.FieldID),
			}
		}
		return []func(*entsql.Selector){dbent.Desc(field), dbent.Desc(dbaccount.FieldID)}
	}
	if defaultOrder {
		return []func(*entsql.Selector){dbent.Asc(dbaccount.FieldName), dbent.Asc(dbaccount.FieldID)}
	}
	if lastUsedSort {
		return []func(*entsql.Selector){
			entsql.OrderByField(field, entsql.OrderNullsFirst()).ToFunc(),
			dbent.Asc(dbaccount.FieldID),
		}
	}
	return []func(*entsql.Selector){dbent.Asc(field), dbent.Asc(dbaccount.FieldID)}
}

func upstreamBillingRateSortExpression(extra string) string {
	status := extra + " #>> '{upstream_billing_probe,status}'"
	dataJSON := func(key string) string {
		return extra + " #> '{upstream_billing_probe,data," + key + "}'"
	}
	dataText := func(key string) string {
		return extra + " #>> '{upstream_billing_probe,data," + key + "}'"
	}

	effectiveJSON := dataJSON("effective_rate_multiplier")
	effective := dataText("effective_rate_multiplier")
	resolvedJSON := dataJSON("resolved_rate_multiplier")
	resolved := dataText("resolved_rate_multiplier")
	peakEnabledJSON := dataJSON("peak_rate_enabled")
	peakEnabled := dataText("peak_rate_enabled")
	peakMultiplierJSON := dataJSON("peak_rate_multiplier")
	peakMultiplier := dataText("peak_rate_multiplier")
	billingScope := dataText("billing_scope")
	timezoneJSON := dataJSON("timezone")
	timezoneName := dataText("timezone")
	startMinuteJSON := dataJSON(service.UpstreamBillingProbePeakStartMinuteKey)
	startMinute := dataText(service.UpstreamBillingProbePeakStartMinuteKey)
	endMinuteJSON := dataJSON(service.UpstreamBillingProbePeakEndMinuteKey)
	endMinute := dataText(service.UpstreamBillingProbePeakEndMinuteKey)
	sortVersion := dataText(service.UpstreamBillingProbeSortMetadataVersionKey)
	resolvedRate := "(" + resolved + ")::double precision"
	peakRate := "(" + peakMultiplier + ")::double precision"
	localMinute := "(EXTRACT(HOUR FROM CURRENT_TIMESTAMP AT TIME ZONE (" + timezoneName + "))::integer * 60 + " +
		"EXTRACT(MINUTE FROM CURRENT_TIMESTAMP AT TIME ZONE (" + timezoneName + "))::integer)"
	normalizedSnapshot := sortVersion + " = '" + strconv.Itoa(service.UpstreamBillingProbeSortMetadataVersion) + "' AND " +
		"jsonb_typeof(" + resolvedJSON + ") = 'number' AND jsonb_typeof(" + peakEnabledJSON + ") = 'boolean' AND " + billingScope + " = 'token'"
	validPeakMetadata := "jsonb_typeof(" + startMinuteJSON + ") = 'number' AND jsonb_typeof(" + endMinuteJSON + ") = 'number' AND " +
		"jsonb_typeof(" + peakMultiplierJSON + ") = 'number' AND jsonb_typeof(" + timezoneJSON + ") = 'string'"
	dynamicRate := "CASE " + peakEnabled + " WHEN 'false' THEN " + resolvedRate + " WHEN 'true' THEN CASE WHEN " + validPeakMetadata +
		" THEN " + resolvedRate + " * CASE WHEN " + localMinute + " >= (" + startMinute + ")::integer AND " + localMinute + " < (" + endMinute +
		")::integer THEN " + peakRate + " ELSE 1 END END END"

	// Legacy snapshots use the last observed effective rate until the next probe
	// stamps validated sort metadata. This keeps the transition query parallel
	// and avoids repeatedly validating clocks and timezones for every list row.
	return "CASE WHEN " + status + " IN ('ok', 'failed') THEN CASE WHEN " + normalizedSnapshot + " THEN " + dynamicRate +
		" WHEN jsonb_typeof(" + effectiveJSON + ") = 'number' THEN (" + effective + ")::double precision END END"
}

func (r *accountRepository) ListByGroup(ctx context.Context, groupID int64) ([]service.Account, error) {
	accounts, err := r.queryAccountsByGroup(ctx, groupID, accountGroupQueryOptions{
		status: service.StatusActive,
	})
	if err != nil {
		return nil, err
	}
	return accounts, nil
}

func (r *accountRepository) ListActive(ctx context.Context) ([]service.Account, error) {
	accounts, err := r.client.Account.Query().
		Where(dbaccount.StatusEQ(service.StatusActive)).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) ListOAuthRefreshCandidatePage(ctx context.Context, options service.OAuthRefreshPageOptions) (*service.OAuthRefreshCandidatePage, error) {
	if r.sql == nil {
		return nil, errors.New("account repository SQL executor not configured")
	}
	if len(options.Platforms) == 0 {
		return nil, errors.New("oauth refresh candidate platforms cannot be empty")
	}
	if options.Limit <= 0 || options.Limit > 1000 {
		return nil, errors.New("oauth refresh candidate page limit must be between 1 and 1000")
	}

	// (cond) IS NOT TRUE 把 NULL 和 FALSE 都视为"可被刷新"。直接写
	// NOT (a AND b) 在 PG 三值逻辑下会把 a 或 b 为 NULL 的行（即绝大多数
	// 健康账号：temp_unschedulable_until=NULL）也排除，导致后台 token
	// 刷新工作器漏掉所有正常账号 → access_token 到期后请求开始 401。
	query := `
		SELECT id
		FROM accounts
		WHERE deleted_at IS NULL
			AND platform = ANY($1)
			AND id > $2`
	if options.ActiveOnly {
		query += `
			AND status = 'active'`
	}
	if options.IncludeSetupToken {
		query += `
			AND type IN ('oauth', 'setup-token')`
	} else {
		query += `
			AND type = 'oauth'`
	}
	if options.RequireRefreshToken {
		query += `
			AND credentials ? 'refresh_token'
			AND btrim(credentials->>'refresh_token') <> ''`
	}
	if options.ExcludeRetryCooldown {
		query += `
			AND (
				temp_unschedulable_until > NOW()
				AND temp_unschedulable_reason LIKE 'token refresh retry exhausted:%'
			) IS NOT TRUE`
	}
	query += `
		ORDER BY id ASC
		LIMIT $3`

	rows, err := r.sql.QueryContext(ctx, query, pq.Array(options.Platforms), options.AfterID, options.Limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ids []int64
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
		return &service.OAuthRefreshCandidatePage{Accounts: []service.Account{}}, nil
	}

	accounts, err := r.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	accountsByID := make(map[int64]*service.Account, len(accounts))
	for _, account := range accounts {
		if account != nil {
			accountsByID[account.ID] = account
		}
	}
	out := make([]service.Account, 0, len(accounts))
	for _, id := range ids {
		if account := accountsByID[id]; account != nil {
			out = append(out, *account)
		}
	}
	page := &service.OAuthRefreshCandidatePage{
		Accounts: out,
		HasMore:  len(ids) == options.Limit,
	}
	if len(ids) > 0 {
		page.NextAfterID = ids[len(ids)-1]
	}
	return page, nil
}

func (r *accountRepository) ListByPlatform(ctx context.Context, platform string) ([]service.Account, error) {
	accounts, err := r.client.Account.Query().
		Where(
			dbaccount.PlatformEQ(platform),
			dbaccount.StatusEQ(service.StatusActive),
		).
		Order(dbent.Asc(dbaccount.FieldPriority)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}
