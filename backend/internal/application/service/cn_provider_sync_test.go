//go:build unit

package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	moduleegress "github.com/Wei-Shaw/sub2api/internal/modules/egress"
	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type cnProbeAccountRepo struct {
	AccountRepository
	updates map[string]any
}

func (r *cnProbeAccountRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updates = updates
	return nil
}

type cnProbeRoutedUpstream struct {
	body        string
	route       platformegress.Route
	routedCalls int
	legacyCalls int
}

type blockingCNProbeUpstream struct {
	body    string
	calls   atomic.Int32
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingCNProbeUpstream(body string) *blockingCNProbeUpstream {
	return &blockingCNProbeUpstream{
		body:    body,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (u *blockingCNProbeUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) {
	return nil, fmt.Errorf("legacy Do must not be used")
}

func (u *blockingCNProbeUpstream) DoWithTLS(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
	return nil, fmt.Errorf("legacy DoWithTLS must not be used")
}

func (u *blockingCNProbeUpstream) DoRoute(*http.Request, platformegress.Route, int64, int) (*http.Response, error) {
	u.calls.Add(1)
	u.once.Do(func() { close(u.entered) })
	<-u.release
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(u.body)),
	}, nil
}

func (u *blockingCNProbeUpstream) DoWithTLSRoute(req *http.Request, route platformegress.Route, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.DoRoute(req, route, accountID, concurrency)
}

func (u *cnProbeRoutedUpstream) response() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(u.body)),
	}
}

func (u *cnProbeRoutedUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.legacyCalls++
	return nil, fmt.Errorf("legacy Do must not be used")
}

func (u *cnProbeRoutedUpstream) DoWithTLS(_ *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	u.legacyCalls++
	return nil, fmt.Errorf("legacy DoWithTLS must not be used")
}

func (u *cnProbeRoutedUpstream) DoRoute(_ *http.Request, route platformegress.Route, _ int64, _ int) (*http.Response, error) {
	u.routedCalls++
	u.route = route
	return u.response(), nil
}

func (u *cnProbeRoutedUpstream) DoWithTLSRoute(_ *http.Request, route platformegress.Route, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	u.routedCalls++
	u.route = route
	return u.response(), nil
}

func TestCNProviderSyncProtocolDefaultsAndNativeResponses(t *testing.T) {
	mk := func(platform, protocol string) *Account {
		credentials := map[string]any{"api_key": "key"}
		if protocol != "" {
			credentials["api_protocol"] = protocol
		}
		return &Account{Platform: platform, Type: AccountTypeAPIKey, Credentials: credentials}
	}

	require.Equal(t, APIProtocolChatCompletions, mk(PlatformKimi, "").GetAPIProtocol())
	require.Equal(t, APIProtocolAdaptive, mk(PlatformOpenCodeGo, "").GetAPIProtocol())
	require.Equal(t, APIProtocolChatCompletions, mk(PlatformZhipu, APIProtocolResponses).GetAPIProtocol())
	for _, platform := range []string{PlatformKimi, PlatformDeepseek, PlatformMiniMax, PlatformOpenCodeGo} {
		require.True(t, mk(platform, APIProtocolResponses).UsesNativeCNResponses(), platform)
	}
	require.False(t, mk(PlatformZhipu, APIProtocolResponses).UsesNativeCNResponses())
}

func TestCNProviderSyncProtocolBaseURLs(t *testing.T) {
	cases := []struct {
		platform, mode, chat, anthropic string
	}{
		{PlatformKimi, AccountModePayG, DefaultKimiPayGBaseURL, DefaultKimiPayGAnthropicBaseURL},
		{PlatformKimi, AccountModeCoding, DefaultKimiCodingBaseURL, DefaultKimiCodingAnthropicBaseURL},
		{PlatformZhipu, AccountModeCoding, DefaultZhipuCodingBaseURL, DefaultZhipuAnthropicBaseURL},
		{PlatformDeepseek, AccountModePayG, DefaultDeepseekBaseURL, DefaultDeepseekAnthropicBaseURL},
		{PlatformMiniMax, AccountModeCoding, DefaultMiniMaxBaseURL, DefaultMiniMaxAnthropicBaseURL},
		{PlatformOpenCodeGo, AccountModeGo, DefaultOpenCodeGoBaseURL, DefaultOpenCodeGoAnthropicBaseURL},
		{PlatformOpenCodeGo, AccountModeZen, DefaultOpenCodeZenBaseURL, DefaultOpenCodeZenAnthropicBaseURL},
	}
	for _, tc := range cases {
		account := &Account{Platform: tc.platform, Type: AccountTypeAPIKey, Credentials: map[string]any{
			"api_protocol": APIProtocolAdaptive,
			"account_mode": tc.mode,
		}}
		require.Equal(t, tc.chat, account.GetCNProtocolBaseURL(APIProtocolChatCompletions))
		require.Equal(t, tc.anthropic, account.GetCNProtocolBaseURL(APIProtocolAnthropic))
	}
}

