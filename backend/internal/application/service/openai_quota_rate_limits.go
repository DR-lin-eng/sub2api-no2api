package service

import (
	"bytes"
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
)

// The ChatGPT App Server documentation uses camelCase for rate-limit
// responses.  The admin API itself is snake_case, and older /wham/usage
// responses use a third (primary_window/secondary_window) shape.  Keep the
// conversion here so the rest of the quota service can work with one stable
// model and so a protocol spelling change cannot silently erase the limits.

type openAIQuotaRateLimitWindowPayload struct {
	UsedPercentCamel        json.RawMessage `json:"usedPercent"`
	UsedPercentSnake        json.RawMessage `json:"used_percent"`
	WindowDurationMinsCamel json.RawMessage `json:"windowDurationMins"`
	WindowDurationMinsSnake json.RawMessage `json:"window_duration_mins"`
	LimitWindowSecondsCamel json.RawMessage `json:"limitWindowSeconds"`
	LimitWindowSecondsSnake json.RawMessage `json:"limit_window_seconds"`
	ResetAfterSecondsCamel  json.RawMessage `json:"resetAfterSeconds"`
	ResetAfterSecondsSnake  json.RawMessage `json:"reset_after_seconds"`
	ResetsAtCamel           json.RawMessage `json:"resetsAt"`
	ResetsAtSnake           json.RawMessage `json:"resets_at"`
	ResetAtCamel            json.RawMessage `json:"resetAt"`
	ResetAtSnake            json.RawMessage `json:"reset_at"`
}

type openAIQuotaRateLimitBucketPayload struct {
	LimitIDCamel              json.RawMessage `json:"limitId"`
	LimitIDSnake              json.RawMessage `json:"limit_id"`
	LimitNameCamel            json.RawMessage `json:"limitName"`
	LimitNameSnake            json.RawMessage `json:"limit_name"`
	UsedPercentCamel          json.RawMessage `json:"usedPercent"`
	UsedPercentSnake          json.RawMessage `json:"used_percent"`
	WindowDurationMinsCamel   json.RawMessage `json:"windowDurationMins"`
	WindowDurationMinsSnake   json.RawMessage `json:"window_duration_mins"`
	ResetsAtCamel             json.RawMessage `json:"resetsAt"`
	ResetsAtSnake             json.RawMessage `json:"resets_at"`
	Primary                   json.RawMessage `json:"primary"`
	PrimaryWindow             json.RawMessage `json:"primary_window"`
	Secondary                 json.RawMessage `json:"secondary"`
	SecondaryWindow           json.RawMessage `json:"secondary_window"`
	RateLimitReachedTypeCamel json.RawMessage `json:"rateLimitReachedType"`
	RateLimitReachedTypeSnake json.RawMessage `json:"rate_limit_reached_type"`
}

// UnmarshalJSON keeps the legacy OpenAIRateLimit type usable by callers that
// decode the App Server's `rateLimits` object directly (outside
// OpenAIQuotaUsage). Legacy /wham/usage fields retain their original decode
// path and reset-after values.
func (r *OpenAIRateLimit) UnmarshalJSON(data []byte) error {
	if r == nil {
		return nil
	}
	type legacyRateLimit OpenAIRateLimit
	var legacy legacyRateLimit
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	*r = OpenAIRateLimit(legacy)
	if !hasAppServerRateLimitShape(data) {
		return nil
	}
	if bucket := decodeOpenAIAppServerRateLimitBucket(data); bucket != nil {
		converted := appServerRateLimitBucketToLegacy(bucket)
		if converted != nil {
			*r = *converted
		}
	}
	return nil
}

func hasAppServerRateLimitShape(data []byte) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return false
	}
	for _, key := range []string{
		"primary",
		"secondary",
		"rateLimitReachedType",
		"rate_limit_reached_type",
	} {
		if _, ok := fields[key]; ok {
			return true
		}
	}
	return false
}

