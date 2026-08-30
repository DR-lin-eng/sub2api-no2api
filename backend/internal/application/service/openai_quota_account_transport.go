package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
)

const openAICodexAuxiliaryBodyLimit int64 = 8 << 20

// quotaCredentialAccount returns the credential-bearing account that owns the
// upstream route. Shadow accounts intentionally resolve to their parent so the
// TLS profile, proxy/egress binding and ChatGPT credentials stay aligned.
func (s *OpenAIQuotaService) quotaCredentialAccount(ctx context.Context, accountID int64) (*Account, error) {
	if s == nil || s.accountRepo == nil {
		return nil, fmt.Errorf("account repository is unavailable")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("account not found")
	}
	if account.IsShadow() {
		account, err = resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil {
			return nil, err
		}
	}
	return account, nil
}

func (s *OpenAIQuotaService) quotaTLSProfile(account *Account) *tlsfingerprint.Profile {
	if s == nil || s.tlsFPProfileService == nil {
		return nil
	}
	return s.tlsFPProfileService.ResolveTLSProfile(account)
}

// doCodexQuotaHTTP is the raw HTTP counterpart of the legacy req.Client path.
// It deliberately uses HTTPUpstream so quota and reset calls share the same
// account-scoped TLS profile, HTTP protocol mode, connection pool and egress
// route as Responses traffic.
func (s *OpenAIQuotaService) doCodexQuotaHTTP(
	ctx context.Context,
	account *Account,
	method string,
	url string,
	headers map[string]string,
	body []byte,
) (int, http.Header, []byte, error) {
	if s == nil || s.httpUpstream == nil {
		return 0, nil, nil, fmt.Errorf("account-scoped HTTP upstream is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if account == nil {
		return 0, nil, nil, fmt.Errorf("account is nil")
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	requestCtx, cancel := context.WithTimeout(ctx, openaiQuotaUpstreamTimeout)
	defer cancel()
	requestCtx = withAccountEgressContext(requestCtx, account, proxyURL, s.cfg)
	req, err := http.NewRequestWithContext(requestCtx, method, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if len(body) > 0 && req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
	}
	profile := s.quotaTLSProfile(account)
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req = req.WithContext(WithHTTPUpstreamTLSProfile(req.Context(), profile))
	resp, err := doAccountHTTPUpstreamWithTLS(s.httpUpstream, req, proxyURL, account, profile)
	if err != nil {
		return 0, nil, nil, err
	}
	if resp == nil {
		return 0, nil, nil, fmt.Errorf("account-scoped HTTP upstream returned nil response")
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, openAICodexAuxiliaryBodyLimit))
	if readErr != nil {
		return resp.StatusCode, resp.Header.Clone(), nil, readErr
	}
	return resp.StatusCode, resp.Header.Clone(), responseBody, nil
}

func (s *OpenAIQuotaService) queryUsageWithAccountTransport(ctx context.Context, accountID int64, includeResetCredits bool) (*OpenAIQuotaUsage, error) {
	var route platformegress.Route
	accessToken, chatGPTAccountID, _, fedRAMP, err := s.prepareUpstreamCall(ctx, accountID, &route)
	if err != nil {
		return nil, err
	}
	account, err := s.quotaCredentialAccount(ctx, accountID)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_ACCOUNT_RESOLVE_FAILED", "failed to resolve quota account: %v", err)
	}
	agentIdentity := s.isAgentIdentityAccount(ctx, accountID)
	var payload OpenAIQuotaUsage
	for recovered := false; ; {
		headers, expectedTaskID, headerErr := s.buildCodexQuotaHeaders(ctx, accountID, accessToken, chatGPTAccountID, fedRAMP)
		if headerErr != nil {
			return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_AUTH_FAILED", "failed to build upstream authentication: %v", headerErr)
		}
		status, _, body, requestErr := s.doCodexQuotaHTTP(ctx, account, http.MethodGet, chatGPTUsageURL, headers, nil)
		if requestErr != nil {
			return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_REQUEST_FAILED", "upstream request failed: %v", requestErr)
		}
		if status < 200 || status >= 300 {
			if agentIdentity && !recovered && isAgentIdentityTaskInvalidHTTPResponse(status, body) {
				recovered = true
				if recoverErr := s.recoverAgentIdentityTask(ctx, accountID, expectedTaskID); recoverErr != nil {
					return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_AUTH_FAILED", "agent identity task recovery failed: %v", recoverErr)
				}
				continue
			}
			cleanBody := truncate(s.redactQuotaErrorBody(ctx, accountID, string(body)), 240)
			return nil, infraerrors.Newf(mapUpstreamStatus(status), "OPENAI_QUOTA_UPSTREAM_ERROR", "upstream returned %d: %s", status, cleanBody)
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_UPSTREAM_ERROR", "invalid quota response: %v", err)
		}
		break
	}
	payload.FetchedAt = time.Now().Unix()
	if includeResetCredits {
		if details := s.queryResetCreditDetailsWithAccountTransport(ctx, accountID, account, accessToken, chatGPTAccountID, fedRAMP); details != nil {
			applyOpenAIQuotaResetCreditDetails(&payload, details)
		}
	}
	return &payload, nil
}

