package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/claude"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/stretchr/testify/assert"
)

func TestSyncBillingHeaderVersion(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		userAgent string
		wantSub   string // substring expected in result
		unchanged bool   // expect body to remain the same
	}{
		{
			name:      "replaces cc_version and recomputes message-derived suffix",
			body:      `{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.81.df2; cc_entrypoint=cli; cch=00000;"},{"type":"text","text":"You are Claude Code.","cache_control":{"type":"ephemeral"}}],"messages":[]}`,
			userAgent: "claude-cli/2.1.22 (external, cli)",
			wantSub:   "cc_version=2.1.22." + computeClaudeCodeFingerprint([]byte(`{"messages":[]}`), "2.1.22"),
		},
		{
			name:      "no billing header in system",
			body:      `{"system":[{"type":"text","text":"You are Claude Code."}],"messages":[]}`,
			userAgent: "claude-cli/2.1.22",
			unchanged: true,
		},
		{
			name:      "no system field",
			body:      `{"messages":[]}`,
			userAgent: "claude-cli/2.1.22",
			unchanged: true,
		},
		{
			name:      "user-agent without version",
			body:      `{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.81; cc_entrypoint=cli; cch=00000;"}],"messages":[]}`,
			userAgent: "Mozilla/5.0",
			unchanged: true,
		},
		{
			name:      "empty user-agent",
			body:      `{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.81; cc_entrypoint=cli; cch=00000;"}],"messages":[]}`,
			userAgent: "",
			unchanged: true,
		},
		{
			name:      "version already matches",
			body:      `{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.22; cc_entrypoint=cli; cch=00000;"}],"messages":[]}`,
			userAgent: "claude-cli/2.1.22",
			unchanged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := syncBillingHeaderVersion([]byte(tt.body), tt.userAgent)
			if tt.unchanged {
				assert.Equal(t, tt.body, string(result), "body should remain unchanged")
			} else {
				assert.Contains(t, string(result), tt.wantSub)
				// Ensure old semver is gone
				assert.NotContains(t, string(result), "cc_version=2.1.81")
			}
		})
	}
}

func TestSyncBillingHeaderVersion_RecomputesFingerprintIdempotently(t *testing.T) {
	body := []byte(`{"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.81.df2; cc_entrypoint=cli;"}],"messages":[{"role":"user","content":"hello"}]}`)
	got := syncBillingHeaderVersion(body, "claude-cli/2.1.220")
	want := "cc_version=2.1.220." + computeClaudeCodeFingerprint(body, "2.1.220")
	if !strings.Contains(gjson.GetBytes(got, "system.0.text").String(), want) {
		t.Fatalf("billing header=%s want substring=%s", gjson.GetBytes(got, "system.0.text").String(), want)
	}
	if string(got) != string(syncBillingHeaderVersion(got, "claude-cli/2.1.220")) {
		t.Fatal("billing rewrite must be idempotent")
	}
}

func TestEffectiveBillingUserAgent(t *testing.T) {
	tests := []struct {
		name        string
		tokenType   string
		mimic       bool
		fingerprint *Fingerprint
		want        string
	}{
		{name: "mimic uses canonical Claude UA", tokenType: "oauth", mimic: true, fingerprint: &Fingerprint{UserAgent: "claude-cli/2.9.0"}, want: claude.DefaultHeaders["User-Agent"]},
		{name: "fingerprint used when not mimicking", tokenType: "oauth", fingerprint: &Fingerprint{UserAgent: "claude-cli/2.9.0"}, want: "claude-cli/2.9.0"},
		{name: "nil fingerprint is empty", tokenType: "oauth", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := effectiveBillingUserAgent(tt.tokenType, tt.mimic, tt.fingerprint); got != tt.want {
				t.Fatalf("effectiveBillingUserAgent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildOAuthRequest_BillingMatchesWireUserAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"messages", "count_tokens"} {
		for _, tc := range []struct {
			name      string
			mimic     bool
			identity  bool
			disableFP bool
		}{
			{name: "mimic_overrides_cached_version", mimic: true, identity: true},
			{name: "mimic_without_identity", mimic: true},
			{name: "mimic_with_fingerprint_disabled", mimic: true, identity: true, disableFP: true},
			{name: "passthrough_uses_cached_version", identity: true},
		} {
			t.Run(endpoint+"/"+tc.name, func(t *testing.T) {
				resetGatewayForwardingSettingsCacheForTest(t)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				body := []byte(`{"model":"claude-haiku-4-5","system":[{"type":"text","text":""}],"messages":[{"role":"user","content":"hello world"}]}`)
				billing, err := buildBillingAttributionText(body, "2.1.81")
				require.NoError(t, err)
				body, err = sjson.SetBytes(body, "system.0.text", billing)
				require.NoError(t, err)

				cfg := &config.Config{}
				svc := &GatewayService{cfg: cfg}
				cachedUA := "claude-cli/2.9.0 (external, cli)"
				if tc.identity {
					svc.identityService = NewIdentityService(&stubIdentityCache{fingerprint: &Fingerprint{
						UserAgent: cachedUA, ClientID: "test-client", UpdatedAt: time.Now().Unix(),
					}})
				}
				if tc.disableFP {
					svc.settingService = NewSettingService(&gatewayTTLSettingRepo{data: map[string]string{
						SettingKeyEnableFingerprintUnification: "false",
					}}, cfg)
				}
				account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeOAuth}
				var req *http.Request
				var wireBody []byte
				if endpoint == "messages" {
					req, wireBody, err = svc.buildUpstreamRequest(context.Background(), c, account,
						body, "test-token", "oauth", "claude-haiku-4-5", false, tc.mimic)
				} else {
					req, wireBody, err = svc.buildCountTokensRequest(context.Background(), c, account,
						body, "test-token", "oauth", "claude-haiku-4-5", tc.mimic)
				}
				require.NoError(t, err)
				defer func() { require.NoError(t, req.Body.Close()) }()
				wantUA := cachedUA
				if tc.mimic {
					wantUA = claude.DefaultHeaders["User-Agent"]
				}
				require.Equal(t, wantUA, getHeaderRaw(req.Header, "User-Agent"))
				version := ExtractCLIVersion(wantUA)
				require.Contains(t, gjson.GetBytes(wireBody, "system.0.text").String(),
					"cc_version="+version+"."+computeClaudeCodeFingerprint(wireBody, version)+";")
				actualBody, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.Equal(t, wireBody, actualBody)
			})
		}
	}
}
