package service

import (
	"bytes"
	"encoding/json"
	"sort"
	"time"
)

// UnmarshalJSON accepts both the App Server account/usage/read response and
// the compatible /wham/profiles/me response used by ChatGPT web clients.
// App Server uses camelCase under result.summary; the profile endpoint uses a
// stats object with snake_case fields.
func (u *OpenAIServerTokenUsage) UnmarshalJSON(data []byte) error {
	if u == nil {
		return nil
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err == nil {
		if nested := firstJSONRaw(envelope["result"]); hasTokenUsagePayload(nested) {
			return json.Unmarshal(nested, u)
		}
	}

	var payload struct {
		Summary             json.RawMessage `json:"summary"`
		Stats               json.RawMessage `json:"stats"`
		DailyUsageBuckets   json.RawMessage `json:"dailyUsageBuckets"`
		DailyUsageBucketsDB json.RawMessage `json:"daily_usage_buckets"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	var summary OpenAITokenUsageSummary
	summaryRaw := firstJSONRaw(payload.Summary, payload.Stats)
	if len(summaryRaw) > 0 {
		summary = decodeOpenAITokenUsageSummary(summaryRaw)
	}
	dailyRaw := firstJSONRaw(payload.DailyUsageBuckets, payload.DailyUsageBucketsDB)
	if len(dailyRaw) == 0 && len(payload.Stats) > 0 {
		var statsFields map[string]json.RawMessage
		if err := json.Unmarshal(payload.Stats, &statsFields); err == nil {
			dailyRaw = firstJSONRaw(statsFields["dailyUsageBuckets"], statsFields["daily_usage_buckets"])
		}
	}
	buckets := decodeOpenAITokenUsageBuckets(dailyRaw)
	var cycle struct {
		Tokens       json.RawMessage `json:"currentResetCycleTokens"`
		TokensSnake  json.RawMessage `json:"current_reset_cycle_tokens"`
		WindowMins   json.RawMessage `json:"currentResetCycleWindowMinutes"`
		WindowMinsDB json.RawMessage `json:"current_reset_cycle_window_minutes"`
		LimitID      json.RawMessage `json:"currentResetCycleLimitId"`
		LimitIDSnake json.RawMessage `json:"current_reset_cycle_limit_id"`
		Approx       json.RawMessage `json:"currentResetCycleApproximate"`
		ApproxSnake  json.RawMessage `json:"current_reset_cycle_approximate"`
	}
	if err := json.Unmarshal(data, &cycle); err != nil {
		return err
	}
	cycleTokens, _ := firstJSONInt(cycle.Tokens, cycle.TokensSnake)
	cycleWindow, _ := firstJSONInt(cycle.WindowMins, cycle.WindowMinsDB)
	cycleLimitID, _ := firstJSONString(cycle.LimitID, cycle.LimitIDSnake)
	cycleApproximate := firstJSONBool(cycle.Approx, cycle.ApproxSnake)
	var cycleTokensPtr *int64
	if len(firstJSONRaw(cycle.Tokens, cycle.TokensSnake)) > 0 {
		cycleTokensPtr = &cycleTokens
	}
	*u = OpenAIServerTokenUsage{
		Summary:                        summary,
		DailyUsageBuckets:              buckets,
		CurrentResetCycleTokens:        cycleTokensPtr,
		CurrentResetCycleWindowMinutes: cycleWindow,
		CurrentResetCycleLimitID:       cycleLimitID,
		CurrentResetCycleApproximate:   cycleApproximate,
	}
	return nil
}

type serverUsageCycleWindow struct {
	limitID string
	minutes int64
	resets  int64
}

// enrichServerTokenUsageResetCycle derives the current cycle total from the
// server's daily buckets and the authoritative rate-limit reset window. The
// service does not expose per-request counts for this endpoint, so this is a
// token-only counter. Windows shorter than one day necessarily remain
// approximate because the server buckets are daily.
func enrichServerTokenUsageResetCycle(usage *OpenAIServerTokenUsage, quota *OpenAIQuotaUsage, now time.Time) {
	if usage == nil || quota == nil || len(usage.DailyUsageBuckets) == 0 {
		return
	}
	window, ok := selectServerUsageCycleWindow(quota)
	if !ok || window.minutes <= 0 || window.resets <= 0 {
		return
	}
	duration := time.Duration(window.minutes) * time.Minute
	resetAt := time.Unix(window.resets, 0).UTC()
	if !resetAt.After(now) {
		elapsed := now.Sub(resetAt)
		steps := elapsed/duration + 1
		resetAt = resetAt.Add(time.Duration(steps) * duration)
	}
	startAt := resetAt.Add(-duration)
	var total int64
	matched := false
	for _, bucket := range usage.DailyUsageBuckets {
		day, err := time.ParseInLocation("2006-01-02", bucket.StartDate, time.UTC)
		if err != nil || day.After(now) || !day.Add(24*time.Hour).After(startAt) || !day.Before(resetAt) {
			continue
		}
		total += bucket.Tokens
		matched = true
	}
	if !matched {
		return
	}
	usage.CurrentResetCycleTokens = &total
	usage.CurrentResetCycleWindowMinutes = window.minutes
	usage.CurrentResetCycleLimitID = window.limitID
	usage.CurrentResetCycleApproximate = window.minutes < 24*60 || startAt.Hour() != 0 || startAt.Minute() != 0
}

func selectServerUsageCycleWindow(quota *OpenAIQuotaUsage) (serverUsageCycleWindow, bool) {
	if quota == nil {
		return serverUsageCycleWindow{}, false
	}
	candidates := make([]serverUsageCycleWindow, 0, 4)
	if bucket, ok := quota.RateLimitsByLimitID["codex"]; ok {
		candidates = append(candidates, serverUsageCycleWindowsForBucket("codex", bucket)...)
	}
	keys := make([]string, 0, len(quota.RateLimitsByLimitID))
	for key := range quota.RateLimitsByLimitID {
		if key != "codex" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		candidates = append(candidates, serverUsageCycleWindowsForBucket(key, quota.RateLimitsByLimitID[key])...)
	}
	if len(candidates) == 0 && quota.RateLimit != nil {
		for _, window := range []*OpenAIRateLimitWindow{quota.RateLimit.PrimaryWindow, quota.RateLimit.SecondaryWindow} {
			if window != nil && window.LimitWindowSeconds > 0 && window.ResetAt > 0 {
				candidates = append(candidates, serverUsageCycleWindow{limitID: quota.RateLimit.LimitID, minutes: window.LimitWindowSeconds / 60, resets: window.ResetAt})
			}
		}
	}
	if len(candidates) == 0 {
		return serverUsageCycleWindow{}, false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].minutes != candidates[j].minutes {
			return candidates[i].minutes > candidates[j].minutes
		}
		if candidates[i].limitID == "codex" && candidates[j].limitID != "codex" {
			return true
		}
		return candidates[i].limitID < candidates[j].limitID
	})
	return candidates[0], true
}

func serverUsageCycleWindowsForBucket(limitID string, bucket OpenAIAppServerRateLimitBucket) []serverUsageCycleWindow {
	result := make([]serverUsageCycleWindow, 0, 2)
	for _, window := range []*OpenAIAppServerRateLimitWindow{bucket.Primary, bucket.Secondary} {
		if window != nil && window.WindowDurationMins > 0 && window.ResetsAt > 0 {
			result = append(result, serverUsageCycleWindow{limitID: limitID, minutes: window.WindowDurationMins, resets: window.ResetsAt})
		}
	}
	if len(result) == 0 && bucket.WindowDurationMins > 0 && bucket.ResetsAt > 0 {
		result = append(result, serverUsageCycleWindow{limitID: limitID, minutes: bucket.WindowDurationMins, resets: bucket.ResetsAt})
	}
	return result
}

func hasTokenUsagePayload(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return false
	}
	_, summary := fields["summary"]
	_, stats := fields["stats"]
	_, daily := fields["dailyUsageBuckets"]
	_, dailySnake := fields["daily_usage_buckets"]
	return summary || stats || daily || dailySnake
}

func decodeOpenAITokenUsageSummary(raw json.RawMessage) OpenAITokenUsageSummary {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return OpenAITokenUsageSummary{}
	}
	return OpenAITokenUsageSummary{
		LifetimeTokens:            optionalJSONInt(fields, "lifetimeTokens", "lifetime_tokens"),
		PeakDailyTokens:           optionalJSONInt(fields, "peakDailyTokens", "peak_daily_tokens"),
		LongestRunningTurnSeconds: optionalJSONInt(fields, "longestRunningTurnSec", "longest_running_turn_sec", "longest_running_turn_seconds"),
		CurrentStreakDays:         optionalJSONInt(fields, "currentStreakDays", "current_streak_days"),
		LongestStreakDays:         optionalJSONInt(fields, "longestStreakDays", "longest_streak_days"),
	}
}

func optionalJSONInt(fields map[string]json.RawMessage, keys ...string) *int64 {
	values := make([]json.RawMessage, 0, len(keys))
	for _, key := range keys {
		if value, ok := fields[key]; ok {
			values = append(values, value)
		}
	}
	value, ok := firstJSONInt(values...)
	if !ok {
		return nil
	}
	return &value
}

func firstJSONBool(values ...json.RawMessage) bool {
	for _, value := range values {
		trimmed := bytes.TrimSpace(value)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			continue
		}
		var result bool
		if json.Unmarshal(trimmed, &result) == nil {
			return result
		}
	}
	return false
}

func decodeOpenAITokenUsageBuckets(raw json.RawMessage) []OpenAITokenUsageDailyBucket {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}
	buckets := make([]OpenAITokenUsageDailyBucket, 0, len(items))
	for _, item := range items {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(item, &fields); err != nil {
			continue
		}
		startDate, _ := firstJSONString(fields["startDate"], fields["start_date"])
		tokens, ok := firstJSONInt(fields["tokens"])
		if !ok || startDate == "" {
			continue
		}
		buckets = append(buckets, OpenAITokenUsageDailyBucket{StartDate: startDate, Tokens: tokens})
	}
	return buckets
}
