package service

import (
	"bytes"
	"context"
	"encoding/json"
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
	parsed, parseErr := buildAccountImageTestRequest(ctx, account, modelID, prompt)
	if parseErr != nil {
		return s.sendErrorAndEnd(c, parseErr.Error())
	}
	if credentialAccount.IsOpenAISIWC() {
		return s.testOpenAIImageAPIKey(c, ctx, account, modelID, prompt)
	}
	appImages := s.settingService != nil && !s.settingService.OpenAIImagesLegacyEnabled(ctx) && !credentialAccount.IsOpenAISIWC() && !credentialAccount.IsOpenAIPersonalAccessToken() && !credentialAccount.IsOpenAIAgentIdentity()
	if !accountTestForceLegacyImages(ctx) && appImages {
		if parsed.OutputFormat == "svg" || parsed.SVG != nil {
			ctx = withAccountTestForceLegacyImages(ctx)
		} else {
			return s.testChatGPTAppImage(c, ctx, account, modelID, prompt)
		}
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
		if err := s.sendAccountImageTestResult(c, parsed, item.Result, item.OutputFormat, item.RevisedPrompt); err != nil {
			return err
		}
	}

	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}

// testOpenAIImageAPIKey tests OpenAI image generation using an API Key account.
func (s *AccountTestService) testOpenAIImageAPIKey(c *gin.Context, ctx context.Context, account *Account, modelID, prompt string) error {
	parsed, parseErr := buildAccountImageTestRequest(ctx, account, modelID, prompt)
	if parseErr != nil {
		return s.sendErrorAndEnd(c, parseErr.Error())
	}
	credentialAccount, err := resolveCredentialAccount(ctx, s.accountRepo, account)
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to resolve account credentials")
	}
	authToken := credentialAccount.GetOpenAIApiKey()
	if credentialAccount.IsOpenAISIWC() {
		authToken = credentialAccount.GetOpenAIAccessToken()
	}
	if authToken == "" {
		return s.sendErrorAndEnd(c, "No API key available")
	}

	baseURL := account.GetOpenAIBaseURL()
	if credentialAccount.IsOpenAISIWC() {
		baseURL, _, err = resolveOpenAIOAuthCodexBaseURL(ctx, s.settingService, s.cfg, credentialAccount)
		if err != nil {
			return s.sendErrorAndEnd(c, "Invalid SIWC API base URL")
		}
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Invalid base URL: %s", err.Error()))
	}
	apiURL := buildOpenAIImagesURL(normalizedBaseURL, openAIImagesGenerationsEndpoint)

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	s.sendEvent(c, TestEvent{Type: "test_start", Model: modelID})

	payload := map[string]any{
		"model":  modelID,
		"prompt": prompt,
		"size":   "1024x1024",
		"n":      1,
	}
	if size := account.OpenAIImageTestSize(); size != "" {
		payload["size"] = size
	}
	if parsed.Size != "" {
		payload["size"] = parsed.Size
	}
	if parsed.OutputFormat != "" {
		payload["output_format"] = parsed.OutputFormat
	}
	if parsed.FrameCount != nil {
		payload["frame_count"] = *parsed.FrameCount
	}
	if parsed.FrameDurationMS != nil {
		payload["frame_duration_ms"] = *parsed.FrameDurationMS
	}
	if parsed.SVG != nil {
		payload["svg"] = parsed.SVG
	}
	payloadBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.sendErrorAndEnd(c, "Failed to create request")
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)

	// 账号级请求头覆写：测试请求与真实转发保持一致的最终头
	account.ApplyHeaderOverrides(req.Header)
	if credentialAccount.IsOpenAISIWC() {
		applyOpenAISIWCHeaders(req.Header)
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.doAccountTestHTTPUpstreamWithTLS(req, proxyURL, account, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Request failed: %s", err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to read response: %s", err.Error()))
	}

	if resp.StatusCode != http.StatusOK {
		return s.sendErrorAndEnd(c, fmt.Sprintf("API returned %d: %s", resp.StatusCode, string(body)))
	}

	// Parse {"data": [{"b64_json": "...", "revised_prompt": "..."}]}
	var result struct {
		OutputFormat string `json:"output_format"`
		Data         []struct {
			B64JSON       string `json:"b64_json"`
			OutputFormat  string `json:"output_format"`
			URL           string `json:"url"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return s.sendErrorAndEnd(c, fmt.Sprintf("Failed to parse response: %s", err.Error()))
	}

	if len(result.Data) == 0 {
		return s.sendErrorAndEnd(c, "No images returned from API")
	}

	outputs := 0
	for _, item := range result.Data {
		if item.B64JSON != "" {
			outputs++
			format := firstNonEmptyString(item.OutputFormat, result.OutputFormat, parsed.OutputFormat)
			if err := s.sendAccountImageTestResult(c, parsed, item.B64JSON, format, item.RevisedPrompt); err != nil {
				return err
			}
		} else if item.URL != "" {
			outputs++
			if parsed.OutputFormat == "svg" || parsed.OutputFormat == "gif" {
				return s.sendErrorAndEnd(c, "The upstream returned a URL instead of file bytes; SVG/GIF output could not be verified")
			}
			s.sendEvent(c, TestEvent{
				Type:     "image",
				ImageURL: item.URL,
				MimeType: "image/*",
			})
		}
	}

	if outputs == 0 {
		return s.sendErrorAndEnd(c, "No image file was returned from API")
	}
	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}
