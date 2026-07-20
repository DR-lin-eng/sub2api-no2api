package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

type grokBillingEligibilitySnapshot struct {
	status, weeklyStatus, monthlyStatus int
	plan                                string
	usageObserved                       bool
	usedObserved                        bool
	monthlyLimitCents                   float64
	monthlyLimitObserved                bool
	monthlyUpdatedAt                    string
	partial                             bool
	failedWindows                       int
}

func grokBillingEligibilitySnapshotFromExtra(extra map[string]any) (grokBillingEligibilitySnapshot, bool) {
	if extra == nil {
		return grokBillingEligibilitySnapshot{}, false
	}
	raw, ok := extra[grokBillingExtraKey]
	if !ok || raw == nil {
		return grokBillingEligibilitySnapshot{}, false
	}
	switch billing := raw.(type) {
	case *xai.BillingSummary:
		if billing == nil {
			return grokBillingEligibilitySnapshot{}, false
		}
		return grokBillingEligibilitySnapshot{
			status: billing.StatusCode, weeklyStatus: billing.WeeklyStatusCode, monthlyStatus: billing.MonthlyStatusCode,
			plan: billing.Plan, usageObserved: billing.UsagePercent != nil, usedObserved: billing.UsedPercent != nil,
			monthlyLimitCents: derefFloat64(billing.MonthlyLimitCents), monthlyLimitObserved: billing.MonthlyLimitCents != nil,
			monthlyUpdatedAt: billing.MonthlyUpdatedAt, partial: billing.Partial, failedWindows: len(billing.FailedWindows),
		}, true
	case xai.BillingSummary:
		return grokBillingEligibilitySnapshot{
			status: billing.StatusCode, weeklyStatus: billing.WeeklyStatusCode, monthlyStatus: billing.MonthlyStatusCode,
			plan: billing.Plan, usageObserved: billing.UsagePercent != nil, usedObserved: billing.UsedPercent != nil,
			monthlyLimitCents: derefFloat64(billing.MonthlyLimitCents), monthlyLimitObserved: billing.MonthlyLimitCents != nil,
			monthlyUpdatedAt: billing.MonthlyUpdatedAt, partial: billing.Partial, failedWindows: len(billing.FailedWindows),
		}, true
	case map[string]any:
		status, statusOK := cachedBillingStatusCode(billing, "status_code")
		weekly, weeklyOK := cachedBillingStatusCode(billing, "weekly_status_code")
		monthly, monthlyOK := cachedBillingStatusCode(billing, "monthly_status_code")
		if !statusOK || !weeklyOK || !monthlyOK {
			return grokBillingEligibilitySnapshot{}, false
		}
		_, usageOK := cachedBillingFloatValue(billing, "usage_percent")
		_, usedOK := cachedBillingFloatValue(billing, "used_percent")
		monthlyLimit, monthlyLimitOK := cachedBillingFloatValue(billing, "monthly_limit_cents")
		partial, _ := billing["partial"].(bool)
		monthlyUpdatedAt, _ := billing["monthly_updated_at"].(string)
		failedWindows := 0
		if values, ok := billing["failed_windows"].([]any); ok {
			failedWindows = len(values)
		}
		plan, _ := billing["plan"].(string)
		return grokBillingEligibilitySnapshot{
			status: status, weeklyStatus: weekly, monthlyStatus: monthly, plan: plan,
			usageObserved: usageOK, usedObserved: usedOK, monthlyLimitCents: monthlyLimit,
			monthlyLimitObserved: monthlyLimitOK, monthlyUpdatedAt: monthlyUpdatedAt,
			partial: partial, failedWindows: failedWindows,
		}, true
	default:
		return grokBillingEligibilitySnapshot{}, false
	}
}

