package handler

import (
	"errors"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	middleware2 "github.com/DR-lin-eng/sub2api-no2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
)

const (
	metadataFallbackWaitTimeout = 30 * time.Second
	metadataFallbackMaxWaiting  = 100
)

func metadataPriorityAdmissionEnabled(c *gin.Context, helper *ConcurrencyHelper) bool {
	if helper == nil || helper.concurrencyService == nil {
		return false
	}
	_, _, enabled := helper.priorityAdmissionRequestSnapshot(c)
	return enabled
}

func acquireMetadataUserSlot(
	c *gin.Context,
	helper *ConcurrencyHelper,
	subject middleware2.AuthSubject,
) (func(), error) {
	helper.SetPriorityAdmissionPendingBytes(c, 0)
	streamStarted := false
	release, err := helper.AcquireUserSlotWithWait(c, subject.UserID, subject.Concurrency, false, &streamStarted)
	if err != nil {
		return nil, err
	}
	return wrapReleaseOnDone(c.Request.Context(), release), nil
}

func acquireMetadataAccountSlot(
	c *gin.Context,
	helper *ConcurrencyHelper,
	cfg *config.Config,
	account *service.Account,
	revalidator accountWaitRevalidator,
	groupID *int64,
) (func(), error) {
	timeout := metadataFallbackWaitTimeout
	maxWaiting := metadataFallbackMaxWaiting
	if cfg != nil {
		if configured := cfg.Gateway.Scheduling.FallbackWaitTimeout; configured > 0 {
			timeout = configured
		}
		if configured := cfg.Gateway.Scheduling.FallbackMaxWaiting; configured > 0 {
			maxWaiting = configured
		}
	}

	selection := &service.AccountSelectionResult{
		Account: account,
		WaitPlan: &service.AccountWaitPlan{
			AccountID:      account.ID,
			MaxConcurrency: account.Concurrency,
			Timeout:        timeout,
			MaxWaiting:     maxWaiting,
		},
	}
	streamStarted := false
	return acquireAccountSelectionSlot(c, helper, selection, false, &streamStarted, nil, revalidator, groupID, "")
}

func (h *GatewayHandler) selectGeminiMetadataAccount(
	c *gin.Context,
	groupID *int64,
	priorityAdmissionEnabled bool,
) (*service.Account, func(), error) {
	var excludedAccountIDs map[int64]struct{}
	for {
		account, err := h.geminiCompatService.SelectAccountForAIStudioEndpointsWithExclusions(
			c.Request.Context(),
			groupID,
			excludedAccountIDs,
		)
		if err != nil || !priorityAdmissionEnabled {
			return account, nil, err
		}

		var revalidator accountWaitRevalidator
		if h.gatewayService != nil {
			revalidator = h.gatewayService
		}
		release, err := acquireMetadataAccountSlot(c, h.concurrencyHelper, h.cfg, account, revalidator, groupID)
		if errors.Is(err, service.ErrAccountSchedulingChanged) {
			addFailedAccountID(&excludedAccountIDs, account.ID)
			continue
		}
		return account, release, err
	}
}