func TestCNProviderSyncProtocolBaseURLOverrides(t *testing.T) {
	adaptive := &Account{Platform: PlatformKimi, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_protocol": APIProtocolAdaptive,
		"base_url":     "https://legacy-chat.example/v1",
		"api_base_urls": map[string]any{
			APIProtocolChatCompletions: "https://adaptive-chat.example/v1",
			APIProtocolAnthropic:       "https://adaptive-anthropic.example",
			APIProtocolResponses:       "https://adaptive-responses.example/v1",
		},
	}}
	require.Equal(t, "https://adaptive-chat.example/v1", adaptive.GetOpenAIBaseURL())
	require.Equal(t, "https://adaptive-chat.example/v1", adaptive.GetCNProtocolBaseURL(APIProtocolChatCompletions))
	require.Equal(t, "https://adaptive-anthropic.example", adaptive.GetAnthropicProtocolBaseURL())
	require.Equal(t, "https://adaptive-responses.example/v1", adaptive.GetCNProtocolBaseURL(APIProtocolResponses))

	fixedAnthropic := &Account{Platform: PlatformKimi, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_protocol": APIProtocolAnthropic,
		"base_url":     "https://fixed-anthropic.example",
		"api_base_urls": map[string]any{
			APIProtocolAnthropic: "https://ignored-adaptive.example",
		},
	}}
	require.Equal(t, "https://fixed-anthropic.example", fixedAnthropic.GetAnthropicProtocolBaseURL())
	require.Equal(t, DefaultKimiPayGBaseURL, fixedAnthropic.GetOpenAIFormatBaseURL())
	require.Equal(t, DefaultKimiPayGBaseURL, fixedAnthropic.GetCNProtocolBaseURL(APIProtocolChatCompletions))
}

func TestCNProviderSyncResponsesURLAndStatelessBody(t *testing.T) {
	require.Equal(t, "https://api.deepseek.com/responses", buildOpenAIResponsesURLForPlatform(PlatformDeepseek, DefaultDeepseekBaseURL))
	require.Equal(t, "https://api.kimi.com/coding/v1/responses", buildOpenAIResponsesURLForPlatform(PlatformKimi, DefaultKimiCodingBaseURL))

	account := &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_protocol": APIProtocolResponses,
	}}
	body := []byte(`{"model":"deepseek-v4-pro","store":true,"previous_response_id":"resp_1","input":"hi"}`)
	normalized := normalizeNativeCNResponsesRequestBody(account, body)
	require.False(t, gjson.GetBytes(normalized, "store").Bool())
	require.False(t, gjson.GetBytes(normalized, "previous_response_id").Exists())

	mediaBody := []byte(`{"model":"deepseek-v4-pro","store":true,"input":[{"type":"function_call","call_id":"call_image","name":"view_image","arguments":"{}"},{"type":"function_call_output","call_id":"call_image","output":[{"type":"input_image","image_url":"data:image/png;base64,AQID"}]}]}`)
	mediaNormalized := normalizeNativeCNResponsesRequestBody(account, mediaBody)
	require.Equal(t, gjson.String, gjson.GetBytes(mediaNormalized, "input.1.output").Type)
	require.NotContains(t, gjson.GetBytes(mediaNormalized, "input.1.output").String(), "data:image/png")
	require.Equal(t, "message", gjson.GetBytes(mediaNormalized, "input.2.type").String())
	require.Equal(t, "user", gjson.GetBytes(mediaNormalized, "input.2.role").String())
	require.Equal(t, "data:image/png;base64,AQID", gjson.GetBytes(mediaNormalized, "input.2.content.1.image_url").String())

	cc := &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey}
	require.Equal(t, string(body), string(normalizeNativeCNResponsesRequestBody(cc, body)))
}

func TestCNProviderSyncQuotaParsers(t *testing.T) {
	kimi := parseKimiUsageTiers([]byte(`{
		"limits":[{"detail":{"limit":1000,"remaining":600,"resetTime":"2026-09-16T10:00:00Z"}}],
		"usage":{"limit":10000,"remaining":4000,"resetTime":"2026-09-20T00:00:00Z"}
	}`))
	require.Len(t, kimi, 2)
	require.InDelta(t, 40, kimi[0].UsedPercent, 1e-9)
	require.InDelta(t, 60, kimi[1].UsedPercent, 1e-9)

	openCode := parseOpenCodeGoUsageTiers([]byte(`{"usage":{"rolling":{"percent":12.5},"weekly":{"percent":40},"monthly":{"percent":22.2}}}`))
	require.Len(t, openCode, 3)
	require.Equal(t, "monthly", openCode[2].Window)

	updates := cnQuotaExtraUpdates(PlatformOpenCodeGo, openCode, time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC))
	require.Equal(t, 22.2, updates["opencode_go_monthly_used_percent"])
}

