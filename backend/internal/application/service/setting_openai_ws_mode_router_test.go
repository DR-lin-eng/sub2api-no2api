package service

import (
	"context"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

type openAIWSModeRouterSettingRepo struct {
	values map[string]string
}

func (r *openAIWSModeRouterSettingRepo) Get(_ context.Context, key string) (*Setting, error) {
	value, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value}, nil
}

func (r *openAIWSModeRouterSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *openAIWSModeRouterSettingRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

func (r *openAIWSModeRouterSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (r *openAIWSModeRouterSettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	for key, value := range settings {
		r.values[key] = value
	}
	return nil
}

func (r *openAIWSModeRouterSettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	result := make(map[string]string, len(r.values))
	for key, value := range r.values {
		result[key] = value
	}
	return result, nil
}

func (r *openAIWSModeRouterSettingRepo) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

func TestSettingServiceOpenAIWSModeRouterV2FallsBackToConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	repo := &openAIWSModeRouterSettingRepo{values: map[string]string{}}
	svc := NewSettingService(repo, cfg)

	require.NoError(t, svc.LoadOpenAIWSModeRouterV2Setting(context.Background()))
	require.True(t, svc.IsOpenAIWSModeRouterV2Enabled(context.Background()))
	require.True(t, svc.parseSettings(map[string]string{}).OpenAIWSModeRouterV2Enabled)
}

func TestSettingServiceOpenAIWSModeRouterV2RuntimeOverride(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool
	repo := &openAIWSModeRouterSettingRepo{values: map[string]string{
		SettingKeyOpenAIWSModeRouterV2Enabled: "false",
	}}
	settings := NewSettingService(repo, cfg)
	require.NoError(t, settings.LoadOpenAIWSModeRouterV2Setting(context.Background()))
	require.False(t, settings.IsOpenAIWSModeRouterV2Enabled(context.Background()))

	resolver := newOpenAIWSProtocolResolver(cfg, func() bool {
		return settings.IsOpenAIWSModeRouterV2Enabled(context.Background())
	})
	account := &Account{
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Extra: map[string]any{
			"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModeCtxPool,
		},
	}
	require.Equal(t, OpenAIUpstreamTransportHTTPSSE, resolver.Resolve(account).Transport)

	updated := settings.parseSettings(repo.values)
	updated.OpenAIWSModeRouterV2Enabled = true
	require.NoError(t, settings.UpdateSettings(context.Background(), updated))
	require.Equal(t, "true", repo.values[SettingKeyOpenAIWSModeRouterV2Enabled])
	require.True(t, settings.IsOpenAIWSModeRouterV2Enabled(context.Background()))
	require.Equal(t, OpenAIUpstreamTransportResponsesWebsocketV2, resolver.Resolve(account).Transport)
}

var benchmarkOpenAIWSModeRouterV2Enabled bool

func BenchmarkSettingServiceIsOpenAIWSModeRouterV2Enabled(b *testing.B) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	settings := NewSettingService(&openAIWSModeRouterSettingRepo{values: map[string]string{}}, cfg)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		benchmarkOpenAIWSModeRouterV2Enabled = settings.IsOpenAIWSModeRouterV2Enabled(ctx)
	}
}