func applyOpenAIQuotaResetCreditDetails(payload *OpenAIQuotaUsage, details *openAIRateLimitResetCreditDetails) {
	if payload == nil || details == nil {
		return
	}
	if payload.RateLimitResetCredits == nil {
		payload.RateLimitResetCredits = &OpenAIRateLimitResetCredits{}
	}
	if details.CreditListPresent {
		payload.RateLimitResetCredits.Credits = details.Credits
	}
	switch {
	case details.AvailableCount != nil:
		payload.RateLimitResetCredits.AvailableCount = *details.AvailableCount
	case details.CreditListPresent:
		payload.RateLimitResetCredits.AvailableCount = details.AvailableCreditCount
	}
}

func (s *OpenAIQuotaService) queryResetCreditDetailsWithAccountTransport(
	ctx context.Context,
	accountID int64,
	account *Account,
	accessToken, chatGPTAccountID string,
	fedRAMP bool,
) *openAIRateLimitResetCreditDetails {
	headers, _, err := s.buildCodexQuotaHeaders(ctx, accountID, accessToken, chatGPTAccountID, fedRAMP)
	if err != nil {
		return nil
	}
	status, _, body, err := s.doCodexQuotaHTTP(ctx, account, http.MethodGet, chatGPTRateLimitCreditsURL, headers, nil)
	if err != nil || status < 200 || status >= 300 {
		return nil
	}
	details, parseErr := parseOpenAIRateLimitResetCreditDetails(body)
	if parseErr != nil && details.AvailableCount == nil {
		return nil
	}
	if details.AvailableCount == nil && !details.CreditListPresent {
		return nil
	}
	return &details
}

func (s *OpenAIQuotaService) resetCreditWithAccountTransport(ctx context.Context, accountID int64) (*OpenAIQuotaResetResult, error) {
	var route platformegress.Route
	accessToken, chatGPTAccountID, _, fedRAMP, err := s.prepareUpstreamCall(ctx, accountID, &route)
	if err != nil {
		return nil, err
	}
	account, err := s.quotaCredentialAccount(ctx, accountID)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_ACCOUNT_RESOLVE_FAILED", "failed to resolve quota account: %v", err)
	}
	redeemRequestID, err := generateRedeemRequestID()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "OPENAI_QUOTA_REDEEM_ID_FAILED", "failed to generate redeem id: %v", err)
	}
	agentIdentity := s.isAgentIdentityAccount(ctx, accountID)
	requestBody, _ := json.Marshal(map[string]string{"redeem_request_id": redeemRequestID})
	var payload OpenAIQuotaResetResult
	for recovered := false; ; {
		headers, expectedTaskID, headerErr := s.buildCodexQuotaHeaders(ctx, accountID, accessToken, chatGPTAccountID, fedRAMP)
		if headerErr != nil {
			return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_AUTH_FAILED", "failed to build upstream authentication: %v", headerErr)
		}
		headers["content-type"] = "application/json"
		status, _, body, requestErr := s.doCodexQuotaHTTP(ctx, account, http.MethodPost, chatGPTRateLimitResetURL, headers, requestBody)
		if requestErr != nil {
			return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_RESET_REQUEST_FAILED", "upstream request failed: %v", requestErr)
		}
		if status < 200 || status >= 300 {
			if agentIdentity && !recovered && isAgentIdentityTaskInvalidHTTPResponse(status, body) {
				recovered = true
				if recoverErr := s.recoverAgentIdentityTask(ctx, accountID, expectedTaskID); recoverErr != nil {
					return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_AUTH_FAILED", "agent identity task recovery failed: %v", recoverErr)
				}
				continue
			}
			cleanBody := truncate(s.redactQuotaErrorBody(ctx, accountID, string(body)), 240)
			return nil, infraerrors.Newf(mapUpstreamStatus(status), "OPENAI_QUOTA_RESET_UPSTREAM_ERROR", "upstream returned %d: %s", status, cleanBody)
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_QUOTA_RESET_UPSTREAM_ERROR", "invalid reset response: %v", err)
		}
		break
	}
	return &payload, nil
}