func TestCNProviderSyncBalanceURLs(t *testing.T) {
	require.Equal(t, "https://api.moonshot.cn/v1/users/me/balance", cnBalanceURL(&Account{Platform: PlatformKimi}))
	require.Empty(t, cnBalanceURL(&Account{Platform: PlatformKimi, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"base_url": "https://relay.example/v1",
	}}))
	require.Empty(t, cnBalanceURL(&Account{Platform: PlatformKimi, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_protocol": APIProtocolAdaptive,
		"base_url":     DefaultKimiPayGBaseURL,
		"api_base_urls": map[string]any{
			APIProtocolChatCompletions: "https://relay.example/v1",
		},
	}}))
	require.Equal(t, "https://api.deepseek.com/user/balance", cnBalanceURL(&Account{Platform: PlatformDeepseek}))
	require.Empty(t, cnBalanceURL(&Account{Platform: PlatformZhipu}))
}

func TestCNProviderProbesUseAccountScopedEgressRoute(t *testing.T) {
	newAccount := func(mode string) *Account {
		return &Account{
			ID:          42,
			Platform:    PlatformKimi,
			Type:        AccountTypeAPIKey,
			Concurrency: 3,
			Credentials: map[string]any{
				"api_key":      "secret",
				"account_mode": mode,
			},
			EgressMode: platformegress.ModeIPv6Pool,
			EgressBinding: &moduleegress.Binding{
				PoolID:     7,
				SourceIPv6: "2001:db8:7::42",
				Status:     moduleegress.BindingStatusActive,
				PoolStatus: moduleegress.PoolStatusActive,
				Version:    2,
			},
		}
	}

	t.Run("quota", func(t *testing.T) {
		repo := &cnProbeAccountRepo{}
		upstream := &cnProbeRoutedUpstream{body: `{"limits":[{"detail":{"limit":100,"remaining":80}}]}`}
		service := NewCNProviderQuotaService(repo, nil, upstream, nil)
		result, err := service.QueryUsageForAccount(context.Background(), newAccount(AccountModeCoding))
		require.NoError(t, err)
		require.True(t, result.Success)
		require.Equal(t, 1, upstream.routedCalls)
		require.Zero(t, upstream.legacyCalls)
		require.Equal(t, platformegress.ModeIPv6Pool, upstream.route.Mode)
		require.Equal(t, "2001:db8:7::42", upstream.route.SourceIPv6)
	})

	t.Run("balance", func(t *testing.T) {
		repo := &cnProbeAccountRepo{}
		upstream := &cnProbeRoutedUpstream{body: `{"code":0,"data":{"available_balance":12.5}}`}
		service := NewCNProviderBalanceService(repo, nil, upstream, nil)
		result, err := service.QueryBalanceForAccount(context.Background(), newAccount(AccountModePayG))
		require.NoError(t, err)
		require.True(t, result.Success)
		require.Equal(t, 1, upstream.routedCalls)
		require.Zero(t, upstream.legacyCalls)
		require.Equal(t, platformegress.ModeIPv6Pool, upstream.route.Mode)
		require.Equal(t, "2001:db8:7::42", upstream.route.SourceIPv6)
	})
}

func TestCNProviderProbeSingleflightCollapsesConcurrentCalls(t *testing.T) {
	account := func(mode string) *Account {
		return &Account{ID: 77, Platform: PlatformKimi, Type: AccountTypeAPIKey, Credentials: map[string]any{
			"api_key": "secret", "account_mode": mode,
		}}
	}

	tests := []struct {
		name string
		body string
	}{
		{
			name: "quota",
			body: `{"limits":[{"detail":{"limit":100,"remaining":80}}]}`,
		},
		{
			name: "balance",
			body: `{"code":0,"data":{"available_balance":12.5}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			upstream := newBlockingCNProbeUpstream(tc.body)
			var run func(context.Context) error
			if tc.name == "quota" {
				service := NewCNProviderQuotaService(&cnProbeAccountRepo{}, nil, upstream, nil)
				run = func(ctx context.Context) error {
					_, err := service.QueryUsageForAccount(ctx, account(AccountModeCoding))
					return err
				}
			} else {
				service := NewCNProviderBalanceService(&cnProbeAccountRepo{}, nil, upstream, nil)
				run = func(ctx context.Context) error {
					_, err := service.QueryBalanceForAccount(ctx, account(AccountModePayG))
					return err
				}
			}
			result := make(chan error, 1)
			go func() { result <- run(context.Background()) }()
			<-upstream.entered

			const followers = 16
			var joined sync.WaitGroup
			followerErrors := make(chan error, followers)
			joined.Add(followers)
			for range followers {
				go func() {
					defer joined.Done()
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					followerErrors <- run(ctx)
				}()
			}
			joined.Wait()
			close(followerErrors)
			for err := range followerErrors {
				require.ErrorIs(t, err, context.Canceled)
			}
			require.EqualValues(t, 1, upstream.calls.Load())

			close(upstream.release)
			require.NoError(t, <-result)
			require.EqualValues(t, 1, upstream.calls.Load())
		})
	}
}
