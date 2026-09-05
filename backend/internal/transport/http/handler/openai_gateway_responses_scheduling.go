package handler

import (
	"errors"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/application/service"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type accountSlotAcquireOutcome uint8

const (
	accountSlotAcquireFailed accountSlotAcquireOutcome = iota
	accountSlotAcquireReady
	accountSlotAcquireReschedule
	accountSlotAcquireProfitVeto
)

func (h *OpenAIGatewayHandler) acquireResponsesUserSlot(
	c *gin.Context,
	userID int64,
	userConcurrency int,
	reqStream bool,
	streamStarted *bool,
	reqLog *zap.Logger,
) (func(), bool) {
	ctx := c.Request.Context()
	userReleaseFunc, err := h.concurrencyHelper.AcquireUserSlotWithWait(c, userID, userConcurrency, reqStream, streamStarted)
	if err != nil {
		if shouldLogConcurrencyAcquireError(err) {
			reqLog.Warn("openai.user_slot_acquire_failed", zap.Error(err))
		}
		h.handleConcurrencyError(c, err, "user", *streamStarted)
		return nil, false
	}
	return wrapReleaseOnDone(ctx, userReleaseFunc), true
}

func (h *OpenAIGatewayHandler) handleOpenAIProfitVetoExhausted(c *gin.Context, streamStarted bool, reqLog *zap.Logger, vetoCount int) {
	reqLog.Warn("openai.profit_veto_attempts_exhausted", zap.Int("profit_veto_count", vetoCount))
	markOpsRoutingCapacityLimited(c)
	h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", profitVetoExhaustedMessage, streamStarted)
}

func (h *OpenAIGatewayHandler) acquireResponsesAccountSlot(
	c *gin.Context,
	groupID *int64,
	sessionHash string,
	selection *service.AccountSelectionResult,
	reqStream bool,
	streamStarted *bool,
	reqLog *zap.Logger,
) (func(), accountSlotAcquireOutcome) {
	if selection == nil || selection.Account == nil {
		markOpsRoutingCapacityLimited(c)
		h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts", *streamStarted)
		return nil, accountSlotAcquireFailed
	}

	ctx := service.ContextWithSelectionProfitGate(c.Request.Context(), selection)
	account := selection.Account
	if selection.Acquired {
		latest, err := h.gatewayService.RefreshCodexContinuationSchedulingAccount(ctx, account)
		if err != nil {
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			if errors.Is(err, service.ErrAccountSchedulingChanged) {
				reqLog.Info("openai.codex_continuation_account_reschedule", zap.Int64("account_id", account.ID), zap.Error(err))
				return nil, accountSlotAcquireReschedule
			}
			reqLog.Warn("openai.codex_continuation_account_refresh_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "Account scheduling state is unavailable", *streamStarted)
			return nil, accountSlotAcquireFailed
		}
		latest, vetoed, reason := h.gatewayService.ProfitControlVetoLatest(ctx, latest)
		if vetoed {
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			reqLog.Debug("openai.account_slot_profit_vetoed", zap.Int64("account_id", account.ID), zap.String("reason", reason))
			return nil, accountSlotAcquireProfitVeto
		}
		account = latest
		selection.Account = latest
		if selection.ProfitGateActive() {
			if err := h.gatewayService.BindStickySessionAfterProfitAdmission(ctx, groupID, sessionHash, account.ID); err != nil {
				reqLog.Warn("openai.bind_sticky_session_after_profit_admission_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			}
		}
		return wrapReleaseOnDone(ctx, selection.ReleaseFunc), accountSlotAcquireReady
	}
	if selection.WaitPlan == nil {
		markOpsRoutingCapacityLimited(c)
		h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts", *streamStarted)
		return nil, accountSlotAcquireFailed
	}

	var accountReleaseFunc func()
	if h.concurrencyHelper.concurrencyService.PriorityAdmissionEnabledForRequest(c.Request.Context()) {
		var err error
		accountReleaseFunc, err = h.concurrencyHelper.AcquireAccountSlotWithPriorityWaitTimeout(
			c,
			account.ID,
			selection.WaitPlan.MaxConcurrency,
			selection.WaitPlan.MaxWaiting,
			selection.WaitPlan.Timeout,
			priorityAdmissionPendingBytes(c),
			reqStream,
			streamStarted,
		)
		if err != nil {
			if shouldLogConcurrencyAcquireError(err) {
				reqLog.Warn("openai.account_slot_priority_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			}
			h.handleConcurrencyError(c, err, "account", *streamStarted)
			return nil, accountSlotAcquireFailed
		}
	} else {
		fastReleaseFunc, fastAcquired, err := h.concurrencyHelper.TryAcquireAccountSlot(
			ctx,
			account.ID,
			selection.WaitPlan.MaxConcurrency,
		)
		if err != nil {
			reqLog.Warn("openai.account_slot_quick_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			h.handleConcurrencyError(c, err, "account", *streamStarted)
			return nil, accountSlotAcquireFailed
		}
		if fastAcquired {
			accountReleaseFunc = fastReleaseFunc
		} else {
			canWait, waitErr := h.concurrencyHelper.IncrementAccountWaitCount(ctx, account.ID, selection.WaitPlan.MaxWaiting)
			if waitErr != nil {
				reqLog.Warn("openai.account_wait_counter_increment_failed", zap.Int64("account_id", account.ID), zap.Error(waitErr))
			} else if !canWait {
				reqLog.Info("openai.account_wait_queue_full",
					zap.Int64("account_id", account.ID),
					zap.Int("max_waiting", selection.WaitPlan.MaxWaiting),
				)
				h.handleStreamingAwareError(c, http.StatusTooManyRequests, "rate_limit_error", "Too many pending requests, please retry later", *streamStarted)
				return nil, accountSlotAcquireFailed
			}

			accountWaitCounted := waitErr == nil && canWait
			releaseWait := func() {
				if accountWaitCounted {
					h.concurrencyHelper.DecrementAccountWaitCount(ctx, account.ID)
					accountWaitCounted = false
				}
			}
			defer releaseWait()

			accountReleaseFunc, err = h.concurrencyHelper.AcquireAccountSlotWithWaitTimeout(
				c,
				account.ID,
				selection.WaitPlan.MaxConcurrency,
				selection.WaitPlan.Timeout,
				reqStream,
				streamStarted,
			)
			if err != nil {
				reqLog.Warn("openai.account_slot_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				h.handleConcurrencyError(c, err, "account", *streamStarted)
				return nil, accountSlotAcquireFailed
			}
			releaseWait()
		}
	}

	if err := h.gatewayService.EnsureAccountSchedulableAfterWait(ctx, groupID, sessionHash, account.ID); err != nil {
		if accountReleaseFunc != nil {
			accountReleaseFunc()
		}
		if errors.Is(err, service.ErrAccountSchedulingChanged) {
			reqLog.Info("openai.account_reschedule_after_wait", zap.Int64("account_id", account.ID))
			return nil, accountSlotAcquireReschedule
		}
		reqLog.Warn("openai.account_recheck_after_wait_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "Account scheduling state is unavailable", *streamStarted)
		return nil, accountSlotAcquireFailed
	}
	latest, refreshErr := h.gatewayService.RefreshCodexContinuationSchedulingAccount(ctx, account)
	if refreshErr != nil {
		if accountReleaseFunc != nil {
			accountReleaseFunc()
		}
		if errors.Is(refreshErr, service.ErrAccountSchedulingChanged) {
			reqLog.Info("openai.codex_continuation_account_reschedule_after_wait", zap.Int64("account_id", account.ID), zap.Error(refreshErr))
			return nil, accountSlotAcquireReschedule
		}
		reqLog.Warn("openai.codex_continuation_account_refresh_after_wait_failed", zap.Int64("account_id", account.ID), zap.Error(refreshErr))
		h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "Account scheduling state is unavailable", *streamStarted)
		return nil, accountSlotAcquireFailed
	}
	latest, vetoed, reason := h.gatewayService.ProfitControlVetoLatest(ctx, latest)
	if vetoed {
		if accountReleaseFunc != nil {
			accountReleaseFunc()
		}
		reqLog.Debug("openai.account_slot_profit_vetoed", zap.Int64("account_id", account.ID), zap.String("reason", reason))
		return nil, accountSlotAcquireProfitVeto
	}
	account = latest
	selection.Account = latest
	if err := h.gatewayService.BindStickySessionAfterProfitAdmission(ctx, groupID, sessionHash, account.ID); err != nil {
		reqLog.Warn("openai.bind_sticky_session_after_profit_admission_failed", zap.Int64("account_id", account.ID), zap.Error(err))
	}
	return wrapReleaseOnDone(ctx, accountReleaseFunc), accountSlotAcquireReady
}
