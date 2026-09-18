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
		now := time.Now()
		switch status {
		case service.StatusActive:
			q = q.Where(
				dbaccount.StatusEQ(status),
				dbaccount.SchedulableEQ(true),
				schedulableRateLimitPredicate(now),
				schedulableTempUnschedulablePredicate(),
			)
		case "rate_limited":
			q = q.Where(
				dbaccount.StatusEQ(service.StatusActive),
				dbaccount.RateLimitResetAtGT(now),
				codexPrewarmContinuationDisabledAccountPredicate(),
				schedulableTempUnschedulablePredicate(),
			)
		case "temp_unschedulable":
			q = q.Where(
				dbaccount.StatusEQ(service.StatusActive),
				activeHardTempUnschedulablePredicate(),
			)
		case "unschedulable":
			q = q.Where(
				dbaccount.StatusEQ(service.StatusActive),
				dbaccount.SchedulableEQ(false),
				schedulableRateLimitPredicate(now),
				schedulableTempUnschedulablePredicate(),
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

// oauthQuotaExhaustedPredicate matches persisted OAuth quota observations.
// OpenAI/Codex stores percentages in the 0..100 range, while Anthropic's
// passive windows store utilization as a 0..1 ratio. jsonb_path_exists keeps
// malformed or string-valued custom extra fields from causing a numeric cast
// error and naturally treats missing observations as unknown (not exhausted).
func oauthQuotaExhaustedPredicate() dbpredicate.Account {
	return dbpredicate.Account(func(s *entsql.Selector) {
		extra := s.C(dbaccount.FieldExtra)
		s.Where(entsql.ExprP(oauthQuotaExhaustedExpression(extra, s.C(dbaccount.FieldSessionWindowEnd))))
	})
}

func oauthQuotaExhaustedExpression(extra, sessionWindowEnd string) string {
	branches := []string{
		jsonQuotaWindowPredicate(extra, "codex_5h_used_percent", 100, jsonQuotaWindowActivePredicate(extra, "codex_5h_reset_at", "codex_5h_reset_after_seconds")),
		jsonQuotaWindowPredicate(extra, "codex_7d_used_percent", 100, jsonQuotaWindowActivePredicate(extra, "codex_7d_reset_at", "codex_7d_reset_after_seconds")),
		jsonQuotaWindowPredicate(extra, "codex_primary_used_percent", 100, jsonQuotaWindowActivePredicate(extra, "codex_primary_reset_at", "codex_primary_reset_after_seconds")),
		jsonQuotaWindowPredicate(extra, "codex_secondary_used_percent", 100, jsonQuotaWindowActivePredicate(extra, "codex_secondary_reset_at", "codex_secondary_reset_after_seconds")),
		jsonQuotaWindowPredicate(extra, "session_window_utilization", 1, "("+sessionWindowEnd+" IS NULL OR "+sessionWindowEnd+" > NOW())"),
		jsonQuotaWindowPredicate(extra, "passive_usage_7d_utilization", 1, jsonFutureUnixPredicate(extra, "passive_usage_7d_reset")),
		jsonQuotaWindowPredicate(extra, "passive_usage_7d_oi_utilization", 1, jsonFutureUnixPredicate(extra, "passive_usage_7d_oi_reset")),
		jsonQuotaWindowPredicate(extra, "grok_billing_snapshot.usage_percent", 100, grokBillingWindowActivePredicate(extra)),
		jsonQuotaWindowPredicate(extra, "grok_billing_snapshot.used_percent", 100, grokBillingWindowActivePredicate(extra)),
		// The active App Server snapshot stores epoch reset times below dynamic
		// limit IDs. These branches cover it without treating an expired window
		// as exhausted.
		jsonSnapshotWindowExhaustedPredicate(extra, "primary"),
		jsonSnapshotWindowExhaustedPredicate(extra, "secondary"),
		jsonSnapshotFlatWindowExhaustedPredicate(extra),
		jsonLegacySnapshotWindowExhaustedPredicate(extra, "primary_window"),
		jsonLegacySnapshotWindowExhaustedPredicate(extra, "secondary_window"),
	}
	return "(" + strings.Join(branches, " OR ") + ")"
}

func oauthQuotaKnownExpression(extra string) string {
	paths := []string{
		"codex_5h_used_percent",
		"codex_7d_used_percent",
		"codex_primary_used_percent",
		"codex_secondary_used_percent",
		"session_window_utilization",
		"passive_usage_7d_utilization",
		"passive_usage_7d_oi_utilization",
		"grok_billing_snapshot.usage_percent",
		"grok_billing_snapshot.used_percent",
		"codex_rate_limit_snapshot.rate_limits_by_limit_id.*.primary.used_percent",
		"codex_rate_limit_snapshot.rate_limits_by_limit_id.*.secondary.used_percent",
		"codex_rate_limit_snapshot.rate_limits_by_limit_id.*.used_percent",
		"codex_rate_limit_snapshot.rate_limit.primary_window.used_percent",
		"codex_rate_limit_snapshot.rate_limit.secondary_window.used_percent",
	}
	known := make([]string, 0, len(paths))
	for _, path := range paths {
		known = append(known, jsonNumericPathExists(extra, path, ">=", 0))
	}
	return "(" + strings.Join(known, " OR ") + ")"
}

func oauthQuotaHasQuotaPredicate() dbpredicate.Account {
	return dbpredicate.Account(func(s *entsql.Selector) {
		extra := s.C(dbaccount.FieldExtra)
		known := oauthQuotaKnownExpression(extra)
		exhausted := oauthQuotaExhaustedExpression(extra, s.C(dbaccount.FieldSessionWindowEnd))
		// A row is available only when it has a known supported snapshot and no
		// currently active window is full. Expired windows are therefore usable
		// again, while unknown snapshots stay out of this filter.
		s.Where(entsql.ExprP("(" + known + " AND NOT " + exhausted + ")"))
	})
}

func openAIQuotaWithResetPredicate() dbpredicate.Account {
	return dbpredicate.Account(func(s *entsql.Selector) {
		extra := s.C(dbaccount.FieldExtra)
		s.Where(entsql.ExprP(jsonResetCreditAvailablePredicate(extra)))
	})
}

func openAIQuotaWindowExhaustedPredicate(window string) dbpredicate.Account {
	return dbpredicate.Account(func(s *entsql.Selector) {
		extra := s.C(dbaccount.FieldExtra)
		var branches []string
		switch window {
		case "5h":
			branches = []string{
				jsonQuotaWindowPredicate(extra, "codex_5h_used_percent", 100, jsonQuotaWindowActivePredicate(extra, "codex_5h_reset_at", "codex_5h_reset_after_seconds")),
				jsonLegacyCodexWindowExhaustedPredicate(extra, "primary", 240, 360),
				jsonLegacyCodexWindowExhaustedPredicate(extra, "secondary", 240, 360),
				jsonLegacyCodexWindowExhaustedWithoutDurationPredicate(extra, "secondary"),
				jsonSnapshotWindowExhaustedPredicateForDuration(extra, "primary", 240, 360),
				jsonSnapshotWindowExhaustedPredicateForDuration(extra, "secondary", 240, 360),
				jsonSnapshotFlatWindowExhaustedPredicateForDuration(extra, 240, 360),
				jsonLegacySnapshotWindowExhaustedPredicateForDuration(extra, "primary_window", 10800, 21600),
				jsonLegacySnapshotWindowExhaustedPredicateForDuration(extra, "secondary_window", 10800, 21600),
			}
		case "7d":
			branches = []string{
				jsonQuotaWindowPredicate(extra, "codex_7d_used_percent", 100, jsonQuotaWindowActivePredicate(extra, "codex_7d_reset_at", "codex_7d_reset_after_seconds")),
				jsonLegacyCodexWindowExhaustedPredicate(extra, "primary", 10000, 10160),
				jsonLegacyCodexWindowExhaustedPredicate(extra, "secondary", 10000, 10160),
				jsonLegacyCodexWindowExhaustedWithoutDurationPredicate(extra, "primary"),
				jsonSnapshotWindowExhaustedPredicateForDuration(extra, "primary", 10000, 10160),
				jsonSnapshotWindowExhaustedPredicateForDuration(extra, "secondary", 10000, 10160),
				jsonSnapshotFlatWindowExhaustedPredicateForDuration(extra, 10000, 10160),
				jsonLegacySnapshotWindowExhaustedPredicateForDuration(extra, "primary_window", 604000, 605600),
				jsonLegacySnapshotWindowExhaustedPredicateForDuration(extra, "secondary_window", 604000, 605600),
			}
		}
		if len(branches) > 0 {
			s.Where(entsql.ExprP("(" + strings.Join(branches, " OR ") + ")"))
		}
	})
}

func jsonPathExists(extra, path string) string {
	return "jsonb_path_exists(" + extra + ", '" + path + "')"
}

func jsonPathExistsWithNow(extra, path string) string {
	return "jsonb_path_exists(" + extra + ", '" + path + "', jsonb_build_object('now', EXTRACT(EPOCH FROM NOW())))"
}

func jsonNumericPathExists(extra, path, operator string, threshold float64) string {
	thresholdText := strconv.FormatFloat(threshold, 'f', -1, 64)
	return jsonPathExists(extra, "$."+path+" ? (@ "+operator+" "+thresholdText+")")
}

func jsonQuotaWindowPredicate(extra, key string, threshold float64, activePredicate string) string {
	return "(" + jsonNumericPathExists(extra, key, ">=", threshold) + " AND " + activePredicate + ")"
}

func jsonQuotaWindowActivePredicate(extra, resetAtKey, resetAfterKey string) string {
	resetAtValue := extra + " #> '{" + resetAtKey + "}'"
	resetAtText := extra + " #>> '{" + resetAtKey + "}'"
	if resetAfterKey == "" {
		return "(CASE WHEN jsonb_typeof(" + resetAtValue + ") = 'string' AND " + resetAtText + " ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T' THEN (" + resetAtText + ")::timestamptz > NOW() ELSE TRUE END)"
	}
	resetAfterValue := extra + " #> '{" + resetAfterKey + "}'"
	resetAfterText := extra + " #>> '{" + resetAfterKey + "}'"
	updatedAtValue := extra + " #> '{codex_usage_updated_at}'"
	updatedAtText := extra + " #>> '{codex_usage_updated_at}'"
	return "(CASE " +
		"WHEN jsonb_typeof(" + resetAtValue + ") = 'string' AND " + resetAtText + " ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T' THEN (" + resetAtText + ")::timestamptz > NOW() " +
		"WHEN jsonb_typeof(" + resetAfterValue + ") = 'number' AND jsonb_typeof(" + updatedAtValue + ") = 'string' AND " + updatedAtText + " ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T' THEN (" + updatedAtText + ")::timestamptz + (" + resetAfterText + ")::double precision * INTERVAL '1 second' > NOW() " +
		"ELSE TRUE END)"
}

func jsonResetCreditAvailablePredicate(extra string) string {
	positiveCount := "(" +
		jsonNumericPathExists(extra, "codex_reset_credit_snapshot.available_count", ">", 0) + " OR " +
		jsonNumericPathExists(extra, "codex_reset_credit_snapshot.availableCount", ">", 0) + ")"
	credits := extra + " #> '{codex_reset_credit_snapshot,credits}'"
	expiresAt := "COALESCE(credit #>> '{expires_at}', credit #>> '{expiresAt}')"
	return "(" + positiveCount + " AND EXISTS (SELECT 1 FROM jsonb_array_elements(" +
		"CASE WHEN jsonb_typeof(" + credits + ") = 'array' THEN " + credits + " ELSE '[]'::jsonb END" +
		") AS credit WHERE " + expiresAt + " <> '' AND CASE WHEN " + expiresAt +
		" ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T' THEN (" + expiresAt + ")::timestamptz > NOW() ELSE TRUE END))"
}

func jsonLegacyCodexWindowExhaustedPredicate(extra, window string, minMinutes, maxMinutes int) string {
	return "(" +
		jsonNumericPathExists(extra, "codex_"+window+"_used_percent", ">=", 100) + " AND " +
		jsonNumericPathExists(extra, "codex_"+window+"_window_minutes", ">=", float64(minMinutes)) + " AND " +
		jsonNumericPathExists(extra, "codex_"+window+"_window_minutes", "<=", float64(maxMinutes)) + " AND " +
		jsonQuotaWindowActivePredicate(extra, "codex_"+window+"_reset_at", "codex_"+window+"_reset_after_seconds") + ")"
}

func jsonLegacyCodexWindowExhaustedWithoutDurationPredicate(extra, window string) string {
	return "(" +
		jsonNumericPathExists(extra, "codex_"+window+"_used_percent", ">=", 100) + " AND NOT " +
		jsonPathExists(extra, "$.codex_"+window+"_window_minutes") + " AND " +
		jsonQuotaWindowActivePredicate(extra, "codex_"+window+"_reset_at", "codex_"+window+"_reset_after_seconds") + ")"
}

func jsonSnapshotWindowExhaustedPredicate(extra, window string) string {
	return jsonPathExistsWithNow(extra, "$.codex_rate_limit_snapshot.rate_limits_by_limit_id.*."+window+" ? (@.used_percent >= 100 && (!exists(@.resets_at) || @.resets_at > $now))")
}

func jsonSnapshotFlatWindowExhaustedPredicate(extra string) string {
	return jsonPathExistsWithNow(extra, "$.codex_rate_limit_snapshot.rate_limits_by_limit_id.* ? (@.used_percent >= 100 && (!exists(@.resets_at) || @.resets_at > $now))")
}

func jsonSnapshotWindowExhaustedPredicateForDuration(extra, window string, minMinutes, maxMinutes int) string {
	return jsonPathExistsWithNow(extra, "$.codex_rate_limit_snapshot.rate_limits_by_limit_id.*."+window+" ? (@.used_percent >= 100 && @.window_duration_mins >= "+strconv.Itoa(minMinutes)+" && @.window_duration_mins <= "+strconv.Itoa(maxMinutes)+" && (!exists(@.resets_at) || @.resets_at > $now))")
}

func jsonSnapshotFlatWindowExhaustedPredicateForDuration(extra string, minMinutes, maxMinutes int) string {
	return jsonPathExistsWithNow(extra, "$.codex_rate_limit_snapshot.rate_limits_by_limit_id.* ? (@.used_percent >= 100 && @.window_duration_mins >= "+strconv.Itoa(minMinutes)+" && @.window_duration_mins <= "+strconv.Itoa(maxMinutes)+" && (!exists(@.resets_at) || @.resets_at > $now))")
}

func jsonLegacySnapshotWindowExhaustedPredicate(extra, window string) string {
	return jsonPathExistsWithNow(extra, "$.codex_rate_limit_snapshot.rate_limit."+window+" ? (@.used_percent >= 100 && (!exists(@.reset_at) || @.reset_at > $now))")
}

func jsonLegacySnapshotWindowExhaustedPredicateForDuration(extra, window string, minSeconds, maxSeconds int) string {
	return jsonPathExistsWithNow(extra, "$.codex_rate_limit_snapshot.rate_limit."+window+" ? (@.used_percent >= 100 && @.limit_window_seconds >= "+strconv.Itoa(minSeconds)+" && @.limit_window_seconds <= "+strconv.Itoa(maxSeconds)+" && (!exists(@.reset_at) || @.reset_at > $now))")
}

func grokBillingWindowActivePredicate(extra string) string {
	return "(" +
		jsonFutureTimestampPredicatePath(extra, "grok_billing_snapshot,period_end") +
		" AND " +
		jsonFutureTimestampPredicatePath(extra, "grok_billing_snapshot,billing_period_end") +
		")"
}

func jsonFutureTimestampPredicatePath(extra, path string) string {
	jsonValue := extra + " #> '{" + path + "}'"
	textValue := extra + " #>> '{" + path + "}'"
	return "(CASE WHEN jsonb_typeof(" + jsonValue + ") = 'string' AND " + textValue + " ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T' THEN (" + textValue + ")::timestamptz > NOW() ELSE TRUE END)"
}

func jsonFutureUnixPredicate(extra, key string) string {
	jsonValue := extra + " #> '{" + key + "}'"
	textValue := extra + " #>> '{" + key + "}'"
	return "(CASE WHEN jsonb_typeof(" + jsonValue + ") = 'number' AND " + textValue + " ~ '^[0-9]+(\\.[0-9]+)?$' THEN (" + textValue + ")::numeric > EXTRACT(EPOCH FROM NOW()) ELSE TRUE END)"
}

func applyOAuthQuotaFilter(q *dbent.AccountQuery, filter string) (*dbent.AccountQuery, error) {
	normalized, err := service.NormalizeAccountOAuthQuotaFilter(filter)
	if err != nil {
		return nil, err
	}
	filter = normalized
	switch filter {
	case "":
		return q, nil
	case service.AccountOAuthQuotaFilterExhausted:
		// The filter is specifically for OAuth accounts. Combining it with an
		// explicit non-OAuth type in the UI correctly yields an empty result.
		return q.Where(
			dbaccount.TypeEQ(service.AccountTypeOAuth),
			oauthQuotaExhaustedPredicate(),
		), nil
	case service.AccountOAuthQuotaFilterHasQuota:
		return q.Where(
			dbaccount.TypeEQ(service.AccountTypeOAuth),
			oauthQuotaHasQuotaPredicate(),
		), nil
	case service.AccountOAuthQuotaFilterWithReset:
		return q.Where(
			dbaccount.PlatformEQ(service.PlatformOpenAI),
			dbaccount.TypeEQ(service.AccountTypeOAuth),
			openAIQuotaWithResetPredicate(),
		), nil
	case service.AccountOAuthQuotaFilter5hExhausted:
		return q.Where(
			dbaccount.PlatformEQ(service.PlatformOpenAI),
			dbaccount.TypeEQ(service.AccountTypeOAuth),
			openAIQuotaWindowExhaustedPredicate("5h"),
		), nil
	case service.AccountOAuthQuotaFilter7dExhausted:
		return q.Where(
			dbaccount.PlatformEQ(service.PlatformOpenAI),
			dbaccount.TypeEQ(service.AccountTypeOAuth),
			openAIQuotaWindowExhaustedPredicate("7d"),
		), nil
	default:
		return nil, errors.New("invalid OAuth quota filter")
	}
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

func (r *accountRepository) ListWithOAuthQuotaFilter(
	ctx context.Context,
	params pagination.PaginationParams,
	platform, accountType, status, search string,
	groupID int64,
	privacyMode, oauthQuotaFilter string,
) ([]service.Account, *pagination.PaginationResult, error) {
	q, err := applyOAuthQuotaFilter(
		r.accountListFilteredQuery(platform, accountType, status, search, groupID, privacyMode),
		oauthQuotaFilter,
	)
	if err != nil {
		return nil, nil, err
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	accountsQuery := q.Offset(params.Offset()).Limit(params.Limit())
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

// ListUpstreamBillingRateProjections keeps the periodic admin refresh off the
// full account hydration path. Only ID and extra are selected; credentials,
// proxy relations, groups, runtime counters, and usage data are not loaded.
func (r *accountRepository) ListUpstreamBillingRateProjections(
	ctx context.Context,
	params pagination.PaginationParams,
	filters service.UpstreamBillingRateListFilters,
) ([]service.UpstreamBillingRateProjection, int64, error) {
	q := r.accountListFilteredQuery(
		filters.Platform,
		filters.AccountType,
		filters.Status,
		filters.Search,
		filters.GroupID,
		filters.PrivacyMode,
	)
	q, err := applyOAuthQuotaFilter(q, filters.OAuthQuotaFilter)
	if err != nil {
		return nil, 0, err
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	accountsQuery := q.Select(dbaccount.FieldID, dbaccount.FieldExtra).Offset(params.Offset()).Limit(params.Limit())
	for _, order := range accountListOrder(params) {
		accountsQuery = accountsQuery.Order(order)
	}
	accounts, err := accountsQuery.All(ctx)
	if err != nil {
		return nil, 0, err
	}
	items := make([]service.UpstreamBillingRateProjection, 0, len(accounts))
	for _, account := range accounts {
		items = append(items, service.UpstreamBillingRateProjection{AccountID: account.ID, Extra: account.Extra})
	}
	return items, int64(total), nil
}

func (r *accountRepository) ListAllWithFilters(ctx context.Context, platform, accountType, status, search string, groupID int64, privacyMode string) ([]service.Account, error) {
	accounts, err := r.accountListFilteredQuery(platform, accountType, status, search, groupID, privacyMode).All(ctx)
	if err != nil {
		return nil, err
	}
	return r.accountsToService(ctx, accounts)
}

func (r *accountRepository) ListAllWithOAuthQuotaFilter(
	ctx context.Context,
	platform, accountType, status, search string,
	groupID int64,
	privacyMode, oauthQuotaFilter string,
) ([]service.Account, error) {
	q, err := applyOAuthQuotaFilter(
		r.accountListFilteredQuery(platform, accountType, status, search, groupID, privacyMode),
		oauthQuotaFilter,
	)
	if err != nil {
		return nil, err
	}
	accounts, err := q.All(ctx)
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
			AND schedulable = TRUE
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
