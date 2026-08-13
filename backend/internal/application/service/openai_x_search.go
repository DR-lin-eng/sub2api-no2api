package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/shared/xai"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// GrokXSearchForwardResult keeps the normalized billing result beside the raw
// Responses body used by the transport layer to build the standalone contract.
type GrokXSearchForwardResult struct {
	Body   []byte
	Result *OpenAIForwardResult
}

// ForwardGrokXSearch executes one non-streaming native xAI Responses request.
// Account selection, concurrency and billing remain owned by the HTTP handler;
// this method owns credentials, upstream I/O, quota observation and error state.
func (s *OpenAIGatewayService) ForwardGrokXSearch(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	requestID string,
) (*GrokXSearchForwardResult, error) {
	if s == nil || s.httpUpstream == nil {
		return nil, errors.New("http upstream not configured")
	}
	if account == nil || !account.IsGrok() {
		return nil, errors.New("grok account required")
	}
	if account.Type != AccountTypeOAuth && account.Type != AccountTypeAPIKey {
		return nil, fmt.Errorf("grok account type %s is not supported by x_search", account.Type)
	}

	requestedModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if requestedModel == "" {
		requestedModel = xai.DefaultTextModel
	}
	upstreamModel := xai.ResolveGrokTextResponsesModelID(strings.TrimSpace(account.GetMappedModel(requestedModel)))
	if isGrokImageGenerationModel(upstreamModel) {
		return nil, fmt.Errorf("model %s is not a Grok text model", upstreamModel)
	}
	patchedBody, err := patchGrokResponsesBody(body, upstreamModel)
	if err != nil {
		return nil, fmt.Errorf("prepare Grok x_search request: %w", err)
	}
	modelCtx := withGrokTeamRateLimitModel(ctx, upstreamModel)

	token, _, err := s.getRequestCredential(modelCtx, c, account)
	if err != nil {
		return nil, err
	}
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(modelCtx)
	defer releaseUpstreamCtx()
	upstreamReq, err := buildGrokResponsesRequest(upstreamCtx, c, account, patchedBody, token, "", s.cfg)
	if err != nil {
		return nil, fmt.Errorf("build Grok x_search request: %w", err)
	}
	SetActualOpenAIUpstreamEndpoint(c, grokChatResponsesEndpoint)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	startedAt := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(startedAt).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(modelCtx, c, account, err, false)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, fmt.Errorf("read Grok x_search response: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		upstreamMsg := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(respBody))
		if upstreamMsg == "" {
			upstreamMsg = fmt.Sprintf("xAI upstream returned status %d", resp.StatusCode)
		}
		kind := "http_error"
		if s.shouldFailoverGrokUpstreamError(resp.StatusCode, respBody) {
			kind = "failover"
		}
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id")),
			Kind:               kind,
			Message:            upstreamMsg,
		})
		s.handleGrokAccountUpstreamError(modelCtx, account, resp.StatusCode, resp.Header, respBody)
		if s.shouldFailoverGrokUpstreamError(resp.StatusCode, respBody) {
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				ResponseHeaders:        resp.Header.Clone(),
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return nil, fmt.Errorf("grok x_search upstream error: %s", upstreamMsg)
	}

	var response struct {
		ID    string                    `json:"id"`
		Usage *apicompat.ResponsesUsage `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &response); err != nil {
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id")),
			Kind:               "protocol_error",
			Message:            "invalid JSON in Grok x_search response",
		})
		return nil, &UpstreamFailoverError{
			StatusCode:   http.StatusBadGateway,
			ResponseBody: []byte(`{"error":{"type":"upstream_error","message":"Invalid Grok x_search response"}}`),
		}
	}
	s.updateGrokUsageFromResponse(modelCtx, account, resp.Header, resp.StatusCode)

	result := &OpenAIForwardResult{
		RequestID:        strings.TrimSpace(requestID),
		ResponseID:       strings.TrimSpace(response.ID),
		Model:            "grok-x-search",
		UpstreamModel:    upstreamModel,
		UpstreamEndpoint: grokChatResponsesEndpoint,
		ResponseHeaders:  resp.Header.Clone(),
		Duration:         time.Since(startedAt),
		WebSearchCalls:   1,
	}
	if response.Usage != nil {
		result.Usage = copyOpenAIUsageFromResponsesUsage(response.Usage)
	}
	return &GrokXSearchForwardResult{Body: respBody, Result: result}, nil
}
