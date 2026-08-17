package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

type codexSimulationSettingRepo struct {
	mu     sync.Mutex
	values map[string]string
	gets   int
}

func newCodexSimulationSettingRepo() *codexSimulationSettingRepo {
	return &codexSimulationSettingRepo{values: make(map[string]string)}
}

func (r *codexSimulationSettingRepo) Get(_ context.Context, key string) (*Setting, error) {
	value, err := r.GetValue(context.Background(), key)
	if err != nil {
		return nil, err
	}
	return &Setting{Key: key, Value: value}, nil
}

func (r *codexSimulationSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gets++
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *codexSimulationSettingRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	r.values[key] = value
	r.mu.Unlock()
	return nil
}

func (r *codexSimulationSettingRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, err := r.GetValue(ctx, key); err == nil {
			result[key] = value
		}
	}
	return result, nil
}

func (r *codexSimulationSettingRepo) SetMultiple(ctx context.Context, values map[string]string) error {
	for key, value := range values {
		if err := r.Set(ctx, key, value); err != nil {
			return err
		}
	}
	return nil
}

func (r *codexSimulationSettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make(map[string]string, len(r.values))
	for key, value := range r.values {
		result[key] = value
	}
	return result, nil
}

func (r *codexSimulationSettingRepo) Delete(_ context.Context, key string) error {
	r.mu.Lock()
	delete(r.values, key)
	r.mu.Unlock()
	return nil
}

func (r *codexSimulationSettingRepo) raw(key string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.values[key]
}

func (r *codexSimulationSettingRepo) getCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.gets
}

func TestCodexSimulationSettingsFallBackToYAMLWhenDBRowIsMissing(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.CodexSimulation = config.GatewayCodexSimulationConfig{
		FullSimulationEnabled: true,
		ContinuationMode:      "shadow",
		StateTTLSeconds:       3600,
		IdentitySecret:        codexSimulationTestSecret,
	}
	svc := NewSettingService(newCodexSimulationSettingRepo(), cfg)

	settings, err := svc.GetCodexSimulationSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.FullSimulationEnabled)
	require.Equal(t, "shadow", settings.ContinuationMode)
	require.Equal(t, 3600, settings.StateTTLSeconds)
	require.Equal(t, codexSimulationTestSecret, settings.IdentitySecret)
}

func TestCodexSimulationSettingsPersistedOffOverridesYAMLOn(t *testing.T) {
	repo := newCodexSimulationSettingRepo()
	require.NoError(t, repo.Set(context.Background(), SettingKeyCodexSimulationSettings,
		`{"full_simulation_enabled":false,"continuation_mode":"off","state_ttl_seconds":604800,"identity_secret":""}`))
	cfg := &config.Config{}
	cfg.Gateway.CodexSimulation = config.GatewayCodexSimulationConfig{
		FullSimulationEnabled: true,
		ContinuationMode:      "enforce",
		StateTTLSeconds:       3600,
		IdentitySecret:        codexSimulationTestSecret,
	}
	svc := NewSettingService(repo, cfg)

	settings, err := svc.GetCodexSimulationSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.FullSimulationEnabled)
	require.Equal(t, "off", settings.ContinuationMode)
	require.Empty(t, settings.IdentitySecret)
}

func TestForceDisableCodexSimulationSettingsRepairsMalformedPersistedValue(t *testing.T) {
	repo := newCodexSimulationSettingRepo()
	require.NoError(t, repo.Set(context.Background(), SettingKeyCodexSimulationSettings, `{malformed`))
	cfg := &config.Config{}
	cfg.Gateway.CodexSimulation = config.GatewayCodexSimulationConfig{
		FullSimulationEnabled: true,
		ContinuationMode:      "enforce",
		StateTTLSeconds:       3600,
		IdentitySecret:        codexSimulationTestSecret,
	}
	svc := NewSettingService(repo, cfg)

	disabled, err := svc.ForceDisableCodexSimulationSettings(context.Background())
	require.NoError(t, err)
	require.False(t, disabled.FullSimulationEnabled)
	require.Equal(t, "off", disabled.ContinuationMode)
	require.Equal(t, 3600, disabled.StateTTLSeconds)
	require.Equal(t, codexSimulationTestSecret, disabled.IdentitySecret)

	var persisted CodexSimulationSettings
	require.NoError(t, json.Unmarshal([]byte(repo.raw(SettingKeyCodexSimulationSettings)), &persisted))
	require.Equal(t, *disabled, persisted)

	loaded, err := svc.GetCodexSimulationSettings(context.Background())
	require.NoError(t, err)
	require.False(t, loaded.FullSimulationEnabled)
	require.Equal(t, "off", loaded.ContinuationMode)
}

