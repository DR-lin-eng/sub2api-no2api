package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIRequestTimezoneRuntimePersistsAndChangesWithoutRestart(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{Gateway: config.GatewayConfig{OpenAIRequestTimezone: "Asia/Singapore"}}
	repo := &gatewayTTLSettingRepo{data: map[string]string{}}
	settings := NewSettingService(repo, cfg)
	gateway := &OpenAIGatewayService{cfg: cfg, settingService: settings}
	account := &Account{Platform: PlatformOpenAI}
	body := []byte(`{"input":[{"role":"user","content":"<environment_context><timezone>Asia/Shanghai</timezone></environment_context>"}],"tools":[{"type":"web_search","user_location":{"timezone":"Asia/Shanghai"}}]}`)
	for _, transport := range []string{"http", "ws"} {
		out := gateway.normalizeOpenAIRequestTimezone(ctx, account, body, transport)
		require.Equal(t, "Asia/Singapore", gjson.GetBytes(out, "tools.0.user_location.timezone").String())
	}

	updated := settings.parseSettings(repo.data)
	updated.OpenAIRequestTimezone = "  Asia/Tokyo  "
	require.NoError(t, settings.UpdateSettings(ctx, updated))
	require.Equal(t, "Asia/Tokyo", repo.data[SettingKeyOpenAIRequestTimezone])
	for _, transport := range []string{"http", "ws"} {
		out := gateway.normalizeOpenAIRequestTimezone(ctx, account, body, transport)
		require.Contains(t, gjson.GetBytes(out, "input.0.content").String(), "<timezone>Asia/Tokyo</timezone>")
		require.Equal(t, "Asia/Tokyo", gjson.GetBytes(out, "tools.0.user_location.timezone").String())
	}
	// A separate process/service reads the persisted override instead of the env.
	enabled, name := NewSettingService(repo, cfg).GetOpenAIRequestTimezoneSettings(ctx)
	require.True(t, enabled)
	require.Equal(t, "Asia/Tokyo", name)

	updated.OpenAIRequestTimezoneEnabled = false
	require.NoError(t, settings.UpdateSettings(ctx, updated))
	for _, transport := range []string{"http", "ws"} {
		require.Equal(t, body, gateway.normalizeOpenAIRequestTimezone(ctx, account, body, transport))
	}
	updated.OpenAIRequestTimezoneEnabled = true
	updated.OpenAIRequestTimezone = ""
	require.NoError(t, settings.UpdateSettings(ctx, updated))
	enabled, name = settings.GetOpenAIRequestTimezoneSettings(ctx)
	require.True(t, enabled)
	require.Equal(t, "Asia/Singapore", name)
}

func TestOpenAIRequestTimezoneRejectsInvalidNameBeforeWriting(t *testing.T) {
	repo := &gatewayTTLSettingRepo{data: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})
	for _, name := range []string{"Invalid/Timezone", "Local", "<timezone>UTC</timezone>"} {
		settings := svc.parseSettings(repo.data)
		settings.OpenAIRequestTimezone = name
		require.ErrorContains(t, svc.UpdateSettings(context.Background(), settings), "IANA")
		require.Empty(t, repo.data)
	}
}

type timezoneReadRepo struct {
	*gatewayTTLSettingRepo
	reads int
	err   error
}

func (r *timezoneReadRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	r.reads++
	if r.err != nil {
		return nil, r.err
	}
	return r.gatewayTTLSettingRepo.GetMultiple(ctx, keys)
}

func TestOpenAIRequestTimezoneCacheRetainsLastKnownStateOnReadFailure(t *testing.T) {
	repo := &timezoneReadRepo{gatewayTTLSettingRepo: &gatewayTTLSettingRepo{data: map[string]string{
		SettingKeyOpenAIRequestTimezoneEnabled: "false",
		SettingKeyOpenAIRequestTimezone:        "Asia/Tokyo",
	}}}
	svc := NewSettingService(repo, &config.Config{})
	for i := 0; i < 3; i++ {
		enabled, name := svc.GetOpenAIRequestTimezoneSettings(context.Background())
		require.False(t, enabled)
		require.Equal(t, "Asia/Tokyo", name)
	}
	require.Equal(t, 1, repo.reads)
	svc.openAIRequestTimezoneCache.Store(&cachedOpenAIRequestTimezoneSettings{
		enabled: false, timezone: "Asia/Tokyo", expiresAt: time.Now().Add(-time.Second),
	})
	repo.err = errors.New("database unavailable")
	enabled, name := svc.GetOpenAIRequestTimezoneSettings(context.Background())
	require.False(t, enabled)
	require.Equal(t, "Asia/Tokyo", name)
	require.Equal(t, 2, repo.reads)
}
