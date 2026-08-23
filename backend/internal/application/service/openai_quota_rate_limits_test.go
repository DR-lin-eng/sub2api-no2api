package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIQuotaUsageUnmarshalAppServerRateLimitsByLimitID(t *testing.T) {
	body := []byte(`{
      "rateLimits": {
        "limitId": "codex",
        "limitName": null,
        "primary": {"usedPercent": 25, "windowDurationMins": 15, "resetsAt": 1730947200},
        "secondary": null,
        "rateLimitReachedType": null
      },
      "rateLimitsByLimitId": {
        "codex": {
          "limitId": "codex",
          "limitName": null,
          "primary": {"usedPercent": 25, "windowDurationMins": 15, "resetsAt": 1730947200},
          "secondary": null,
          "rateLimitReachedType": null
        },
        "codex_other": {
          "limitId": "codex_other",
          "limitName": "codex_other",
          "primary": {"usedPercent": 42, "windowDurationMins": 60, "resetsAt": 1730950800},
          "secondary": null,
          "rateLimitReachedType": null
        }
      }
    }`)

	var usage OpenAIQuotaUsage
	require.NoError(t, json.Unmarshal(body, &usage))
	require.Len(t, usage.RateLimitsByLimitID, 2)

	codex := usage.RateLimitsByLimitID["codex"]
	require.Equal(t, "codex", codex.LimitID)
	require.NotNil(t, codex.Primary)
	require.Equal(t, float64(25), codex.Primary.UsedPercent)
	require.Equal(t, int64(15), codex.Primary.WindowDurationMins)
	require.Equal(t, int64(1730947200), codex.Primary.ResetsAt)

	other := usage.RateLimitsByLimitID["codex_other"]
	require.Equal(t, "codex_other", other.LimitID)
	require.Equal(t, "codex_other", other.LimitName)
	require.NotNil(t, other.Primary)
	require.Equal(t, float64(42), other.Primary.UsedPercent)
	require.Equal(t, int64(60), other.Primary.WindowDurationMins)
	require.Equal(t, int64(1730950800), other.Primary.ResetsAt)

	// The new protocol still feeds old single-bucket consumers.
	require.NotNil(t, usage.RateLimit)
	require.NotNil(t, usage.RateLimit.PrimaryWindow)
	require.Equal(t, float64(25), usage.RateLimit.PrimaryWindow.UsedPercent)
	require.Equal(t, int64(900), usage.RateLimit.PrimaryWindow.LimitWindowSeconds)
}

func TestOpenAIQuotaUsageUnmarshalRateLimitsByLimitIDSnakeAndFlat(t *testing.T) {
	body := []byte(`{
      "rate_limits_by_limit_id": {
        "codex_other": {
          "limit_id": "codex_other",
          "limit_name": "Other",
          "used_percent": "17.5",
          "window_duration_mins": "30",
          "resets_at": "1730950800"
        }
      }
    }`)

	var usage OpenAIQuotaUsage
	require.NoError(t, json.Unmarshal(body, &usage))
	bucket, ok := usage.RateLimitsByLimitID["codex_other"]
	require.True(t, ok)
	require.Equal(t, "Other", bucket.LimitName)
	require.InDelta(t, 17.5, bucket.UsedPercent, 0.0001)
	require.Equal(t, int64(30), bucket.WindowDurationMins)
	require.Equal(t, int64(1730950800), bucket.ResetsAt)
	require.NotNil(t, usage.RateLimit)
	require.NotNil(t, usage.RateLimit.PrimaryWindow)
}

func TestOpenAIQuotaUsageLegacyRateLimitRemainsUnchanged(t *testing.T) {
	body := []byte(`{
      "rate_limit": {
        "allowed": true,
        "limit_reached": false,
        "primary_window": {
          "used_percent": 12,
          "limit_window_seconds": 18000,
          "reset_after_seconds": 3600,
          "reset_at": 1730947200
        }
      }
    }`)

	var usage OpenAIQuotaUsage
	require.NoError(t, json.Unmarshal(body, &usage))
	require.Nil(t, usage.RateLimitsByLimitID)
	require.NotNil(t, usage.RateLimit)
	require.Equal(t, float64(12), usage.RateLimit.PrimaryWindow.UsedPercent)
	require.Equal(t, int64(18000), usage.RateLimit.PrimaryWindow.LimitWindowSeconds)
}

