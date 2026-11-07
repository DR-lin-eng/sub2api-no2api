package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// testOpenAIImageOAuth follows the configured App or legacy Codex image protocol.
func (s *AccountTestService) testOpenAIImageOAuth(c *gin.Context, ctx context.Context, account *Account, modelID, prompt string) error {
	credentialAccount := account
	if account.IsShadow() {
		resolved, err := resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil {
			return s.sendErrorAndEnd(c, "Failed to resolve account credentials")
		}
		credentialAccount = resolved
	}
	if s.settingService != nil && !s.settingService.OpenAIImagesLegacyEnabled(ctx) && !credentialAccount.IsOpenAISIWC() && !credentialAccount.IsOpenAIPersonalAccessToken() && !credentialAccount.IsOpenAIAgentIdentity() {
		return s.testChatGPTAppImage(c, ctx, account, modelID, prompt)
	}
	authToken := ""
	if !credentialAccount.IsOpenAIAgentIdentity() {
		authToken = credentialAccount.GetOpenAIAccessToken()
	}
	if authToken == "" && !credentialAccount.IsOpenAIAgentIdentity() {
		return s.sendErrorAndEnd(c, "No access token available")
	}

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	s.sendEvent(c, TestEvent{Type: "test_start", Model: modelID})
	if !usesCodexDirectImages(modelID) {
		s.sendEvent(c, TestEvent{Type: "content", Text: "Calling Codex /responses image tool...\n"})
	}

	parsed := &OpenAIImagesRequest{
		Endpoint: openAIImagesGenerationsEndpoint,
		Model:    strings.TrimSpace(modelID),
		Prompt:   prompt,
		Size:     account.OpenAIImageTestSize(),
	}
	parsed.ExplicitSize = parsed.Size != ""
	applyOpenAIImagesDefaults(parsed)
	if !usesCodexDirectImages(parsed.Model) {
		s.sendEvent(c, TestEvent{Type: "content", Text: fmt.Sprintf("Responses driver: %s; image model: %s\n", openAIImagesResponsesMainModelValue(), parsed.Model)})
	}

	responsesBody, _, err := buildOpenAIImagesOAuthPayload(parsed, parsed.Model)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to build image request: %s", err.Error()))
	}

	baseURL, useOfficialOAuthEndpoint, relayErr := resolveOpenAIOAuthCodexBaseURL(ctx, s.settingService, s.cfg, credentialAccount)
	if relayErr != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid OpenAI OAuth Codex relay URL: %s", relayErr.Error()))
	}
	targetURL := buildOpenAIOAuthCodexResponsesURL(baseURL)
	if usesCodexDirectImages(parsed.Model) {
		targetURL = strings.TrimSuffix(targetURL, "/responses") + "/images/generations"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(responsesBody))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create request")
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	if useOfficialOAuthEndpoint {
		req.Host = "chatgpt.com"
	}
	if credentialAccount.IsOpenAIAgentIdentity() {
		authHeaders, authErr := buildAgentIdentityAuthenticationHeaders(ctx, s.accountRepo, s.agentIdentityWS, &s.agentIdentityTaskMu, credentialAccount, s.cfg)
		if authErr != nil {
			_ = setOpenAIAgentIdentityAccountError(ctx, s.accountRepo, s.agentIdentityWS, credentialAccount, "Agent Identity authentication failed: "+authErr.Error())
			return s.sendErrorAndEnd(c, "Failed to build Agent Identity authentication")
		}
		for key, values := range authHeaders {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
	} else {
		req.Header.Set("Authorization", "Bearer "+authToken)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	if usesCodexDirectImages(parsed.Model) {
		req.Header.Del("OpenAI-Beta")
		req.Header.Set("Accept", "application/json")
	}
	req.Header.Set("originator", "codex_cli_rs")
	if customUA := strings.TrimSpace(credentialAccount.GetOpenAIUserAgent()); customUA != "" {
		req.Header.Set("User-Agent", customUA)
	} else {
		req.Header.Set("User-Agent", codexCLIUserAgent)
	}
	setOpenAIChatGPTAccountHeaders(req.Header, credentialAccount)
	// 与真实转发一致：originator 与最终 User-Agent 首段配套（原 opencode 与 Codex UA 错配会 404，issue #3901）。
	enforceCodexIdentityHeadersWithUA(req.Header, credentialAccount.GetOpenAIUserAgent())

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.doAccountTestHTTPUpstream(req, proxyURL, account)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Responses API request failed: %s", err.Error()))
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		body = redactAgentIdentitySensitiveBodyForAccount(ctx, s.accountRepo, credentialAccount, body)
		if resp.StatusCode == http.StatusForbidden && credentialAccount.IsOpenAIAgentIdentity() {
			_ = setOpenAIAgentIdentityAccountError(ctx, s.accountRepo, s.agentIdentityWS, credentialAccount, buildForbiddenErrorMessage(
				"Agent Identity access forbidden (403):", extractUpstreamErrorMessage(body), body, "credentials require account action",
			))
		}
		message := strings.TrimSpace(extractUpstreamErrorMessage(body))
		if message == "" {
			message = fmt.Sprintf("Responses API returned %d", resp.StatusCode)
		}
		return s.sendErrorAndEnd(c, message)
	}

	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to read image response: %s", err.Error()))
	}
	body = redactAgentIdentitySensitiveBodyForAccount(ctx, s.accountRepo, credentialAccount, body)

	var results []openAIResponsesImageResult
	if usesCodexDirectImages(parsed.Model) {
		results, err = parseCodexDirectImagesResponse(body)
	} else {
		results, _, _, _, _, err = collectOpenAIImagesFromResponsesBody(body)
	}
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to parse image response: %s", err.Error()))
	}
	if len(results) == 0 {
		if upstreamErr := extractOpenAIImagesUpstreamError(body); upstreamErr != nil {
			return s.sendErrorAndEnd(c, upstreamErr.clientMessage())
		}
		if textErr := openAIImagesTextFallbackError(body); textErr != nil {
			return s.sendErrorAndEnd(c, textErr.clientMessage())
		}
		return s.sendErrorAndEnd(c, "No images returned from responses API")
	}

	for _, item := range results {
		if item.RevisedPrompt != "" {
			s.sendEvent(c, TestEvent{Type: "content", Text: item.RevisedPrompt})
		}
		mimeType := openAIImageOutputMIMEType(item.OutputFormat)
		s.sendEvent(c, TestEvent{
			Type:     "image",
			ImageURL: "data:" + mimeType + ";base64," + item.Result,
			MimeType: mimeType,
		})
	}

	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}
