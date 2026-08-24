package admin

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler/dto"
	"github.com/gin-gonic/gin"
)

type openAIQuotaService interface {
	QueryUsage(ctx context.Context, accountID int64) (*service.OpenAIQuotaUsage, error)
	CacheResetCreditsSnapshot(ctx context.Context, accountID int64, credits *service.OpenAIRateLimitResetCredits) error
	ResetCredit(ctx context.Context, accountID int64) (*service.OpenAIQuotaResetResult, error)
}

type openAIQuotaServerUsageQuerier interface {
	QueryUsageWithServerUsage(ctx context.Context, accountID int64) (*service.OpenAIQuotaUsage, error)
}

type openAIQuotaRateLimitSnapshotCacher interface {
	CacheRateLimitSnapshot(ctx context.Context, accountID int64, usage *service.OpenAIQuotaUsage) error
}

type openAIAccountStateRecoverer interface {
	RecoverAccountState(ctx context.Context, accountID int64, options service.AccountRecoveryOptions) (*service.SuccessfulTestRecoveryResult, error)
}

const (
	openAIQuotaResetWarningCacheRefreshFailed    = "reset_credit_cache_refresh_failed"
	openAIQuotaResetWarningAccountRecoveryFailed = "account_state_recovery_failed"
	openAIQuotaResetWarningAccountRefreshFailed  = "account_state_refresh_failed"
	openAIQuotaResetPostProcessTimeout           = 8 * time.Second
)

type openAIQuotaResetResponse struct {
	service.OpenAIQuotaResetResult
	Quota                 *service.OpenAIQuotaUsage `json:"quota,omitempty"`
	Account               *dto.Account              `json:"account,omitempty"`
	CacheRefreshed        bool                      `json:"cache_refreshed"`
	AccountStateRecovered bool                      `json:"account_state_recovered"`
	WarningCode           string                    `json:"warning_code,omitempty"`
}

type openAIQuotaRefreshResponse struct {
	service.OpenAIQuotaUsage
	CachePersisted             bool `json:"cache_persisted"`
	RateLimitSnapshotPersisted bool `json:"rate_limit_snapshot_persisted"`
}

