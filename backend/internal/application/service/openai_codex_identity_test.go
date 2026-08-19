package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func requireOpenAICodexProbeHeaders(t *testing.T, h http.Header) {
	t.Helper()
	require.Equal(t, codexCLIUserAgent, h.Get("User-Agent"))
	require.Equal(t, "codex_cli_rs", h.Get("Originator"))
	require.Equal(t, codexCLIVersion, h.Get("Version"))
	require.Equal(t, "responses=experimental", h.Get("OpenAI-Beta"))
	require.NotEmpty(t, h.Get("X-Codex-Window-ID"))
}

func TestEnsureCodexIdentityHeaders(t *testing.T) {
	t.Run("补齐缺失身份头", func(t *testing.T) {
		h := make(http.Header)

		ensureCodexIdentityHeaders(h)
		enforceCodexIdentityHeaders(h)

		require.Equal(t, "codex_cli_rs", h.Get("originator"))
		require.Equal(t, codexCLIUserAgent, h.Get("user-agent"))
		require.Equal(t, codexCLIVersion, h.Get("version"))
		require.Equal(t, "responses=experimental", h.Get("OpenAI-Beta"))
	})

	t.Run("统一非 CLI 官方客户端身份", func(t *testing.T) {
		const vscodeUA = "codex_vscode/9.9.9 (Mac OS X 14.0; arm64) vscode (codex_vscode; 9.9.9)"
		h := make(http.Header)
		h.Set("user-agent", vscodeUA)
		h.Set("version", "9.9.9")
		h.Set("OpenAI-Beta", "assistants=v2")

		ensureCodexIdentityHeaders(h)
		enforceCodexIdentityHeaders(h)

		require.Equal(t, "codex_cli_rs", h.Get("originator"))
		require.Equal(t, codexCLIUserAgent, h.Get("user-agent"))
		require.Equal(t, codexCLIVersion, h.Get("version"))
		require.Equal(t, "responses=experimental", h.Get("OpenAI-Beta"))
	})
}

func TestEnforceCodexIdentityHeaders(t *testing.T) {
	tests := []struct {
		name       string
		originator string
		userAgent  string
		version    string
	}{
		{
			name:       "错配 originator",
			originator: "codex_cli_rs",
			userAgent:  "codex-tui/0.140.2 (Mac OS X 14.0; arm64) iTerm (codex-tui; 0.140.2)",
		},
		{
			name:       "官方 vscode 身份",
			originator: "codex_vscode",
			userAgent:  "codex_vscode/1.2.3 (Ubuntu 22.4.0; x86_64) vscode",
		},
		{
			name:       "第三方 UA",
			originator: "opencode",
			userAgent:  "luna/1.0.0",
			version:    "2.1.0",
		},
		{
			name:       "UA 缺失",
			originator: "codex_vscode",
		},
		{
			name:       "originator override",
			originator: "cccc",
			userAgent:  "cccc/0.142.0 (Ubuntu 22.4.0; x86_64) screen (codex-tui; 0.142.0)",
		},
		{
			name:       "陈旧客户端版本",
			originator: "codex_cli_rs",
			userAgent:  "codex_cli_rs/0.125.0",
			version:    "0.125.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := make(http.Header)
			if tt.originator != "" {
				h.Set("originator", tt.originator)
			}
			if tt.userAgent != "" {
				h.Set("user-agent", tt.userAgent)
			}
			if tt.version != "" {
				h.Set("version", tt.version)
			}

			enforceCodexIdentityHeaders(h)

			require.Equal(t, "codex_cli_rs", h.Get("originator"))
			require.Equal(t, codexCLIUserAgent, h.Get("user-agent"))
			require.Equal(t, codexCLIVersion, h.Get("version"))
		})
	}
}

func TestCodexIdentityEnforcementZeroValueConfigKeepsItEnabled(t *testing.T) {
	var cfg config.Config
	require.False(t, cfg.Gateway.DisableCodexIdentityEnforcement)

	SetCodexIdentityEnforcementEnabled(!cfg.Gateway.DisableCodexIdentityEnforcement)
	t.Cleanup(func() { SetCodexIdentityEnforcementEnabled(true) })

	h := make(http.Header)
	h.Set("originator", "codex-tui")
	h.Set("user-agent", "codex-tui/0.140.2 (Mac OS X 14.0; arm64) iTerm (codex-tui; 0.140.2)")
	enforceCodexIdentityHeaders(h)
	require.Equal(t, "codex_cli_rs", h.Get("originator"))
}

func TestEnforceCodexIdentityHeadersEnforcementDisabled(t *testing.T) {
	const tuiUA = "codex-tui/0.145.2 (Mac OS X 14.0; arm64) iTerm (codex-tui; 0.145.2)"
	SetCodexIdentityEnforcementEnabled(false)
	t.Cleanup(func() { SetCodexIdentityEnforcementEnabled(true) })

	h := make(http.Header)
	h.Set("originator", "codex-tui")
	h.Set("user-agent", tuiUA)
	h.Set("version", "0.145.2")
	enforceCodexIdentityHeaders(h)

	require.Equal(t, "codex-tui", h.Get("originator"))
	require.Equal(t, tuiUA, h.Get("user-agent"))
	require.Equal(t, "0.145.2", h.Get("version"))
}

