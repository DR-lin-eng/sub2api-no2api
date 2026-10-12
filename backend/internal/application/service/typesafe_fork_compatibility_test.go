//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	moduleegress "github.com/Wei-Shaw/sub2api/internal/modules/egress"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type typeSafeRoutedUpstream struct {
	HTTPUpstream
	route  platformegress.Route
	header http.Header
	calls  int
}

func (u *typeSafeRoutedUpstream) DoRoute(req *http.Request, route platformegress.Route, _ int64, _ int) (*http.Response, error) {
	u.calls++
	u.route, u.header = route, make(http.Header)
	for name, values := range req.Header {
		for _, value := range values {
			u.header.Add(name, value)
		}
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"model":"jev-latest","answers":{},"usage":{"input_tokens":3}}`))}, nil
}

func (u *typeSafeRoutedUpstream) DoWithTLSRoute(req *http.Request, route platformegress.Route, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.DoRoute(req, route, accountID, concurrency)
}

func TestTypeSafeNativeAndAccountTestUseForkEgressAndHeaders(t *testing.T) {
	for _, probe := range []bool{false, true} {
		for _, proxy := range []bool{false, true} {
			account := &Account{ID: 91, Platform: PlatformTypeSafe, Type: AccountTypeAPIKey, Credentials: map[string]any{
				"api_key": "fixture-secret", "header_override_enabled": true,
				"header_overrides": map[string]any{"X-Relay-Access": "fixture", "Authorization": "blocked"},
			}, EgressMode: platformegress.ModeIPv6Pool, EgressBinding: &moduleegress.Binding{
				PoolID: 7, SourceIPv6: "2001:db8:7::91", Status: moduleegress.BindingStatusActive, PoolStatus: moduleegress.PoolStatusActive, Version: 2,
			}}
			if proxy {
				id := int64(9)
				account.ProxyID, account.Proxy = &id, &Proxy{ID: id, Protocol: "http", Host: "proxy.test", Port: 8080}
			}
			upstream := &typeSafeRoutedUpstream{}
			c := newSystemOneTestContext()
			if probe {
				svc := &AccountTestService{httpUpstream: upstream, cfg: newSystemOneTestService(upstream).cfg}
				require.NoError(t, svc.testTypeSafeAccountConnection(c, account, "fixture"))
			} else {
				_, err := newSystemOneTestService(upstream).ForwardSystemOne(context.Background(), c, account, []byte(`{}`))
				require.NoError(t, err)
			}
			require.Equal(t, 1, upstream.calls)
			require.Equal(t, "Bearer fixture-secret", upstream.header.Get("Authorization"))
			require.Equal(t, "fixture", upstream.header.Get("X-Relay-Access"))
			if proxy {
				require.Equal(t, platformegress.ModeExternalProxy, upstream.route.Mode)
				require.Equal(t, "http://proxy.test:8080", upstream.route.ProxyURL)
				require.Empty(t, upstream.route.SourceIPv6)
			} else {
				require.Equal(t, platformegress.ModeIPv6Pool, upstream.route.Mode)
				require.Equal(t, "2001:db8:7::91", upstream.route.SourceIPv6)
			}
		}
	}
}

func TestTypeSafeBillingProbeAndModelIdentity(t *testing.T) {
	require.True(t, IsUpstreamBillingProbeIdentity(PlatformTypeSafe, AccountTypeAPIKey))
	for _, kind := range []string{AccountTypeOAuth, AccountTypeSetupToken, "future"} {
		require.False(t, IsUpstreamBillingProbeIdentity(PlatformTypeSafe, kind))
	}
	require.True(t, upstreamBillingProbeTargetIsOfficialAPI("https://api.typesafe.ai/v1"))
	require.False(t, upstreamBillingProbeTargetIsOfficialAPI("https://typesafe.ai.relay.test/v1"))
	require.False(t, upstreamBillingProbeTargetIsOfficialAPI("https://relay.test/typesafe.ai"))
	account := &Account{Platform: PlatformTypeSafe, Type: AccountTypeAPIKey}
	require.Equal(t, map[string]string{"jev-latest": "jev-latest"}, account.GetModelMapping())
	require.False(t, account.IsModelSupported("claude-sonnet-4-5"))
	require.True(t, account.IsModelSupported("jev-latest"))
	pricing := NewBillingService(&config.Config{}, nil).getFallbackPricing("jev-latest")
	require.InDelta(t, 0.042/1e6, pricing.InputPricePerToken, 1e-14)
	require.Zero(t, pricing.OutputPricePerToken)
}