// OpenAIQuotaUsage implements json.Unmarshaler for the upstream protocol.
// Because the quota refresh response embeds that type, encoding/json would
// otherwise promote its UnmarshalJSON method and silently drop the sibling
// persistence flags when clients/tests decode the response envelope.
func (r *openAIQuotaRefreshResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return nil
	}
	var usage service.OpenAIQuotaUsage
	if err := json.Unmarshal(data, &usage); err != nil {
		return err
	}
	var meta struct {
		CachePersisted             bool `json:"cache_persisted"`
		RateLimitSnapshotPersisted bool `json:"rate_limit_snapshot_persisted"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return err
	}
	r.OpenAIQuotaUsage = usage
	r.CachePersisted = meta.CachePersisted
	r.RateLimitSnapshotPersisted = meta.RateLimitSnapshotPersisted
	return nil
}

func openAIQuotaResetPostProcessContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	return context.WithTimeout(base, openAIQuotaResetPostProcessTimeout)
}

func (h *OpenAIOAuthHandler) queryOpenAIQuotaUsage(ctx context.Context, accountID int64) (*service.OpenAIQuotaUsage, error) {
	if detailed, ok := h.quotaService.(openAIQuotaServerUsageQuerier); ok {
		return detailed.QueryUsageWithServerUsage(ctx, accountID)
	}
	return h.quotaService.QueryUsage(ctx, accountID)
}

// QueryQuota queries the rate-limit / quota usage without mutating account state.
// GET /api/v1/admin/openai/accounts/:id/quota
func (h *OpenAIOAuthHandler) QueryQuota(c *gin.Context) {
	accountID, ok := openAIQuotaAccountID(c)
	if !ok {
		return
	}
	if h.quotaService == nil {
		response.BadRequest(c, "openai quota service is not enabled")
		return
	}
	usage, err := h.queryOpenAIQuotaUsage(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, usage)
}

// RefreshQuota queries the latest quota and persists reset-credit expiration data.
// POST /api/v1/admin/openai/accounts/:id/quota/refresh
func (h *OpenAIOAuthHandler) RefreshQuota(c *gin.Context) {
	accountID, ok := openAIQuotaAccountID(c)
	if !ok {
		return
	}
	if h.quotaService == nil {
		response.BadRequest(c, "openai quota service is not enabled")
		return
	}

	usage, err := h.queryOpenAIQuotaUsage(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if usage == nil {
		response.Error(c, http.StatusInternalServerError, "openai quota query returned an empty result")
		return
	}

	result := openAIQuotaRefreshResponse{OpenAIQuotaUsage: *usage}
	if err := h.quotaService.CacheResetCreditsSnapshot(c.Request.Context(), accountID, usage.RateLimitResetCredits); err != nil {
		slog.Warn("openai_quota_reset_credit_cache_persist_failed", "account_id", accountID, "error", err)
	} else {
		result.CachePersisted = true
	}
	if cacher, ok := h.quotaService.(openAIQuotaRateLimitSnapshotCacher); ok {
		if err := cacher.CacheRateLimitSnapshot(c.Request.Context(), accountID, usage); err != nil {
			slog.Warn("openai_quota_rate_limit_snapshot_persist_failed", "account_id", accountID, "error", err)
		} else {
			result.RateLimitSnapshotPersisted = true
		}
	}
	response.Success(c, result)
}

// ResetQuota consumes a non-refundable reset credit and repairs local account state.
// POST /api/v1/admin/openai/accounts/:id/reset-quota
func (h *OpenAIOAuthHandler) ResetQuota(c *gin.Context) {
	accountID, ok := openAIQuotaAccountID(c)
	if !ok {
		return
	}
	if h.quotaService == nil {
		response.BadRequest(c, "openai quota service is not enabled")
		return
	}

	result, err := h.quotaService.ResetCredit(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if result == nil {
		response.Error(c, http.StatusInternalServerError, "openai quota reset returned an empty result")
		return
	}

	resetResponse := openAIQuotaResetResponse{OpenAIQuotaResetResult: *result}
	postCtx, cancelPost := openAIQuotaResetPostProcessContext(c.Request.Context())
	defer cancelPost()

	if h.rateLimitService == nil {
		resetResponse.WarningCode = openAIQuotaResetWarningAccountRecoveryFailed
		response.Success(c, resetResponse)
		return
	}
	if _, err := h.rateLimitService.RecoverAccountState(postCtx, accountID, service.AccountRecoveryOptions{
		InvalidateToken: true,
	}); err != nil {
		slog.Warn("openai_quota_reset_account_recovery_failed", "account_id", accountID, "error", err)
		resetResponse.WarningCode = openAIQuotaResetWarningAccountRecoveryFailed
		response.Success(c, resetResponse)
		return
	}
	resetResponse.AccountStateRecovered = true

	// Reset post-processing only needs fresh quota/reset-credit state; avoid an
	// additional profile counter request on this latency-sensitive path.
	usage, usageErr := h.quotaService.QueryUsage(postCtx, accountID)
	if usageErr != nil || usage == nil {
		slog.Warn("openai_quota_reset_cache_refresh_failed", "account_id", accountID, "error", usageErr)
		resetResponse.WarningCode = openAIQuotaResetWarningCacheRefreshFailed
	} else {
		creditsErr := h.quotaService.CacheResetCreditsSnapshot(postCtx, accountID, usage.RateLimitResetCredits)
		var snapshotErr error
		if cacher, ok := h.quotaService.(openAIQuotaRateLimitSnapshotCacher); ok {
			snapshotErr = cacher.CacheRateLimitSnapshot(postCtx, accountID, usage)
		}
		if creditsErr != nil {
			slog.Warn("openai_quota_reset_cache_refresh_failed", "account_id", accountID, "error", creditsErr)
		}
		if snapshotErr != nil {
			slog.Warn("openai_quota_rate_limit_snapshot_persist_failed", "account_id", accountID, "error", snapshotErr)
		}
		if creditsErr != nil {
			resetResponse.WarningCode = openAIQuotaResetWarningCacheRefreshFailed
		} else if snapshotErr != nil {
			resetResponse.WarningCode = openAIQuotaResetWarningCacheRefreshFailed
		} else {
			resetResponse.Quota = usage
			resetResponse.CacheRefreshed = true
		}
	}

	account, err := h.adminService.GetAccount(postCtx, accountID)
	if err != nil {
		slog.Warn("openai_quota_reset_account_refresh_failed", "account_id", accountID, "error", err)
		if resetResponse.WarningCode == "" {
			resetResponse.WarningCode = openAIQuotaResetWarningAccountRefreshFailed
		}
		response.Success(c, resetResponse)
		return
	}
	resetResponse.Account = dto.AccountFromService(account)
	response.Success(c, resetResponse)
}

func openAIQuotaAccountID(c *gin.Context) (int64, bool) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return 0, false
	}
	return accountID, true
}