func TestEnforceCodexIdentityHeadersFollowsCanonicalResolver(t *testing.T) {
	SetCodexCanonicalUserAgentResolver(func() string {
		return "codex_cli_rs/0.200.1" + codexCLIUserAgentSuffix
	})
	t.Cleanup(func() { SetCodexCanonicalUserAgentResolver(nil) })

	h := make(http.Header)
	h.Set("originator", "codex-tui")
	h.Set("user-agent", "codex-tui/0.140.2")
	enforceCodexIdentityHeaders(h)

	require.Equal(t, "codex_cli_rs", h.Get("originator"))
	require.Equal(t, "codex_cli_rs/0.200.1"+codexCLIUserAgentSuffix, h.Get("user-agent"))
	require.Equal(t, "0.200.1", h.Get("version"))
}

func TestResolveCodexOutboundIdentityNormalizesConfiguredTUIWithoutDroppingFingerprint(t *testing.T) {
	SetCodexIdentityEnforcementEnabled(true)
	t.Cleanup(func() { SetCodexIdentityEnforcementEnabled(true) })

	configured := "codex-tui/0.146.0 (Mac OS X 14.0; arm64) iTerm (codex-tui; 0.146.0)"
	identity := resolveCodexOutboundIdentity(configured)

	require.Equal(t, "codex_cli_rs", identity.originator)
	require.Equal(t, "codex_cli_rs/0.146.0 (Mac OS X 14.0; arm64) iTerm", identity.userAgent)
	require.Equal(t, "0.146.0", identity.version)
}

func TestResolveCodexOutboundIdentityNormalizesCanonicalTUI(t *testing.T) {
	SetCodexIdentityEnforcementEnabled(true)
	SetCodexCanonicalUserAgentResolver(func() string {
		return "codex-tui/0.147.1 (Mac OS X 14.1; arm64) iTerm (codex-tui; 0.147.1)"
	})
	t.Cleanup(func() {
		SetCodexCanonicalUserAgentResolver(nil)
		SetCodexIdentityEnforcementEnabled(true)
	})

	identity := resolveCodexOutboundIdentity("")
	require.Equal(t, "codex_cli_rs", identity.originator)
	require.Equal(t, "codex_cli_rs/0.147.1 (Mac OS X 14.1; arm64) iTerm", identity.userAgent)
	require.Equal(t, "0.147.1", identity.version)
}

func TestResolveCodexOutboundIdentityRollbackPreservesTUI(t *testing.T) {
	SetCodexIdentityEnforcementEnabled(false)
	t.Cleanup(func() { SetCodexIdentityEnforcementEnabled(true) })

	configured := "codex-tui/0.146.0 (Mac OS X 14.0; arm64) iTerm (codex-tui; 0.146.0)"
	identity := resolveCodexOutboundIdentity(configured)

	require.Equal(t, "codex-tui", identity.originator)
	require.Equal(t, configured, identity.userAgent)
}

func TestBuildUpstreamRequestNormalizesConfiguredTUIAccountIdentity(t *testing.T) {
	SetCodexIdentityEnforcementEnabled(true)
	t.Cleanup(func() { SetCodexIdentityEnforcementEnabled(true) })
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("originator", "codex-tui")
	account := &Account{
		ID:       44,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":       "token",
			"chatgpt_account_id": "account",
			"user_agent":         "codex-tui/0.146.0 (Mac OS X 14.0; arm64) iTerm (codex-tui; 0.146.0)",
		},
	}

	req, err := (&OpenAIGatewayService{}).buildUpstreamRequest(
		context.Background(), c, account,
		[]byte(`{"model":"gpt-5.5","stream":true,"input":[]}`),
		"token", true, "", true,
	)
	require.NoError(t, err)
	require.Equal(t, "codex_cli_rs", req.Header.Get("originator"))
	require.Equal(t, "codex_cli_rs/0.146.0 (Mac OS X 14.0; arm64) iTerm", req.Header.Get("user-agent"))
}

func TestNormalizeCodexClientVersion(t *testing.T) {
	require.Equal(t, "0.146.0", NormalizeCodexClientVersion(" 0.146.0 "))
	require.Equal(t, "0.147.0-alpha.4", NormalizeCodexClientVersion("0.147.0-alpha.4"))
	require.Empty(t, NormalizeCodexClientVersion("v0.146.0"))
	require.Empty(t, NormalizeCodexClientVersion("0.146.0\r\nX-Injected: 1"))
	require.Empty(t, NormalizeCodexClientVersion("latest"))
}

// enforce 本身仍只负责收口：缺少 originator 时必须保持 no-op，由需要恢复身份的
// 调用方先显式调用 ensureCodexIdentityHeaders。
func TestEnforceCodexIdentityHeaders_NoOriginatorIsNoop(t *testing.T) {
	h := make(http.Header)
	h.Set("user-agent", "third-party-client/1.0.0")

	enforceCodexIdentityHeaders(h)

	require.Empty(t, h.Get("originator"))
	require.Equal(t, "third-party-client/1.0.0", h.Get("user-agent"))
}
