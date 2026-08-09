package repository

import (
	"context"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/timezone"
	"github.com/Wei-Shaw/sub2api/internal/shared/usagestats"
)

type accountUsageAggregateRow struct {
	Date                string
	Model               string
	InboundEndpoint     string
	UpstreamEndpoint    string
	Requests            int64
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
	StandardCost        float64
	AccountCost         float64
	UserCost            float64
	DurationSumMs       int64
	DurationSampleCount int64
	TTFTSumMs           int64
	TTFTSampleCount     int64
}

// GetAccountUsageStats combines durable Ops aggregates with raw rows that were
// written after the latest aggregate. Deleting raw request details therefore
// does not change the account/channel statistics shown to administrators.
func (r *usageLogRepository) GetAccountUsageStats(ctx context.Context, accountID int64, startTime, endTime time.Time) (*usagestats.AccountUsageStatsResponse, error) {
	rows, err := r.queryAccountUsageAggregateRows(ctx, accountID, startTime, endTime)
	if err != nil {
		return nil, err
	}

	historyByDate := make(map[string]*usagestats.AccountUsageHistory)
	modelsByName := make(map[string]*usagestats.ModelStat)
	inboundByEndpoint := make(map[string]*usagestats.EndpointStat)
	upstreamByEndpoint := make(map[string]*usagestats.EndpointStat)
	var durationSumMs, durationSampleCount int64
	var ttftSumMs, ttftSampleCount int64

	for _, row := range rows {
		totalTokens := row.InputTokens + row.OutputTokens + row.CacheCreationTokens + row.CacheReadTokens

		history := historyByDate[row.Date]
		if history == nil {
			parsed, _ := time.Parse("2006-01-02", row.Date)
			history = &usagestats.AccountUsageHistory{Date: row.Date, Label: parsed.Format("01/02")}
			historyByDate[row.Date] = history
		}
		history.Requests += row.Requests
		history.Tokens += totalTokens
		history.Cost += row.StandardCost
		history.ActualCost += row.AccountCost
		history.UserCost += row.UserCost

		model := modelsByName[row.Model]
		if model == nil {
			model = &usagestats.ModelStat{Model: row.Model}
			modelsByName[row.Model] = model
		}
		model.Requests += row.Requests
		model.InputTokens += row.InputTokens
		model.OutputTokens += row.OutputTokens
		model.CacheCreationTokens += row.CacheCreationTokens
		model.CacheReadTokens += row.CacheReadTokens
		model.TotalTokens += totalTokens
		model.Cost += row.StandardCost
		model.ActualCost += row.AccountCost
		model.AccountCost += row.AccountCost

		mergeAccountUsageEndpoint(inboundByEndpoint, row.InboundEndpoint, row.Requests, totalTokens, row.StandardCost, row.AccountCost)
		mergeAccountUsageEndpoint(upstreamByEndpoint, row.UpstreamEndpoint, row.Requests, totalTokens, row.StandardCost, row.AccountCost)

		durationSumMs += row.DurationSumMs
		durationSampleCount += row.DurationSampleCount
		ttftSumMs += row.TTFTSumMs
		ttftSampleCount += row.TTFTSampleCount
	}

	history := make([]usagestats.AccountUsageHistory, 0, len(historyByDate))
	for _, item := range historyByDate {
		history = append(history, *item)
	}
	sort.Slice(history, func(i, j int) bool { return history[i].Date < history[j].Date })

	models := make([]usagestats.ModelStat, 0, len(modelsByName))
	for _, item := range modelsByName {
		models = append(models, *item)
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].TotalTokens != models[j].TotalTokens {
			return models[i].TotalTokens > models[j].TotalTokens
		}
		return models[i].Model < models[j].Model
	})

	endpoints := sortedAccountUsageEndpoints(inboundByEndpoint)
	upstreamEndpoints := sortedAccountUsageEndpoints(upstreamByEndpoint)
	summary := buildAccountUsageSummary(history, startTime, endTime, durationSumMs, durationSampleCount, ttftSumMs, ttftSampleCount)

	return &usagestats.AccountUsageStatsResponse{
		History:           history,
		Summary:           summary,
		Models:            models,
		Endpoints:         endpoints,
		UpstreamEndpoints: upstreamEndpoints,
	}, nil
}

