package admin

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler/dto"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
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
	openAIQuotaRefreshBatchConcurrency           = 4
	openAIQuotaRefreshBatchMaxAccounts           = 20
)

var errOpenAIQuotaEmptyResult = errors.New("openai quota query returned an empty result")

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

func (h *OpenAIOAuthHandler) refreshOpenAIQuota(ctx context.Context, accountID int64) (*openAIQuotaRefreshResponse, error) {
	usage, err := h.queryOpenAIQuotaUsage(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if usage == nil {
		return nil, errOpenAIQuotaEmptyResult
	}

	result := &openAIQuotaRefreshResponse{OpenAIQuotaUsage: *usage}
	if err := h.quotaService.CacheResetCreditsSnapshot(ctx, accountID, usage.RateLimitResetCredits); err != nil {
		slog.Warn("openai_quota_reset_credit_cache_persist_failed", "account_id", accountID, "error", err)
	} else {
		result.CachePersisted = true
	}
	if cacher, ok := h.quotaService.(openAIQuotaRateLimitSnapshotCacher); ok {
		if err := cacher.CacheRateLimitSnapshot(ctx, accountID, usage); err != nil {
			slog.Warn("openai_quota_rate_limit_snapshot_persist_failed", "account_id", accountID, "error", err)
		} else {
			result.RateLimitSnapshotPersisted = true
		}
	}
	return result, nil
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

	result, err := h.refreshOpenAIQuota(c.Request.Context(), accountID)
	if err != nil {
		if errors.Is(err, errOpenAIQuotaEmptyResult) {
			response.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

type openAIQuotaRefreshBatchRequest struct {
	AccountIDs []int64 `json:"account_ids"`
}

type openAIQuotaRefreshBatchResponse struct {
	Results           map[int64]*openAIQuotaRefreshResponse `json:"results"`
	Errors            map[int64]string                      `json:"errors"`
	SkippedAccountIDs []int64                               `json:"skipped_account_ids"`
}

// RefreshQuotaBatch actively queries selected OpenAI OAuth accounts with bounded concurrency.
// POST /api/v1/admin/openai/accounts/quota/refresh/batch
func (h *OpenAIOAuthHandler) RefreshQuotaBatch(c *gin.Context) {
	var req openAIQuotaRefreshBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	accountIDs := normalizeInt64IDList(req.AccountIDs)
	if len(accountIDs) == 0 {
		response.BadRequest(c, "account_ids is required")
		return
	}
	if len(accountIDs) > openAIQuotaRefreshBatchMaxAccounts {
		response.BadRequest(c, "account_ids must contain at most 20 unique IDs")
		return
	}
	if h.quotaService == nil {
		response.BadRequest(c, "openai quota service is not enabled")
		return
	}
	if h.adminService == nil {
		response.Error(c, http.StatusServiceUnavailable, "admin account service is not available")
		return
	}

	accounts, err := h.adminService.GetAccountsByIDs(c.Request.Context(), accountIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	accountsByID := make(map[int64]*service.Account, len(accounts))
	for _, account := range accounts {
		if account != nil {
			accountsByID[account.ID] = account
		}
	}

	result := openAIQuotaRefreshBatchResponse{
		Results: make(map[int64]*openAIQuotaRefreshResponse),
		Errors:  make(map[int64]string),
	}
	eligibleIDs := make([]int64, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		account := accountsByID[accountID]
		if account == nil {
			result.Errors[accountID] = service.ErrAccountNotFound.Error()
			continue
		}
		if !account.IsOpenAIOAuth() {
			result.SkippedAccountIDs = append(result.SkippedAccountIDs, accountID)
			continue
		}
		eligibleIDs = append(eligibleIDs, accountID)
	}

	var mu sync.Mutex
	g, gctx := errgroup.WithContext(c.Request.Context())
	g.SetLimit(openAIQuotaRefreshBatchConcurrency)
	for _, accountID := range eligibleIDs {
		accountID := accountID
		g.Go(func() error {
			quota, quotaErr := h.refreshOpenAIQuota(gctx, accountID)
			mu.Lock()
			defer mu.Unlock()
			if quotaErr != nil {
				result.Errors[accountID] = quotaErr.Error()
				return nil
			}
			result.Results[accountID] = quota
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		response.ErrorFrom(c, err)
		return
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
