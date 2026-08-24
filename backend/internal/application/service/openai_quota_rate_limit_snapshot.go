package service

import (
	"context"
	"encoding/json"
	"net/http"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

// openAIQuotaRateLimitSnapshotKey is a managed account-extra field.  It keeps
// only the rate-limit portion of the last explicit App Server query so the
// admin list can render the known percentages before another upstream call.
const openAIQuotaRateLimitSnapshotKey = "codex_rate_limit_snapshot"

const maxOpenAIQuotaRateLimitSnapshotBytes = 64 * 1024

// OpenAIRateLimitSnapshot is deliberately narrower than OpenAIQuotaUsage:
// server token activity and reset-credit details have their own lifecycles.
// Keeping the bucket map separate also prevents a refresh from persisting
// credentials or unrelated upstream response fields.
type OpenAIRateLimitSnapshot struct {
	FetchedAt           int64                                     `json:"fetched_at"`
	RateLimit           *OpenAIRateLimit                          `json:"rate_limit,omitempty"`
	RateLimitsByLimitID map[string]OpenAIAppServerRateLimitBucket `json:"rate_limits_by_limit_id,omitempty"`
}

func rateLimitSnapshotFromUsage(usage *OpenAIQuotaUsage) *OpenAIRateLimitSnapshot {
	if usage == nil {
		return nil
	}
	if len(usage.RateLimitsByLimitID) == 0 && usage.RateLimit == nil {
		return nil
	}
	return &OpenAIRateLimitSnapshot{
		FetchedAt:           usage.FetchedAt,
		RateLimit:           usage.RateLimit,
		RateLimitsByLimitID: usage.RateLimitsByLimitID,
	}
}

// CacheRateLimitSnapshot persists only the bucket data from an explicit
// admin quota query. A nil snapshot clears the previous value, which avoids
// displaying an obsolete bucket after an upstream response changes shape.
func (s *OpenAIQuotaService) CacheRateLimitSnapshot(ctx context.Context, accountID int64, usage *OpenAIQuotaUsage) error {
	if s == nil || s.accountRepo == nil {
		return infraerrors.New(
			http.StatusInternalServerError,
			"OPENAI_QUOTA_NOT_CONFIGURED",
			"openai quota account repository is not configured",
		)
	}

	snapshot := rateLimitSnapshotFromUsage(usage)
	if snapshot != nil {
		payload, err := json.Marshal(snapshot)
		if err != nil {
			return infraerrors.New(http.StatusInternalServerError, "OPENAI_QUOTA_SNAPSHOT_SERIALIZE_FAILED", "failed to serialize rate-limit snapshot").WithCause(err)
		}
		if len(payload) > maxOpenAIQuotaRateLimitSnapshotBytes {
			return infraerrors.New(http.StatusBadGateway, "OPENAI_QUOTA_SNAPSHOT_TOO_LARGE", "rate-limit snapshot exceeds the persistence limit")
		}
	}

	var value any
	if snapshot != nil {
		value = snapshot
	}
	if err := s.accountRepo.UpdateExtra(ctx, accountID, map[string]any{
		openAIQuotaRateLimitSnapshotKey: value,
	}); err != nil {
		return infraerrors.New(
			http.StatusInternalServerError,
			"OPENAI_QUOTA_SNAPSHOT_CACHE_WRITE_FAILED",
			"failed to cache rate-limit snapshot",
		).WithCause(err)
	}
	return nil
}