func TestOpenAIQuotaUsageUnmarshalAppServerEnvelope(t *testing.T) {
	body := []byte(`{
      "id": 6,
      "result": {
        "rateLimitsByLimitId": {
          "codex_other": {
            "limitId": "codex_other",
            "primary": {"usedPercent": 9, "windowDurationMins": 60, "resetsAt": 1730950800}
          }
        }
      }
    }`)
	var usage OpenAIQuotaUsage
	require.NoError(t, json.Unmarshal(body, &usage))
	require.Equal(t, float64(9), usage.RateLimitsByLimitID["codex_other"].Primary.UsedPercent)

	update := []byte(`{
      "method": "account/rateLimits/updated",
      "params": {
        "rateLimits": {
          "limitId": "codex",
          "primary": {"usedPercent": 31, "windowDurationMins": 15, "resetsAt": 1730948100}
        }
      }
    }`)
	var notification OpenAIQuotaUsage
	require.NoError(t, json.Unmarshal(update, &notification))
	require.NotNil(t, notification.RateLimit)
	require.Equal(t, float64(31), notification.RateLimit.PrimaryWindow.UsedPercent)
}

func TestOpenAIQuotaUsagePrefersPopulatedCodexBucketOverEmptyDirectView(t *testing.T) {
	body := []byte(`{
      "rateLimits": {"primary": null, "secondary": null},
      "rateLimitsByLimitId": {
        "codex": {
          "primary": {"usedPercent": 18, "windowDurationMins": 300, "resetsAt": 1730947200}
        }
      }
    }`)
	var usage OpenAIQuotaUsage
	require.NoError(t, json.Unmarshal(body, &usage))
	require.NotNil(t, usage.RateLimit)
	require.NotNil(t, usage.RateLimit.PrimaryWindow)
	require.Equal(t, float64(18), usage.RateLimit.PrimaryWindow.UsedPercent)
}

func TestOpenAIQuotaUsageUnmarshalNestedRateLimitsByLimitID(t *testing.T) {
	body := []byte(`{
      "rateLimits": {
        "primary": null,
        "rateLimitsByLimitId": {
          "codex_other": {
            "limitId": "codex_other",
            "primary": {"usedPercent": 7, "windowDurationMins": 60, "resetsAt": 1730950800}
          }
        }
      }
    }`)
	var usage OpenAIQuotaUsage
	require.NoError(t, json.Unmarshal(body, &usage))
	require.Equal(t, float64(7), usage.RateLimitsByLimitID["codex_other"].Primary.UsedPercent)
}

func TestOpenAIRateLimitUnmarshalAppServerShape(t *testing.T) {
	var rateLimit OpenAIRateLimit
	require.NoError(t, json.Unmarshal([]byte(`{
      "primary": {"usedPercent": 4, "windowDurationMins": 300, "resetsAt": 1730947200},
      "secondary": null,
      "rateLimitReachedType": null
    }`), &rateLimit))
	require.True(t, rateLimit.Allowed)
	require.NotNil(t, rateLimit.PrimaryWindow)
	require.Equal(t, float64(4), rateLimit.PrimaryWindow.UsedPercent)
	require.Equal(t, int64(18000), rateLimit.PrimaryWindow.LimitWindowSeconds)
}

func TestBuildCodexSparkWindowExtraUpdatesReadsAppServerBucket(t *testing.T) {
	usage := &OpenAIQuotaUsage{
		RateLimitsByLimitID: map[string]OpenAIAppServerRateLimitBucket{
			"codex_bengalfox": {
				LimitID: "codex_bengalfox",
				Primary: &OpenAIAppServerRateLimitWindow{
					UsedPercent:        0.35,
					WindowDurationMins: 300,
				},
				Secondary: &OpenAIAppServerRateLimitWindow{
					UsedPercent:        0.12,
					WindowDurationMins: 10080,
				},
			},
		},
	}
	updates := buildCodexSparkWindowExtraUpdates(usage, time.Unix(1730947200, 0))
	require.InDelta(t, 0.35, updates["codex_5h_used_percent"], 1e-9)
	require.InDelta(t, 0.12, updates["codex_7d_used_percent"], 1e-9)
}