func TestCodexSimulationSettingsGenerateAndPreserveIdentitySecret(t *testing.T) {
	repo := newCodexSimulationSettingRepo()
	svc := NewSettingService(repo, &config.Config{})
	ctx := context.Background()

	enabled, err := svc.SetCodexSimulationSettings(ctx, &CodexSimulationSettings{
		FullSimulationEnabled: true,
		ContinuationMode:      "enforce",
		StateTTLSeconds:       604800,
	})
	require.NoError(t, err)
	require.True(t, enabled.IdentitySecretConfigured())
	generatedSecret := enabled.IdentitySecret
	require.NotEmpty(t, generatedSecret)

	disabled, err := svc.SetCodexSimulationSettings(ctx, &CodexSimulationSettings{
		FullSimulationEnabled: false,
		ContinuationMode:      "off",
		StateTTLSeconds:       604800,
	})
	require.NoError(t, err)
	require.Equal(t, generatedSecret, disabled.IdentitySecret)

	reenabled, err := svc.SetCodexSimulationSettings(ctx, &CodexSimulationSettings{
		FullSimulationEnabled: true,
		ContinuationMode:      "shadow",
		StateTTLSeconds:       604800,
	})
	require.NoError(t, err)
	require.Equal(t, generatedSecret, reenabled.IdentitySecret)

	var persisted CodexSimulationSettings
	require.NoError(t, json.Unmarshal([]byte(repo.raw(SettingKeyCodexSimulationSettings)), &persisted))
	require.Equal(t, generatedSecret, persisted.IdentitySecret)
}

func TestCodexSimulationSettingsPreserveYAMLSecretWhenDBOverrideHasNone(t *testing.T) {
	repo := newCodexSimulationSettingRepo()
	require.NoError(t, repo.Set(context.Background(), SettingKeyCodexSimulationSettings,
		`{"full_simulation_enabled":false,"continuation_mode":"off","state_ttl_seconds":604800,"identity_secret":""}`))
	cfg := &config.Config{}
	cfg.Gateway.CodexSimulation.IdentitySecret = codexSimulationTestSecret
	svc := NewSettingService(repo, cfg)

	settings, err := svc.SetCodexSimulationSettings(context.Background(), &CodexSimulationSettings{
		FullSimulationEnabled: true,
		ContinuationMode:      "shadow",
		StateTTLSeconds:       604800,
	})
	require.NoError(t, err)
	require.Equal(t, codexSimulationTestSecret, settings.IdentitySecret)
}

func TestCodexSimulationSettingsRejectInvalidModeAndTTL(t *testing.T) {
	for _, test := range []struct {
		name     string
		settings CodexSimulationSettings
	}{
		{name: "mode", settings: CodexSimulationSettings{ContinuationMode: "enabled", StateTTLSeconds: 60}},
		{name: "ttl", settings: CodexSimulationSettings{ContinuationMode: "off", StateTTLSeconds: 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newCodexSimulationSettingRepo()
			svc := NewSettingService(repo, &config.Config{})
			_, err := svc.SetCodexSimulationSettings(context.Background(), &test.settings)
			require.Error(t, err)
			require.Empty(t, repo.raw(SettingKeyCodexSimulationSettings))
		})
	}
}

