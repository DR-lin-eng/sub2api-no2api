package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *OpenAIGatewayService) buildUpstreamRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token string, isStream bool, promptCacheKey string, isCodexCLI bool) (*http.Request, error) {
	return s.buildUpstreamRequestWithFingerprint(ctx, c, account, body, token, isStream, promptCacheKey, isCodexCLI, nil)
}

func (s *OpenAIGatewayService) buildUpstreamRequestWithFingerprint(ctx context.Context, c *gin.Context, account *Account, body []byte, token string, isStream bool, promptCacheKey string, isCodexCLI bool, fingerprintIDs *codexFingerprintIDs) (*http.Request, error) {
	// A Gin context can be reused across account failover attempts. The caller's
	// explicit plan is the source of truth for this attempt, including nil when
	// the selected account has convergence disabled.
	stageCodexFingerprintIDs(c, fingerprintIDs)
	// Determine target URL based on account type
	var targetURL string
	switch account.Type {
	case AccountTypeOAuth:
		// OAuth accounts use ChatGPT internal API
		targetURL = chatgptCodexURL
	case AccountTypeAPIKey:
		// API Key accounts use Platform API or custom base URL
		baseURL := account.GetOpenAIBaseURL()
		if baseURL == "" {
			targetURL = openaiPlatformAPIURL
		} else {
			validatedURL, err := s.validateUpstreamBaseURL(baseURL)
			if err != nil {
				return nil, err
			}
			targetURL = buildOpenAIResponsesURL(validatedURL)
		}
	default:
		targetURL = openaiPlatformAPIURL
	}
	targetURL = appendOpenAIResponsesRequestPathSuffix(targetURL, openAIResponsesRequestPathSuffix(c))

	req, err := newOpenAIHTTPUpstreamRequest(ctx, http.MethodPost, targetURL, account, body)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), openAIHTTPUpstreamProfile(ctx, account, isStream)))

	// Build authentication for this request. Agent Identity signs a fresh
	// assertion here; OAuth/PAT/API-key keep their existing Bearer behavior.
	authHeaders, err := s.buildOpenAIAuthenticationHeaders(ctx, account, token)
	if err != nil {
		return nil, fmt.Errorf("build openai authentication headers: %w", err)
	}
	for key, values := range authHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Set headers specific to OAuth accounts (ChatGPT internal API)
	if account.Type == AccountTypeOAuth {
		// Required: set Host for ChatGPT API (must use req.Host, not Header.Set)
		req.Host = "chatgpt.com"
		if err := resolveAndSetOpenAIChatGPTAccountHeaders(ctx, s.accountRepo, req.Header, account); err != nil {
			return nil, fmt.Errorf("resolve chatgpt account headers: %w", err)
		}
	}

	// Whitelist passthrough headers
	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		if openaiAllowedHeaders[lowerKey] {
			for _, v := range values {
				req.Header.Add(key, v)
			}
		}
	}
	// This namespace signal is consumed only while constructing the request
	// root. Keep an explicit deletion even though it is not in the whitelist.
	if account.IsOpenAIOAuth() && s.CodexSimulationRequestEnabled(c) {
		req.Header.Del(CodexProjectIDHeader)
	}
	// A turn-state minted by another account is incompatible with this
	// attempt's outbound identity. Unknown and same-account values pass through.
	s.guardOpenAICodexTurnStateEcho(c, account, req.Header)
	if account.Type == AccountTypeOAuth {
		compatMessagesBridge := isOpenAICompatMessagesBridgeContext(c) || isOpenAICompatMessagesBridgeBody(body)
		// 清除客户端透传的 session 头，后续用隔离后的值重新设置，防止跨用户会话碰撞。
		clientConversationID := strings.TrimSpace(req.Header.Get("conversation_id"))
		req.Header.Del("conversation_id")
		req.Header.Del("session_id")

		if compatMessagesBridge {
			req.Header.Del("OpenAI-Beta")
			req.Header.Del("originator")
		} else {
			req.Header.Set("originator", resolveOpenAIUpstreamOriginator(c, isCodexCLI))
		}
		apiKeyID := getAPIKeyIDFromContext(c)
		if isOpenAIResponsesCompactPath(c) {
			req.Header.Set("accept", "application/json")
			if req.Header.Get("version") == "" {
				req.Header.Set("version", codexCLIVersion)
			}
			compactSession := resolveOpenAICompactSessionID(c)
			req.Header.Set("session_id", isolateOpenAISessionID(apiKeyID, compactSession))
		} else {
			req.Header.Set("accept", "text/event-stream")
		}
		if promptCacheKey != "" {
			isolated := isolateOpenAISessionID(apiKeyID, promptCacheKey)
			req.Header.Set("session_id", isolated)
			if !compatMessagesBridge || clientConversationID != "" {
				req.Header.Set("conversation_id", isolated)
			}
		}
	} else if isOpenAIResponsesCompactPath(c) {
		// compact 上游是 unary JSON 协议：API-key 账号也显式声明 Accept，
		// 避免 OpenAI 兼容网关按 SSE 返回（#3777 期望行为 4）。
		req.Header.Set("accept", "application/json")
	}

	// Apply custom User-Agent if configured
	customUA := account.GetOpenAIUserAgent()
	if customUA != "" {
		req.Header.Set("user-agent", customUA)
	}

	// 若开启 ForceCodexCLI，则强制将上游 User-Agent 伪装为 Codex CLI。
	// 用于网关未透传/改写 User-Agent 时，仍能命中 Codex 侧识别逻辑。
	if s.cfg != nil && s.cfg.Gateway.ForceCodexCLI {
		req.Header.Set("user-agent", codexCLIUserAgent)
	}
	applyStagedCodexFingerprintHeaders(c, account, req.Header)

	// OAuth requests leave through one canonical identity. Account-level UA
	// overrides keep their fingerprint but their version is rebuilt.
	if account.Type == AccountTypeOAuth {
		enforceCodexIdentityHeadersWithUA(req.Header, s.codexIdentityOverrideUA(account))
	}

	// Ensure required headers exist
	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
	}

	// 账号级请求头覆写（仅 openai api_key 账号启用时生效；OAuth 路径 no-op）
	account.ApplyHeaderOverrides(req.Header)
	applyOpenAICodexBetaFeatures(c, account, req.Header)
	applyOpenAICodexRoutingHintFromBody(ctx, account, "http", req.Header, body, "not_applicable")
	applyCodexSimulationProfileHeaders(req.Header, fingerprintIDs)

	return req, nil
}