func (w *OpenAIAppServerRateLimitWindow) UnmarshalJSON(data []byte) error {
	if w == nil {
		return nil
	}
	var payload openAIQuotaRateLimitWindowPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	used, _ := firstJSONFloat(payload.UsedPercentCamel, payload.UsedPercentSnake)
	windowMins, hasWindowMins := firstJSONInt(payload.WindowDurationMinsCamel, payload.WindowDurationMinsSnake)
	if !hasWindowMins {
		if seconds, ok := firstJSONInt(payload.LimitWindowSecondsCamel, payload.LimitWindowSecondsSnake); ok {
			windowMins = seconds / 60
		}
	}
	resetsAt, _ := firstJSONInt(payload.ResetsAtCamel, payload.ResetsAtSnake, payload.ResetAtCamel, payload.ResetAtSnake)
	*w = OpenAIAppServerRateLimitWindow{
		UsedPercent:        used,
		WindowDurationMins: windowMins,
		ResetsAt:           resetsAt,
	}
	return nil
}

func (b *OpenAIAppServerRateLimitBucket) UnmarshalJSON(data []byte) error {
	if b == nil {
		return nil
	}
	var payload openAIQuotaRateLimitBucketPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		var value any
		if json.Unmarshal(data, &value) == nil {
			*b = OpenAIAppServerRateLimitBucket{RawValue: value}
			return nil
		}
		return err
	}

	limitID, _ := firstJSONString(payload.LimitIDCamel, payload.LimitIDSnake)
	limitName, _ := firstJSONString(payload.LimitNameCamel, payload.LimitNameSnake)
	used, usedOK := firstJSONFloat(payload.UsedPercentCamel, payload.UsedPercentSnake)
	windowMins, windowOK := firstJSONInt(payload.WindowDurationMinsCamel, payload.WindowDurationMinsSnake)
	resetsAt, _ := firstJSONInt(payload.ResetsAtCamel, payload.ResetsAtSnake)
	reachedType, _ := firstJSONString(payload.RateLimitReachedTypeCamel, payload.RateLimitReachedTypeSnake)

	primary := decodeOpenAIAppServerRateLimitWindow(firstJSONRaw(payload.Primary, payload.PrimaryWindow))
	secondary := decodeOpenAIAppServerRateLimitWindow(firstJSONRaw(payload.Secondary, payload.SecondaryWindow))
	rawFields := decodeRawObjectFields(data)
	if primary != nil || secondary != nil || (usedOK && windowOK) {
		rawFields = nil
	}
	*b = OpenAIAppServerRateLimitBucket{
		LimitID:              limitID,
		LimitName:            limitName,
		UsedPercent:          used,
		WindowDurationMins:   windowMins,
		ResetsAt:             resetsAt,
		Primary:              primary,
		Secondary:            secondary,
		RateLimitReachedType: reachedType,
		RawFields:            rawFields,
	}
	return nil
}

func decodeOpenAIAppServerRateLimitWindow(raw json.RawMessage) *OpenAIAppServerRateLimitWindow {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	var window OpenAIAppServerRateLimitWindow
	if err := json.Unmarshal(raw, &window); err != nil {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}
	recognized := false
	for _, key := range []string{
		"usedPercent", "used_percent", "windowDurationMins", "window_duration_mins",
		"limitWindowSeconds", "limit_window_seconds", "resetsAt", "resets_at", "resetAt", "reset_at",
	} {
		if _, ok := fields[key]; ok {
			recognized = true
			break
		}
	}
	if !recognized {
		return nil
	}
	usedOK := false
	windowOK := false
	for _, key := range []string{"usedPercent", "used_percent"} {
		if _, ok := fields[key]; ok {
			_, usedOK = firstJSONFloat(fields[key])
			if usedOK {
				break
			}
		}
	}
	for _, key := range []string{"windowDurationMins", "window_duration_mins", "limitWindowSeconds", "limit_window_seconds"} {
		if _, ok := fields[key]; ok {
			_, windowOK = firstJSONInt(fields[key])
			if windowOK {
				break
			}
		}
	}
	if !usedOK || !windowOK {
		return nil
	}
	return &window
}