func (r *usageLogRepository) queryAccountUsageAggregateRows(ctx context.Context, accountID int64, startTime, endTime time.Time) (result []accountUsageAggregateRow, err error) {
	query := `
		WITH raw_normalized AS (
			SELECT
				id AS usage_log_id,
				(created_at AT TIME ZONE $4)::date AS bucket_date,
				account_id,
				COALESCE(NULLIF(TRIM(requested_model), ''), model) AS model,
				COALESCE(NULLIF(TRIM(inbound_endpoint), ''), 'unknown') AS inbound_endpoint,
				COALESCE(NULLIF(TRIM(upstream_endpoint), ''), 'unknown') AS upstream_endpoint,
				input_tokens,
				output_tokens,
				cache_creation_tokens,
				cache_read_tokens,
				total_cost AS standard_cost,
				COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1) AS account_cost,
				actual_cost AS user_cost,
				duration_ms,
				CASE
					WHEN COALESCE(image_count, 0) = 0 AND COALESCE(video_count, 0) = 0
					THEN first_token_ms
				END AS first_token_ms
			FROM usage_logs
			WHERE account_id = $1 AND created_at >= $2 AND created_at < $3
		),
		raw_delta AS (
			SELECT
				raw.bucket_date,
				raw.model,
				raw.inbound_endpoint,
				raw.upstream_endpoint,
				COUNT(*) AS request_count,
				COALESCE(SUM(raw.input_tokens), 0) AS input_tokens,
				COALESCE(SUM(raw.output_tokens), 0) AS output_tokens,
				COALESCE(SUM(raw.cache_creation_tokens), 0) AS cache_creation_tokens,
				COALESCE(SUM(raw.cache_read_tokens), 0) AS cache_read_tokens,
				COALESCE(SUM(raw.standard_cost), 0) AS standard_cost,
				COALESCE(SUM(raw.account_cost), 0) AS account_cost,
				COALESCE(SUM(raw.user_cost), 0) AS user_cost,
				COALESCE(SUM(raw.duration_ms), 0) AS duration_sum_ms,
				COUNT(raw.duration_ms) AS duration_sample_count,
				COALESCE(SUM(raw.first_token_ms), 0) AS ttft_sum_ms,
				COUNT(raw.first_token_ms) AS ttft_sample_count
			FROM raw_normalized raw
			LEFT JOIN ops_account_usage_daily archived
				ON archived.bucket_date = raw.bucket_date
				AND archived.account_id = raw.account_id
				AND archived.model = raw.model
				AND archived.inbound_endpoint = raw.inbound_endpoint
				AND archived.upstream_endpoint = raw.upstream_endpoint
			WHERE raw.usage_log_id > COALESCE(archived.last_usage_log_id, 0)
			GROUP BY 1, 2, 3, 4
		),
		combined AS (
			SELECT
				bucket_date,
				model,
				inbound_endpoint,
				upstream_endpoint,
				request_count,
				input_tokens,
				output_tokens,
				cache_creation_tokens,
				cache_read_tokens,
				standard_cost,
				account_cost,
				user_cost,
				duration_sum_ms,
				duration_sample_count,
				ttft_sum_ms,
				ttft_sample_count
			FROM ops_account_usage_daily
			WHERE account_id = $1
				AND bucket_date >= ($2 AT TIME ZONE $4)::date
				AND bucket_date < ($3 AT TIME ZONE $4)::date
			UNION ALL
			SELECT
				bucket_date,
				model,
				inbound_endpoint,
				upstream_endpoint,
				request_count,
				input_tokens,
				output_tokens,
				cache_creation_tokens,
				cache_read_tokens,
				standard_cost,
				account_cost,
				user_cost,
				duration_sum_ms,
				duration_sample_count,
				ttft_sum_ms,
				ttft_sample_count
			FROM raw_delta
		)
		SELECT
			TO_CHAR(bucket_date, 'YYYY-MM-DD') AS date,
			model,
			inbound_endpoint,
			upstream_endpoint,
			request_count,
			input_tokens,
			output_tokens,
			cache_creation_tokens,
			cache_read_tokens,
			standard_cost,
			account_cost,
			user_cost,
			duration_sum_ms,
			duration_sample_count,
			ttft_sum_ms,
			ttft_sample_count
		FROM combined
		ORDER BY bucket_date ASC, model ASC, inbound_endpoint ASC, upstream_endpoint ASC
	`

	rows, err := r.sql.QueryContext(ctx, query, accountID, startTime, endTime, timezone.Name())
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
			result = nil
		}
	}()

	result = make([]accountUsageAggregateRow, 0)
	for rows.Next() {
		var row accountUsageAggregateRow
		if err := rows.Scan(
			&row.Date,
			&row.Model,
			&row.InboundEndpoint,
			&row.UpstreamEndpoint,
			&row.Requests,
			&row.InputTokens,
			&row.OutputTokens,
			&row.CacheCreationTokens,
			&row.CacheReadTokens,
			&row.StandardCost,
			&row.AccountCost,
			&row.UserCost,
			&row.DurationSumMs,
			&row.DurationSampleCount,
			&row.TTFTSumMs,
			&row.TTFTSampleCount,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func mergeAccountUsageEndpoint(target map[string]*usagestats.EndpointStat, endpoint string, requests, totalTokens int64, standardCost, accountCost float64) {
	item := target[endpoint]
	if item == nil {
		item = &usagestats.EndpointStat{Endpoint: endpoint}
		target[endpoint] = item
	}
	item.Requests += requests
	item.TotalTokens += totalTokens
	item.Cost += standardCost
	item.ActualCost += accountCost
}

func sortedAccountUsageEndpoints(items map[string]*usagestats.EndpointStat) []usagestats.EndpointStat {
	result := make([]usagestats.EndpointStat, 0, len(items))
	for _, item := range items {
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Requests != result[j].Requests {
			return result[i].Requests > result[j].Requests
		}
		return result[i].Endpoint < result[j].Endpoint
	})
	return result
}

func buildAccountUsageSummary(history []usagestats.AccountUsageHistory, startTime, endTime time.Time, durationSumMs, durationSampleCount, ttftSumMs, ttftSampleCount int64) usagestats.AccountUsageSummary {
	daysCount := accountUsageCalendarDayCount(startTime, endTime)
	var totalAccountCost, totalUserCost, totalStandardCost float64
	var totalRequests, totalTokens int64
	var highestCostDay, highestRequestDay *usagestats.AccountUsageHistory

	for i := range history {
		item := &history[i]
		totalAccountCost += item.ActualCost
		totalUserCost += item.UserCost
		totalStandardCost += item.Cost
		totalRequests += item.Requests
		totalTokens += item.Tokens
		if highestCostDay == nil || item.ActualCost > highestCostDay.ActualCost {
			highestCostDay = item
		}
		if highestRequestDay == nil || item.Requests > highestRequestDay.Requests {
			highestRequestDay = item
		}
	}

	actualDaysUsed := len(history)
	averageDivisor := actualDaysUsed
	if averageDivisor == 0 {
		averageDivisor = 1
	}
	summary := usagestats.AccountUsageSummary{
		Days:              daysCount,
		ActualDaysUsed:    actualDaysUsed,
		TotalCost:         totalAccountCost,
		TotalUserCost:     totalUserCost,
		TotalStandardCost: totalStandardCost,
		TotalRequests:     totalRequests,
		TotalTokens:       totalTokens,
		AvgDailyCost:      totalAccountCost / float64(averageDivisor),
		AvgDailyUserCost:  totalUserCost / float64(averageDivisor),
		AvgDailyRequests:  float64(totalRequests) / float64(averageDivisor),
		AvgDailyTokens:    float64(totalTokens) / float64(averageDivisor),
	}
	if durationSampleCount > 0 {
		summary.AvgDurationMs = float64(durationSumMs) / float64(durationSampleCount)
	}
	if ttftSampleCount > 0 {
		value := float64(ttftSumMs) / float64(ttftSampleCount)
		summary.AvgFirstTokenMs = &value
	}

	today := timezone.Now().Format("2006-01-02")
	for i := range history {
		if history[i].Date != today {
			continue
		}
		summary.Today = &struct {
			Date     string  `json:"date"`
			Cost     float64 `json:"cost"`
			UserCost float64 `json:"user_cost"`
			Requests int64   `json:"requests"`
			Tokens   int64   `json:"tokens"`
		}{
			Date:     history[i].Date,
			Cost:     history[i].ActualCost,
			UserCost: history[i].UserCost,
			Requests: history[i].Requests,
			Tokens:   history[i].Tokens,
		}
		break
	}

	if highestCostDay != nil {
		summary.HighestCostDay = &struct {
			Date     string  `json:"date"`
			Label    string  `json:"label"`
			Cost     float64 `json:"cost"`
			UserCost float64 `json:"user_cost"`
			Requests int64   `json:"requests"`
		}{
			Date:     highestCostDay.Date,
			Label:    highestCostDay.Label,
			Cost:     highestCostDay.ActualCost,
			UserCost: highestCostDay.UserCost,
			Requests: highestCostDay.Requests,
		}
	}
	if highestRequestDay != nil {
		summary.HighestRequestDay = &struct {
			Date     string  `json:"date"`
			Label    string  `json:"label"`
			Requests int64   `json:"requests"`
			Cost     float64 `json:"cost"`
			UserCost float64 `json:"user_cost"`
		}{
			Date:     highestRequestDay.Date,
			Label:    highestRequestDay.Label,
			Requests: highestRequestDay.Requests,
			Cost:     highestRequestDay.ActualCost,
			UserCost: highestRequestDay.UserCost,
		}
	}
	return summary
}

func accountUsageCalendarDayCount(startTime, endTime time.Time) int {
	if !endTime.After(startTime) {
		return 30
	}
	start := timezone.StartOfDay(startTime)
	end := timezone.StartOfDay(endTime)
	days := 0
	for cursor := start; cursor.Before(end); cursor = cursor.AddDate(0, 0, 1) {
		days++
	}
	if days <= 0 {
		return 30
	}
	return days
}
