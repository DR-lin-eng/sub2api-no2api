package xai

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// GrokQuotaSignalMaxAge bounds how long a grok-4.5 Responses window can
// influence SuperGrok vs Heavy inference.
const GrokQuotaSignalMaxAge = 24 * time.Hour

const (
	grok45ResponsesModel             = "grok-4.5"
	grokHeavyQuotaRequestLimit int64 = 8_300
	grokHeavyQuotaTokenLimit   int64 = 53_000_000
)

// MapJWTSubscriptionTier maps prod_auth.SubscriptionTier numeric JWT claims
// to stable snake_case keys used by Grok Build / Mixpanel.
func MapJWTSubscriptionTier(tier uint64) string {
	switch tier {
	case 0:
		return "free"
	case 1:
		return "supergrok"
	case 2:
		return "x_basic"
	case 3:
		return "x_premium"
	case 4:
		return "x_premium_plus"
	case 5:
		return "supergrok_heavy"
	case 6:
		return "supergrok_lite"
	case 7:
		return "supergrok_plus"
	default:
		return strconv.FormatUint(tier, 10)
	}
}

// NormalizeSubscriptionTier canonicalizes display names, /user strings, and
// JWT-derived keys onto the same snake_case identifiers.
func NormalizeSubscriptionTier(raw string) string {
	tier := strings.ToLower(strings.TrimSpace(raw))
	tier = strings.ReplaceAll(tier, "-", "_")
	tier = strings.Join(strings.Fields(tier), "_")
	switch tier {
	case "free", "grok_free", "grokfree", "free_tier", "freetier", "grok_basic", "grokbasic":
		return "free"
	case "supergrok", "grokpro":
		return "supergrok"
	case "supergrok_lite", "supergroklite":
		return "supergrok_lite"
	case "supergrok_heavy", "supergrokheavy":
		return "supergrok_heavy"
	case "supergrok_pro", "supergrokpro":
		return "supergrok_pro"
	case "supergrok_plus", "supergrokplus":
		return "supergrok_plus"
	case "x_basic", "xbasic", "basic":
		return "x_basic"
	case "x_premium", "xpremium":
		return "x_premium"
	case "x_premium_plus", "xpremiumplus", "x_premium+":
		return "x_premium_plus"
	default:
		return tier
	}
}

// SubscriptionTierFromJWT decodes an access token payload without verifying
// its signature and maps the numeric or string tier claim.
func SubscriptionTierFromJWT(token string) string {
	claims := DecodeJWTClaims(token)
	if claims == nil {
		return ""
	}
	raw, ok := claims["tier"]
	if !ok || raw == nil {
		return ""
	}
	switch value := raw.(type) {
	case float64:
		if value < 0 {
			return ""
		}
		return MapJWTSubscriptionTier(uint64(value))
	case json.Number:
		number, err := value.Int64()
		if err != nil || number < 0 {
			return NormalizeSubscriptionTier(value.String())
		}
		return MapJWTSubscriptionTier(uint64(number))
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return ""
		}
		if number, err := strconv.ParseUint(trimmed, 10, 64); err == nil {
			return MapJWTSubscriptionTier(number)
		}
		return NormalizeSubscriptionTier(trimmed)
	default:
		return ""
	}
}

// CanonicalGrokPlan resolves SuperGrok vs Heavy when the provider label is
// ambiguous. A fresh grok-4.5 Responses quota window is the only rate-limit
// signal that may disambiguate the plan.
func CanonicalGrokPlan(monthlyLimitCents *float64, subscriptionTier string, quota *QuotaSnapshot) string {
	if plan := resolvePlan(monthlyLimitCents); plan != "" {
		return NormalizeSubscriptionTier(plan)
	}

	normalized := NormalizeSubscriptionTier(subscriptionTier)
	switch normalized {
	case "free", "x_basic":
		return "free"
	case "supergrok_heavy", "supergrok_lite", "supergrok_plus":
		return normalized
	}

	if isAmbiguousGrokPaidPlan(normalized) {
		if hint := Grok45ResponsesPlanHint(quota, time.Time{}); hint != "" {
			return hint
		}
		return "supergrok"
	}
	return ""
}

func isAmbiguousGrokPaidPlan(normalized string) bool {
	switch normalized {
	case "supergrok", "supergrok_pro", "paid", "pro":
		return true
	default:
		return false
	}
}