func firstJSONRaw(values ...json.RawMessage) json.RawMessage {
	for _, value := range values {
		trimmed := bytes.TrimSpace(value)
		if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
			return trimmed
		}
	}
	return nil
}

func firstJSONString(values ...json.RawMessage) (string, bool) {
	for _, value := range values {
		trimmed := bytes.TrimSpace(value)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			continue
		}
		var text string
		if err := json.Unmarshal(trimmed, &text); err == nil {
			return strings.TrimSpace(text), true
		}
	}
	return "", false
}

func firstJSONFloat(values ...json.RawMessage) (float64, bool) {
	for _, value := range values {
		trimmed := bytes.TrimSpace(value)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			continue
		}
		var number float64
		if err := json.Unmarshal(trimmed, &number); err == nil {
			if math.IsNaN(number) || math.IsInf(number, 0) {
				continue
			}
			return number, true
		}
		if text, ok := firstJSONString(trimmed); ok {
			if number, err := strconv.ParseFloat(text, 64); err == nil {
				if math.IsNaN(number) || math.IsInf(number, 0) {
					continue
				}
				return number, true
			}
		}
	}
	return 0, false
}

func firstJSONInt(values ...json.RawMessage) (int64, bool) {
	for _, value := range values {
		trimmed := bytes.TrimSpace(value)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			continue
		}
		var number int64
		if err := json.Unmarshal(trimmed, &number); err == nil {
			return number, true
		}
		var decimal float64
		if err := json.Unmarshal(trimmed, &decimal); err == nil {
			if math.IsNaN(decimal) || math.IsInf(decimal, 0) {
				continue
			}
			return int64(decimal), true
		}
		if text, ok := firstJSONString(trimmed); ok {
			if number, err := strconv.ParseInt(text, 10, 64); err == nil {
				return number, true
			}
			if decimal, err := strconv.ParseFloat(text, 64); err == nil {
				if math.IsNaN(decimal) || math.IsInf(decimal, 0) {
					continue
				}
				return int64(decimal), true
			}
		}
	}
	return 0, false
}

