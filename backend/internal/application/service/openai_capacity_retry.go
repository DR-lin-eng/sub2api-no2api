package service

import (
	"bytes"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

const openAIRequestScopedRetryMaxDelay = 8 * time.Second

// Only capacity failures call this parser. Successful requests and ordinary
// errors keep their existing retry policy and incur no hint parsing cost.
func openAIRequestScopedRetryDelay(headers http.Header, body []byte) time.Duration {
	for _, key := range []string{"Retry-After", "Retry-After-Ms"} {
		unit := time.Second
		if key == "Retry-After-Ms" {
			unit = time.Millisecond
		}
		if delay := parseOpenAICapacityRetryDelay(headers.Get(key), unit); delay > 0 {
			return delay
		}
	}
	// Most capacity errors have no hint. Avoid four JSON path scans unless a
	// literal key or an escaped Unicode key could carry one.
	if !bytes.Contains(body, []byte("retry-after")) && !bytes.Contains(body, []byte(`\u`)) {
		return 0
	}
	for _, path := range []string{
		"error.headers.retry-after", "response.error.headers.retry-after",
		"error.headers.retry-after-ms", "response.error.headers.retry-after-ms",
	} {
		unit := time.Second
		if strings.HasSuffix(path, "-ms") {
			unit = time.Millisecond
		}
		if delay := parseOpenAICapacityRetryDelay(gjson.GetBytes(body, path).String(), unit); delay > 0 {
			return delay
		}
	}
	return 0
}

func parseOpenAICapacityRetryDelay(raw string, unit time.Duration) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if value, err := strconv.ParseFloat(raw, 64); err == nil {
		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return 0
		}
		// Clamp before converting: very large finite hints must not overflow
		// time.Duration into a negative value and trigger an immediate retry.
		if value >= float64(openAIRequestScopedRetryMaxDelay)/float64(unit) {
			return openAIRequestScopedRetryMaxDelay
		}
		return time.Duration(value * float64(unit))
	}
	if unit == time.Second {
		if resetAt, err := http.ParseTime(raw); err == nil {
			if delay := time.Until(resetAt); delay > 0 {
				return min(delay, openAIRequestScopedRetryMaxDelay)
			}
		}
	}
	return 0
}