// IsGrok45ResponsesQuotaModel reports whether model is the grok-4.5 Responses
// id or a dated grok-4.5 variant.
func IsGrok45ResponsesQuotaModel(model string) bool {
	native := strings.ToLower(strings.TrimSpace(StripGrokProviderPrefix(model)))
	return native == grok45ResponsesModel || strings.HasPrefix(native, grok45ResponsesModel+"-")
}

// Grok45ResponsesPlanHint returns a plan inferred from a fresh grok-4.5
// Responses quota window. Other models' limits are ignored.
func Grok45ResponsesPlanHint(quota *QuotaSnapshot, now time.Time) string {
	if quota == nil {
		return ""
	}
	if plan := NormalizeSubscriptionTier(quota.PlanFrom45Responses); plan == "supergrok" || plan == "supergrok_heavy" {
		if isQuotaTimestampFresh(quota.PlanFrom45ResponsesAt, now) {
			return plan
		}
	}
	if !IsGrok45ResponsesQuotaModel(quota.Model) || !IsQuotaSnapshotFresh(quota, now) {
		return ""
	}
	if quotaLooksLikeGrokHeavy(quota) {
		return "supergrok_heavy"
	}
	return ""
}

// ApplyGrok45ResponsesPlanSignal records a grok-4.5 plan hint, or carries the
// previous hint across a later snapshot from another model.
func (s *QuotaSnapshot) ApplyGrok45ResponsesPlanSignal(previous *QuotaSnapshot) {
	if s == nil {
		return
	}
	observedAt := firstNonEmptyQuotaTime(s.LastHeadersSeenAt, s.UpdatedAt)
	if IsGrok45ResponsesQuotaModel(s.Model) && quotaHasLimitWindow(s) {
		if quotaLooksLikeGrokHeavy(s) {
			s.PlanFrom45Responses = "supergrok_heavy"
		} else {
			s.PlanFrom45Responses = "supergrok"
		}
		s.PlanFrom45ResponsesAt = observedAt
		return
	}
	if previous != nil && strings.TrimSpace(previous.PlanFrom45Responses) != "" {
		s.PlanFrom45Responses = previous.PlanFrom45Responses
		s.PlanFrom45ResponsesAt = previous.PlanFrom45ResponsesAt
	}
}

// QuotaSnapshotObservedAt prefers the original header observation time so a
// later snapshot rewrite cannot refresh a stale plan signal.
func QuotaSnapshotObservedAt(snapshot *QuotaSnapshot) (time.Time, bool) {
	if snapshot == nil {
		return time.Time{}, false
	}
	return parseQuotaTimestamp(firstNonEmptyQuotaTime(snapshot.LastHeadersSeenAt, snapshot.UpdatedAt))
}

func IsQuotaSnapshotFresh(snapshot *QuotaSnapshot, now time.Time) bool {
	observedAt, ok := QuotaSnapshotObservedAt(snapshot)
	return ok && isTimeFresh(observedAt, now)
}

func isQuotaTimestampFresh(raw string, now time.Time) bool {
	parsed, ok := parseQuotaTimestamp(raw)
	return ok && isTimeFresh(parsed, now)
}

func parseQuotaTimestamp(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	return parsed, err == nil
}

func isTimeFresh(observedAt, now time.Time) bool {
	if now.IsZero() {
		now = time.Now()
	}
	age := now.Sub(observedAt)
	return age <= GrokQuotaSignalMaxAge && age >= -5*time.Minute
}

func quotaHasLimitWindow(quota *QuotaSnapshot) bool {
	return quota != nil && ((quota.Requests != nil && quota.Requests.Limit != nil) ||
		(quota.Tokens != nil && quota.Tokens.Limit != nil))
}

func quotaLooksLikeGrokHeavy(quota *QuotaSnapshot) bool {
	if quota == nil {
		return false
	}
	var requestLimit, tokenLimit int64
	if quota.Requests != nil && quota.Requests.Limit != nil {
		requestLimit = *quota.Requests.Limit
	}
	if quota.Tokens != nil && quota.Tokens.Limit != nil {
		tokenLimit = *quota.Tokens.Limit
	}
	return requestLimit >= grokHeavyQuotaRequestLimit || tokenLimit >= grokHeavyQuotaTokenLimit
}

func firstNonEmptyQuotaTime(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