func derefFloat64(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func cachedBillingFloatValue(values map[string]any, key string) (float64, bool) {
	value, ok := values[key]
	if !ok || value == nil {
		return 0, false
	}
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

const grokQuotaSnapshotExtraKey = "grok_usage_snapshot"

type GrokQuotaFetcher struct{}

func NewGrokQuotaFetcher() *GrokQuotaFetcher {
	return &GrokQuotaFetcher{}
}

func (f *GrokQuotaFetcher) BuildUsageInfo(account *Account) *UsageInfo {
	now := time.Now()
	usage := &UsageInfo{
		Source:             "passive",
		UpdatedAt:          &now,
		GrokFreeTokenLimit: xai.GrokFreeRolling24hTokenLimit,
	}
	if account == nil {
		usage.ErrorCode = "quota_unknown"
		usage.Error = "Grok quota is unknown until billing is probed or an upstream response includes xAI rate-limit headers"
		return usage
	}

	billing, _ := grokBillingSnapshotFromExtra(account.Extra)
	snapshot, err := grokQuotaSnapshotFromExtra(account.Extra)
	activeProbeClearsForbidden := newerSuccessfulGrokActiveProbeClearsBillingForbidden(billing, snapshot)
	if billing != nil {
		usage.GrokBilling = billing
		if billing.Plan != "" {
			usage.SubscriptionTier = billing.Plan
			usage.SubscriptionTierRaw = billing.Plan
		}
		if parsedAt, parseErr := time.Parse(time.RFC3339, billing.UpdatedAt); parseErr == nil {
			usage.UpdatedAt = &parsedAt
		}
		if billing.FetchedAt != "" {
			usage.GrokLastQuotaProbeAt = billing.FetchedAt
		}
		usage.GrokQuotaSnapshotState = "billing_observed"
		usage.GrokLastStatusCode = billing.StatusCode
		switch billing.StatusCode {
		case 401:
			usage.NeedsReauth = true
			usage.ErrorCode = "unauthenticated"
		case 403:
			usage.IsForbidden = true
			usage.ForbiddenType = "forbidden"
			usage.ErrorCode = "forbidden"
		case 429:
			usage.ErrorCode = "rate_limited"
		}
	}

	if err != nil || snapshot == nil {
		applyGrokCredentialUsageFallback(usage, account)
		if billing == nil {
			usage.ErrorCode = "quota_unknown"
			usage.Error = "Grok quota is unknown until billing is probed or an upstream response includes xAI rate-limit headers"
		}
		return usage
	}

	if parsedAt, parseErr := time.Parse(time.RFC3339, snapshot.UpdatedAt); parseErr == nil {
		if billing == nil || usage.UpdatedAt == nil || parsedAt.After(*usage.UpdatedAt) {
			usage.UpdatedAt = &parsedAt
		}
	}
	usage.GrokRequestQuota = snapshot.Requests
	usage.GrokTokenQuota = snapshot.Tokens
	usage.GrokRetryAfterSeconds = snapshot.RetryAfterSeconds
	if usage.SubscriptionTier == "" {
		usage.SubscriptionTier = snapshot.SubscriptionTier
		usage.SubscriptionTierRaw = snapshot.SubscriptionTier
	}
	if usage.GrokEntitlementStatus == "" {
		usage.GrokEntitlementStatus = snapshot.EntitlementStatus
	}
	if usage.GrokLastQuotaProbeAt == "" {
		usage.GrokLastQuotaProbeAt = snapshot.LastProbeAt
	}
	usage.GrokLastHeadersSeenAt = snapshot.LastHeadersSeenAt
	if activeProbeClearsForbidden {
		usage.IsForbidden = false
		usage.ForbiddenType = ""
		usage.ErrorCode = ""
		usage.GrokLastQuotaProbeAt = snapshot.LastProbeAt
		usage.GrokLastStatusCode = snapshot.StatusCode
	} else if snapshot.StatusCode >= http.StatusBadRequest || usage.GrokLastStatusCode == 0 {
		usage.GrokLastStatusCode = snapshot.StatusCode
	}
	if snapshot.HasObservedHeaders() {
		if usage.GrokQuotaSnapshotState == "" {
			usage.GrokQuotaSnapshotState = "observed"
		}
	} else if billing == nil {
		usage.GrokQuotaSnapshotState = "no_headers"
		usage.ErrorCode = "quota_unknown"
		usage.Error = "No xAI quota headers observed on the latest Grok probe"
	}

	if usage.ErrorCode == "" {
		switch snapshot.StatusCode {
		case 401:
			usage.NeedsReauth = true
			usage.ErrorCode = "unauthenticated"
		case 403:
			usage.IsForbidden = true
			usage.ForbiddenType = "forbidden"
			usage.ErrorCode = "forbidden"
			if usage.GrokEntitlementStatus == "" {
				usage.GrokEntitlementStatus = "forbidden"
			}
		case 429:
			usage.ErrorCode = "rate_limited"
		}
	}
	applyGrokCredentialUsageFallback(usage, account)
	if activeProbeClearsForbidden && strings.TrimSpace(snapshot.EntitlementStatus) == "" &&
		strings.EqualFold(strings.TrimSpace(usage.GrokEntitlementStatus), "forbidden") {
		usage.GrokEntitlementStatus = ""
	}
	return usage
}

func newerSuccessfulGrokActiveProbeClearsBillingForbidden(billing *xai.BillingSummary, snapshot *xai.QuotaSnapshot) bool {
	if billing == nil || billing.StatusCode != http.StatusForbidden || snapshot == nil ||
		snapshot.StatusCode != http.StatusOK || strings.TrimSpace(snapshot.ObservationSource) != "active_probe" {
		return false
	}

	billingAt, billingOK := firstGrokObservationTime(billing.UpdatedAt, billing.FetchedAt)
	probeAt, probeOK := firstGrokObservationTime(snapshot.LastProbeAt, snapshot.UpdatedAt)
	// Both snapshots use second precision, so a billing request followed by the
	// active probe in the same refresh can legitimately have equal timestamps.
	return billingOK && probeOK && !probeAt.Before(billingAt)
}

func firstGrokObservationTime(values ...string) (time.Time, bool) {
	for _, value := range values {
		parsedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
		if err == nil {
			return parsedAt, true
		}
	}
	return time.Time{}, false
}

func applyGrokCredentialUsageFallback(usage *UsageInfo, account *Account) {
	if usage == nil || account == nil {
		return
	}
	if usage.SubscriptionTier == "" {
		tier := strings.TrimSpace(account.GetCredential("subscription_tier"))
		usage.SubscriptionTier = tier
		usage.SubscriptionTierRaw = tier
	}
	if usage.GrokEntitlementStatus == "" {
		usage.GrokEntitlementStatus = strings.TrimSpace(account.GetCredential("entitlement_status"))
	}
}

func grokBillingSnapshotFromExtra(extra map[string]any) (*xai.BillingSummary, error) {
	if extra == nil {
		return nil, nil
	}
	raw, ok := extra[grokBillingExtraKey]
	if !ok || raw == nil {
		return nil, nil
	}
	switch snapshot := raw.(type) {
	case *xai.BillingSummary:
		return snapshot, nil
	case xai.BillingSummary:
		return &snapshot, nil
	case map[string]any:
		data, err := json.Marshal(snapshot)
		if err != nil {
			return nil, err
		}
		var out xai.BillingSummary
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return &out, nil
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("marshal grok billing snapshot: %w", err)
		}
		var out xai.BillingSummary
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}
}

func cachedBillingStatusCode(snapshot map[string]any, key string) (int, bool) {
	value, exists := snapshot[key]
	if !exists || value == nil {
		return 0, true
	}
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), int64(int(typed)) == typed
	case float64:
		converted := int(typed)
		return converted, float64(converted) == typed
	case json.Number:
		converted, err := typed.Int64()
		return int(converted), err == nil && int64(int(converted)) == converted
	default:
		return 0, false
	}
}

func grokQuotaSnapshotFromExtra(extra map[string]any) (*xai.QuotaSnapshot, error) {
	if extra == nil {
		return nil, nil
	}
	raw, ok := extra[grokQuotaSnapshotExtraKey]
	if !ok || raw == nil {
		return nil, nil
	}
	switch snapshot := raw.(type) {
	case *xai.QuotaSnapshot:
		return snapshot, nil
	case xai.QuotaSnapshot:
		return &snapshot, nil
	case map[string]any:
		data, err := json.Marshal(snapshot)
		if err != nil {
			return nil, err
		}
		var out xai.QuotaSnapshot
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return &out, nil
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("marshal grok quota snapshot: %w", err)
		}
		var out xai.QuotaSnapshot
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}
}
