package admin

import (
	"context"
	"strconv"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/gin-gonic/gin"
)

type testCPACapacityRequest struct {
	UseAccountBaseURL          bool   `json:"use_account_base_url"`
	BaseURL                    string `json:"base_url"`
	ManagementURL              string `json:"management_url"`
	ManagementPassword         string `json:"management_password"`
	ConcurrencyPerCredential   *int   `json:"concurrency_per_credential"`
	ExcludeAbnormalCredentials *bool  `json:"exclude_abnormal_credentials"`
}

// TestCPAConnection validates unsaved CPA settings without mutating account
// credentials or the scheduler snapshot cache.
func (h *AccountHandler) TestCPAConnection(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	var req testCPACapacityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	result, err := h.concurrencyService.TestCPACapacity(c.Request.Context(), account, service.CPATestInput{
		UseAccountBaseURL:          req.UseAccountBaseURL,
		BaseURL:                    strings.TrimSpace(req.BaseURL),
		ManagementURL:              strings.TrimSpace(req.ManagementURL),
		ManagementPassword:         strings.TrimSpace(req.ManagementPassword),
		ConcurrencyPerCredential:   req.ConcurrencyPerCredential,
		ExcludeAbnormalCredentials: req.ExcludeAbnormalCredentials,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// SyncCPACapacity bypasses the normal snapshot TTL for an enabled CPA account.
func (h *AccountHandler) SyncCPACapacity(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	account, err := h.adminService.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	status, err := h.concurrencyService.ForceRefreshCPACapacity(c.Request.Context(), account)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

func (h *AccountHandler) getCPACapacityStatus(ctx context.Context, account *service.Account) *service.CPACapacityStatus {
	if h == nil || h.concurrencyService == nil || account == nil || !service.IsCPAModeEnabled(account) {
		return nil
	}
	status, _ := h.concurrencyService.GetCPACapacityStatus(ctx, account)
	return status
}

func (h *AccountHandler) getCPACapacityStatuses(ctx context.Context, accounts []service.Account) map[int64]*service.CPACapacityStatus {
	result := make(map[int64]*service.CPACapacityStatus)
	if h == nil || h.concurrencyService == nil || len(accounts) == 0 {
		return result
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for index := range accounts {
		account := &accounts[index]
		if !service.IsCPAModeEnabled(account) {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			status := h.getCPACapacityStatus(ctx, account)
			mu.Lock()
			result[account.ID] = status
			mu.Unlock()
		}()
	}
	wg.Wait()
	return result
}
