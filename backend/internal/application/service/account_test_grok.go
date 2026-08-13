package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/xai"
	"github.com/gin-gonic/gin"
)

// testGrokAccountConnection tests a Grok OAuth or API-key account through xAI's Responses API.
func (s *AccountTestService) testGrokAccountConnection(c *gin.Context, account *Account, modelID string) error {
	ctx := c.Request.Context()

	if s.httpUpstream == nil {
		return s.sendErrorAndEnd(c, "HTTP upstream not configured")
	}

	testModelID := strings.TrimSpace(modelID)
	if testModelID == "" {
		testModelID = grokDefaultResponsesModel
	}
	if mapped := strings.TrimSpace(account.GetMappedModel(testModelID)); mapped != "" {
		testModelID = mapped
	}

	var authToken string
	switch account.Type {
	case AccountTypeOAuth:
		if s.grokTokenProvider == nil {
			return s.sendErrorAndEnd(c, "Grok token provider not configured")
		}
		var err error
		// Manual tests intentionally bypass production scheduling eligibility.
		authToken, err = s.grokTokenProvider.GetAccessTokenForManualTest(ctx, account)
		if err != nil {
			return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to get Grok access token: %s", err.Error()))
		}
	case AccountTypeAPIKey:
		authToken = strings.TrimSpace(account.GetCredential("api_key"))
		if authToken == "" {
			return s.sendErrorAndEnd(c, "Grok API key is missing")
		}
	default:
		return s.sendErrorAndEnd(c, fmt.Sprintf("Unsupported Grok account type: %s", account.Type))
	}

	apiURL, err := buildGrokResponsesURL(account, s.cfg)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid Grok base URL: %s", err.Error()))
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	payloadBytes, err := buildGrokQuotaProbeBody(testModelID)
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create Grok test payload")
	}

	if !agentIdentityTaskRecoveryWasTried(ctx) {
		s.sendEvent(c, TestEvent{Type: "test_start", Model: testModelID})
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create Grok request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+authToken)
	if account.IsGrokOAuth() {
		applyGrokCLIHeaders(req.Header)
	}
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Grok Responses API request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	now := time.Now()
	snapshot := parseGrokQuotaSnapshot(resp.Header, resp.StatusCode, now)
	stampGrokQuotaSnapshotForPlan(account, snapshot, testModelID)
	if snapshot != nil && s.accountRepo != nil {
		resetAt, limited := grokRateLimitResetAtForAccount(account, snapshot, now)
		if limited {
			normalizeGrokExhaustedWindowResets(snapshot, resetAt, now)
		}
		_ = s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
			grokQuotaSnapshotExtraKey: snapshot,
		})
		if limited {
			persistGrokRateLimit(ctx, s.accountRepo, account, resetAt)
		} else if isSuccessfulGrokRateLimitRecovery(account, snapshot) {
			clearGrokRateLimitAfterRecovery(ctx, s.accountRepo, account)
		}
	} else if s.accountRepo != nil && isSuccessfulGrokRateLimitRecovery(account, &xai.QuotaSnapshot{StatusCode: resp.StatusCode}) {
		clearGrokRateLimitAfterRecovery(ctx, s.accountRepo, account)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusPaymentRequired && s.accountRepo != nil {
			stateCtx, cancel := openAIAccountStateContext(ctx)
			defer cancel()
			_ = s.accountRepo.SetTempUnschedulable(
				stateCtx,
				account.ID,
				time.Now().Add(grokPaymentRequiredCooldown),
				grokPaymentRequiredReason,
			)
		}
		return s.sendErrorAndEnd(c, fmt.Sprintf("Grok Responses API returned %d: %s", resp.StatusCode, string(body)))
	}

	return s.processOpenAIStream(c, resp.Body)
}