// UnmarshalJSON accepts both the current App Server multi-bucket field and
// the legacy snake_case admin representation.  The latter is useful for
// cached fixtures and for rolling upgrades where one node may still return
// the old spelling.
func (u *OpenAIQuotaUsage) UnmarshalJSON(data []byte) error {
	if u == nil {
		return nil
	}
	// App Server replies are JSON-RPC envelopes (`result`), while update
	// notifications carry the same payload under `params`.  Accepting the
	// envelope here makes the parser useful for both the HTTP quota endpoint
	// and recorded app-server messages.
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err == nil {
		for _, key := range []string{"result", "params"} {
			nested := firstJSONRaw(envelope[key])
			if !hasOpenAIRateLimitPayload(nested) {
				continue
			}
			return json.Unmarshal(nested, u)
		}
	}
	type quotaUsageAlias OpenAIQuotaUsage
	var decoded quotaUsageAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*u = OpenAIQuotaUsage(decoded)

	var raw struct {
		RateLimitsByLimitIDCamel json.RawMessage `json:"rateLimitsByLimitId"`
		RateLimitsByLimitIDAlt   json.RawMessage `json:"rateLimitsByLimitID"`
		RateLimitsByLimitIDSnake json.RawMessage `json:"rate_limits_by_limit_id"`
		RateLimitsCamel          json.RawMessage `json:"rateLimits"`
		RateLimitCamel           json.RawMessage `json:"rateLimit"`
		RateLimitSnake           json.RawMessage `json:"rate_limit"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	for _, rawMap := range []json.RawMessage{raw.RateLimitsByLimitIDCamel, raw.RateLimitsByLimitIDAlt, raw.RateLimitsByLimitIDSnake} {
		mergeOpenAIAppServerRateLimitBucketsIntoUsage(u, rawMap)
	}
	// Some app-server adapters nest the keyed map inside the legacy
	// `rateLimits` object. Accept that variant as well; it is semantically the
	// same multi-bucket view and costs no extra network request.
	for _, rawBucket := range []json.RawMessage{raw.RateLimitsCamel, raw.RateLimitCamel, raw.RateLimitSnake} {
		var nested struct {
			RateLimitsByLimitIDCamel json.RawMessage `json:"rateLimitsByLimitId"`
			RateLimitsByLimitIDAlt   json.RawMessage `json:"rateLimitsByLimitID"`
			RateLimitsByLimitIDSnake json.RawMessage `json:"rate_limits_by_limit_id"`
		}
		if err := json.Unmarshal(rawBucket, &nested); err != nil {
			continue
		}
		for _, rawMap := range []json.RawMessage{nested.RateLimitsByLimitIDCamel, nested.RateLimitsByLimitIDAlt, nested.RateLimitsByLimitIDSnake} {
			mergeOpenAIAppServerRateLimitBucketsIntoUsage(u, rawMap)
		}
	}
	if len(u.RateLimitsByLimitID) == 0 {
		u.RateLimitsByLimitID = nil
	}

	// App Server's `rateLimits` is the backward-compatible single-bucket view.
	// Convert it into the legacy model so existing scheduling/analytics code
	// continues to work even when the upstream stops sending `rate_limit`.
	if u.RateLimit == nil {
		if bucket := decodeOpenAIAppServerRateLimitBucket(firstJSONRaw(raw.RateLimitsCamel, raw.RateLimitCamel, raw.RateLimitSnake)); bucket != nil {
			if len(u.RateLimitsByLimitID) == 0 && (bucket.LimitID != "" || bucket.RawFields != nil || bucket.RawValue != nil) {
				if u.RateLimitsByLimitID == nil {
					u.RateLimitsByLimitID = make(map[string]OpenAIAppServerRateLimitBucket)
				}
				key := bucket.LimitID
				if key == "" {
					key = "rateLimits"
				}
				u.RateLimitsByLimitID[key] = *bucket
			}
			u.RateLimit = appServerRateLimitBucketToLegacy(bucket)
		}
	}
	// A few deployments expose a direct `rateLimits` object whose windows are
	// null while the keyed codex bucket is populated.  In that case prefer the
	// useful codex bucket; otherwise keep the direct view.  When only the
	// multi-bucket view exists, prefer codex and then a stable key so old
	// single-window consumers still get useful data.
	if !legacyRateLimitHasWindow(u.RateLimit) && len(u.RateLimitsByLimitID) > 0 {
		if bucket, ok := u.RateLimitsByLimitID["codex"]; ok {
			if candidate := appServerRateLimitBucketToLegacy(&bucket); legacyRateLimitHasWindow(candidate) {
				u.RateLimit = candidate
			}
		} else {
			keys := make([]string, 0, len(u.RateLimitsByLimitID))
			for key := range u.RateLimitsByLimitID {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			bucket := u.RateLimitsByLimitID[keys[0]]
			candidate := appServerRateLimitBucketToLegacy(&bucket)
			if legacyRateLimitHasWindow(candidate) {
				u.RateLimit = candidate
			}
		}
	}
	return nil
}

func mergeOpenAIAppServerRateLimitBucketsIntoUsage(usage *OpenAIQuotaUsage, raw json.RawMessage) {
	if usage == nil {
		return
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return
	}
	if usage.RateLimitsByLimitID == nil {
		usage.RateLimitsByLimitID = make(map[string]OpenAIAppServerRateLimitBucket)
	}
	mergeOpenAIAppServerRateLimitBuckets(usage.RateLimitsByLimitID, trimmed)
}

func legacyRateLimitHasWindow(rateLimit *OpenAIRateLimit) bool {
	return rateLimit != nil && (rateLimit.PrimaryWindow != nil || rateLimit.SecondaryWindow != nil)
}

func hasOpenAIRateLimitPayload(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return false
	}
	for _, key := range []string{
		"rateLimitsByLimitId",
		"rateLimitsByLimitID",
		"rate_limits_by_limit_id",
		"rateLimits",
		"rate_limit",
	} {
		if _, ok := fields[key]; ok {
			return true
		}
	}
	return false
}

func mergeOpenAIAppServerRateLimitBuckets(dst map[string]OpenAIAppServerRateLimitBucket, raw json.RawMessage) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return
	}
	if dst == nil {
		return
	}
	for key, value := range payload {
		bucket := decodeOpenAIAppServerRateLimitBucket(value)
		if bucket == nil {
			continue
		}
		if bucket.LimitID == "" {
			bucket.LimitID = key
		}
		dst[key] = *bucket
	}
}

func decodeOpenAIAppServerRateLimitBucket(raw json.RawMessage) *OpenAIAppServerRateLimitBucket {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	var bucket OpenAIAppServerRateLimitBucket
	if err := json.Unmarshal(raw, &bucket); err != nil {
		if fields := decodeRawObjectFields(raw); fields != nil {
			return &OpenAIAppServerRateLimitBucket{RawFields: fields}
		}
		var value any
		if json.Unmarshal(raw, &value) == nil {
			return &OpenAIAppServerRateLimitBucket{RawValue: value}
		}
		return nil
	}
	return &bucket
}

// decodeRawObjectFields keeps unknown protocol fields in their original
// English spelling for forward-compatible admin display. It is intentionally
// best-effort and never participates in quota decisions.
func decodeRawObjectFields(raw json.RawMessage) map[string]any {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return nil
	}
	decoded := make(map[string]any, len(fields))
	for key, value := range fields {
		if key == "raw_fields" || key == "raw_value" {
			continue
		}
		var item any
		if err := json.Unmarshal(value, &item); err != nil {
			continue
		}
		decoded[key] = item
	}
	if len(decoded) == 0 {
		return nil
	}
	return decoded
}

func appServerRateLimitBucketToLegacy(bucket *OpenAIAppServerRateLimitBucket) *OpenAIRateLimit {
	if bucket == nil {
		return nil
	}
	primary := appServerRateLimitWindowToLegacy(bucket.Primary)
	secondary := appServerRateLimitWindowToLegacy(bucket.Secondary)
	if primary == nil && secondary == nil && (bucket.UsedPercent != 0 || bucket.WindowDurationMins != 0 || bucket.ResetsAt != 0) {
		primary = appServerRateLimitWindowToLegacy(&OpenAIAppServerRateLimitWindow{
			UsedPercent:        bucket.UsedPercent,
			WindowDurationMins: bucket.WindowDurationMins,
			ResetsAt:           bucket.ResetsAt,
		})
	}
	reached := strings.TrimSpace(bucket.RateLimitReachedType) != ""
	return &OpenAIRateLimit{
		LimitID:         bucket.LimitID,
		LimitName:       bucket.LimitName,
		Allowed:         !reached,
		LimitReached:    reached,
		PrimaryWindow:   primary,
		SecondaryWindow: secondary,
	}
}

func appServerRateLimitWindowToLegacy(window *OpenAIAppServerRateLimitWindow) *OpenAIRateLimitWindow {
	if window == nil {
		return nil
	}
	return &OpenAIRateLimitWindow{
		UsedPercent:        window.UsedPercent,
		LimitWindowSeconds: window.WindowDurationMins * 60,
		ResetAt:            window.ResetsAt,
	}
}