func TestCodexSimulationSettingsPublishImmediatelyAndRefreshAcrossInstances(t *testing.T) {
	repo := newCodexSimulationSettingRepo()
	first := NewSettingService(repo, &config.Config{})
	second := NewSettingService(repo, &config.Config{})
	ctx := context.Background()
	require.NoError(t, first.LoadCodexSimulationSettings(ctx))
	require.NoError(t, second.LoadCodexSimulationSettings(ctx))

	updated, err := first.SetCodexSimulationSettings(ctx, &CodexSimulationSettings{
		FullSimulationEnabled: true,
		ContinuationMode:      "shadow",
		StateTTLSeconds:       120,
	})
	require.NoError(t, err)
	require.True(t, first.CodexSimulationSettingsSnapshot(ctx).FullSimulationEnabled)
	require.False(t, second.CodexSimulationSettingsSnapshot(ctx).FullSimulationEnabled)

	syncCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	second.startCodexSimulationSettingsSync(syncCtx, 5*time.Millisecond)
	require.Eventually(t, func() bool {
		return second.CodexSimulationSettingsSnapshot(ctx) == *updated
	}, time.Second, 5*time.Millisecond)
}

func TestCodexSimulationSettingsSnapshotNeverReadsDatabase(t *testing.T) {
	repo := newCodexSimulationSettingRepo()
	svc := NewSettingService(repo, &config.Config{})
	before := repo.getCalls()

	for range 10 {
		settings := svc.CodexSimulationSettingsSnapshot(context.Background())
		require.False(t, settings.FullSimulationEnabled)
		require.Equal(t, "off", settings.ContinuationMode)
	}

	require.Equal(t, before, repo.getCalls())
}

func TestCodexSimulationRequestKeepsStartingSnapshotAndDBOffRestoresNoOp(t *testing.T) {
	repo := newCodexSimulationSettingRepo()
	settingsService := NewSettingService(repo, &config.Config{})
	ctx := context.Background()
	_, err := settingsService.SetCodexSimulationSettings(ctx, &CodexSimulationSettings{
		FullSimulationEnabled: true,
		ContinuationMode:      "enforce",
		StateTTLSeconds:       120,
	})
	require.NoError(t, err)

	gateway := &OpenAIGatewayService{
		cfg:                &config.Config{},
		settingService:     settingsService,
		openaiWSStateStore: NewOpenAIWSStateStore(nil),
	}
	body := []byte(`{"model":"gpt-5.4","input":"hello"}`)
	account := openAIFingerprintAccount(71, map[string]any{codexFingerprintModeExtraKey: "full"})
	account.Credentials = map[string]any{"chatgpt_account_id": "snapshot-principal"}
	c := newCodexSimulationTestContext("/v1/responses")
	c.Request.Header.Set("thread-id", "snapshot-thread")
	gateway.PrepareCodexSimulationRequest(c, 1, nil, body)
	request, ok := codexSimulationRequestStateFromGin(c)
	require.True(t, ok)
	startingSecret := request.settings.IdentitySecret

	_, err = settingsService.SetCodexSimulationSettings(ctx, &CodexSimulationSettings{
		FullSimulationEnabled: false,
		ContinuationMode:      "off",
		StateTTLSeconds:       300,
	})
	require.NoError(t, err)
	_, err = gateway.PrepareCodexSimulationAttempt(ctx, c, account, body)
	require.NoError(t, err)
	attempt, ok := codexSimulationAttemptFromGin(c)
	require.True(t, ok)
	require.NotNil(t, attempt.fingerprint)
	require.Equal(t, startingSecret, attempt.request.settings.IdentitySecret)
	require.Equal(t, 120, attempt.request.settings.StateTTLSeconds)

	newRequest := newCodexSimulationTestContext("/v1/responses")
	gateway.PrepareCodexSimulationRequest(newRequest, 1, nil, body)
	_, ok = codexSimulationRequestStateFromGin(newRequest)
	require.False(t, ok)
	unchanged, err := gateway.PrepareCodexSimulationAttempt(ctx, newRequest, account, body)
	require.NoError(t, err)
	require.Equal(t, body, unchanged)
	_, ok = codexSimulationAttemptFromGin(newRequest)
	require.False(t, ok)
}
