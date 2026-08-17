package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

type cloudflareSettingsRepoStub struct {
	mu     sync.Mutex
	values map[string]string
}

func newCloudflareSettingsRepoStub() *cloudflareSettingsRepoStub {
	return &cloudflareSettingsRepoStub{values: make(map[string]string)}
}

func (r *cloudflareSettingsRepoStub) Get(_ context.Context, key string) (*Setting, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value}, nil
}

func (r *cloudflareSettingsRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	setting, err := r.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}

func (r *cloudflareSettingsRepoStub) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[key] = value
	return nil
}

func (r *cloudflareSettingsRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (r *cloudflareSettingsRepoStub) SetMultiple(ctx context.Context, values map[string]string) error {
	for key, value := range values {
		if err := r.Set(ctx, key, value); err != nil {
			return err
		}
	}
	return nil
}

func (r *cloudflareSettingsRepoStub) GetAll(_ context.Context) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make(map[string]string, len(r.values))
	for key, value := range r.values {
		result[key] = value
	}
	return result, nil
}

func (r *cloudflareSettingsRepoStub) Delete(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.values, key)
	return nil
}

type cloudflareSettingsEncryptorStub struct{}

func (cloudflareSettingsEncryptorStub) Encrypt(value string) (string, error) {
	return "cipher:" + base64.StdEncoding.EncodeToString([]byte(value)), nil
}

func (cloudflareSettingsEncryptorStub) Decrypt(value string) (string, error) {
	const prefix = "cipher:"
	if len(value) <= len(prefix) || value[:len(prefix)] != prefix {
		return "", errors.New("invalid ciphertext")
	}
	decoded, err := base64.StdEncoding.DecodeString(value[len(prefix):])
	return string(decoded), err
}

type cloudflareSettingsEdgeStub struct {
	health        InvalidAuthEdgeHealth
	validated     []CloudflareIngressSettings
	applied       []CloudflareIngressSettings
	validationErr error
}

func (s *cloudflareSettingsEdgeStub) EnqueueBlock(string, time.Time) bool { return false }
func (s *cloudflareSettingsEdgeStub) Health() InvalidAuthEdgeHealth       { return s.health }
func (s *cloudflareSettingsEdgeStub) Stop()                               {}
func (s *cloudflareSettingsEdgeStub) ValidateCloudflareIngressSettings(_ context.Context, settings CloudflareIngressSettings) error {
	s.validated = append(s.validated, settings)
	return s.validationErr
}
func (s *cloudflareSettingsEdgeStub) ApplyCloudflareIngressSettings(_ context.Context, settings CloudflareIngressSettings) error {
	s.applied = append(s.applied, settings)
	return nil
}

func enabledCloudflareSettingsConfig() *config.Config {
	return &config.Config{APIKeyAuth: config.APIKeyAuthCacheConfig{
		InvalidAbuse: config.InvalidAuthAbuseConfig{Enabled: true},
	}}
}

func validCloudflareSettingsInput() UpdateCloudflareIngressSettingsInput {
	return UpdateCloudflareIngressSettingsInput{
		Enabled:                  true,
		Mode:                     CloudflareIngressModeZoneAccessRules,
		ZoneID:                   "0123456789abcdef0123456789abcdef",
		APIToken:                 "cloudflare-secret-token",
		RequestTimeoutSeconds:    6,
		QueueCapacity:            2048,
		MaxActiveRules:           1500,
		ReconcileIntervalSeconds: 180,
	}
}

func TestCloudflareIngressSettingServiceEncryptsAndMasksToken(t *testing.T) {
	repo := newCloudflareSettingsRepoStub()
	edge := &cloudflareSettingsEdgeStub{}
	encryptor := cloudflareSettingsEncryptorStub{}
	svc := NewCloudflareIngressSettingService(repo, encryptor, edge, enabledCloudflareSettingsConfig())

	view, err := svc.Update(t.Context(), validCloudflareSettingsInput())
	require.NoError(t, err)
	require.True(t, view.Enabled)
	require.True(t, view.APITokenConfigured)
	require.Equal(t, "0123456789abcdef0123456789abcdef", view.ZoneID)
	require.Equal(t, CloudflareIngressModeZoneAccessRules, view.Mode)
	require.Len(t, edge.validated, 1)
	require.Len(t, edge.applied, 1)
	require.Equal(t, "cloudflare-secret-token", edge.applied[0].APIToken)

	raw, err := repo.GetValue(t.Context(), SettingKeyCloudflareIngressSettings)
	require.NoError(t, err)
	require.NotContains(t, raw, "cloudflare-secret-token")
	require.Contains(t, raw, "api_token_ciphertext")
	encodedView, err := json.Marshal(view)
	require.NoError(t, err)
	require.NotContains(t, string(encodedView), "token\"")
	require.NotContains(t, string(encodedView), "cipher")

	loaded, err := LoadPersistedCloudflareIngressSettings(t.Context(), repo, encryptor)
	require.NoError(t, err)
	require.Equal(t, "cloudflare-secret-token", loaded.APIToken)

	update := validCloudflareSettingsInput()
	update.Enabled = false
	update.APIToken = ""
	view, err = svc.Update(t.Context(), update)
	require.NoError(t, err)
	require.False(t, view.Enabled)
	require.True(t, view.APITokenConfigured)
	require.Len(t, edge.validated, 1, "disabled updates do not call the remote API")
	require.Equal(t, "cloudflare-secret-token", edge.applied[1].APIToken)
}

func TestCloudflareIngressSettingServicePersistsWAFMode(t *testing.T) {
	repo := newCloudflareSettingsRepoStub()
	edge := &cloudflareSettingsEdgeStub{}
	svc := NewCloudflareIngressSettingService(repo, cloudflareSettingsEncryptorStub{}, edge, enabledCloudflareSettingsConfig())

	input := validCloudflareSettingsInput()
	input.Mode = CloudflareIngressModeWAFCustomRules
	input.WAFHostnames = []string{"Edge.Example.COM.", "API.Example.COM.", "edge.example.com"}
	input.WAFRuleIDs = []string{
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}
	input.WAFSyncIntervalSeconds = 20
	input.AnalyticsIntervalSeconds = 600
	view, err := svc.Update(t.Context(), input)
	require.NoError(t, err)
	require.Equal(t, CloudflareIngressModeWAFCustomRules, view.Mode)
	require.Equal(t, "api.example.com", view.WAFHostname)
	require.Equal(t, []string{"api.example.com", "edge.example.com"}, view.WAFHostnames)
	require.Equal(t, []string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}, view.WAFRuleIDs)
	require.Equal(t, 20, view.WAFSyncIntervalSeconds)
	require.Equal(t, 600, view.AnalyticsIntervalSeconds)
	require.Equal(t, "api.example.com", edge.validated[0].WAFHostname)
	require.Equal(t, view.WAFHostnames, edge.validated[0].WAFHostnames)

	loaded, err := LoadPersistedCloudflareIngressSettings(t.Context(), repo, cloudflareSettingsEncryptorStub{})
	require.NoError(t, err)
	require.Equal(t, CloudflareIngressModeWAFCustomRules, loaded.Mode)
	require.Equal(t, view.WAFHostnames, loaded.WAFHostnames)
	require.Equal(t, view.WAFRuleIDs, loaded.WAFRuleIDs)

	legacyUpdate := input
	legacyUpdate.Enabled = false
	legacyUpdate.APIToken = ""
	legacyUpdate.WAFHostnames = nil
	legacyUpdate.WAFHostname = view.WAFHostname
	legacyView, err := svc.Update(t.Context(), legacyUpdate)
	require.NoError(t, err)
	require.Equal(t, view.WAFHostnames, legacyView.WAFHostnames)
}

func TestLoadPersistedCloudflareIngressSettingsMigratesLegacyHostname(t *testing.T) {
	repo := newCloudflareSettingsRepoStub()
	record := persistedCloudflareIngressSettings{
		Version: 2, Mode: CloudflareIngressModeWAFCustomRules, WAFHostname: "Legacy.Example.COM.",
	}
	raw, err := json.Marshal(record)
	require.NoError(t, err)
	require.NoError(t, repo.Set(t.Context(), SettingKeyCloudflareIngressSettings, string(raw)))

	loaded, err := LoadPersistedCloudflareIngressSettings(t.Context(), repo, cloudflareSettingsEncryptorStub{})
	require.NoError(t, err)
	require.Equal(t, "legacy.example.com", loaded.WAFHostname)
	require.Equal(t, []string{"legacy.example.com"}, loaded.WAFHostnames)
}

func TestCloudflareIngressSettingServiceRequiresDisableBeforeCredentialChange(t *testing.T) {
	repo := newCloudflareSettingsRepoStub()
	edge := &cloudflareSettingsEdgeStub{}
	svc := NewCloudflareIngressSettingService(repo, cloudflareSettingsEncryptorStub{}, edge, enabledCloudflareSettingsConfig())
	require.NoError(t, func() error {
		_, err := svc.Update(t.Context(), validCloudflareSettingsInput())
		return err
	}())

	update := validCloudflareSettingsInput()
	update.Enabled = false
	update.APIToken = "replacement-token"
	_, err := svc.Update(t.Context(), update)
	require.ErrorIs(t, err, ErrCloudflareIngressCredentialsBusy)
}

func TestCloudflareIngressSettingServiceKeepsWAFBindingLockedUntilRemoteExpressionsAreEmpty(t *testing.T) {
	repo := newCloudflareSettingsRepoStub()
	edge := &cloudflareSettingsEdgeStub{}
	svc := NewCloudflareIngressSettingService(repo, cloudflareSettingsEncryptorStub{}, edge, enabledCloudflareSettingsConfig())
	input := validCloudflareSettingsInput()
	input.Mode = CloudflareIngressModeWAFCustomRules
	input.WAFHostnames = []string{"api.example.com", "edge.example.com"}
	input.WAFRuleIDs = []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	require.NoError(t, func() error {
		_, err := svc.Update(t.Context(), input)
		return err
	}())

	disable := input
	disable.Enabled = false
	require.NoError(t, func() error {
		_, err := svc.Update(t.Context(), disable)
		return err
	}())
	edge.health.WAF = &InvalidAuthWAFHealth{SyncedEntries: 1}
	replacement := disable
	replacement.WAFHostnames = []string{"new-api.example.com"}
	_, err := svc.Update(t.Context(), replacement)
	require.ErrorIs(t, err, ErrCloudflareIngressCredentialsBusy)
}

func TestCloudflareIngressSettingServiceDefaultsAndValidation(t *testing.T) {
	repo := newCloudflareSettingsRepoStub()
	edge := &cloudflareSettingsEdgeStub{}
	encryptor := cloudflareSettingsEncryptorStub{}

	settings, err := LoadPersistedCloudflareIngressSettings(t.Context(), repo, encryptor)
	require.NoError(t, err)
	require.False(t, settings.Enabled)
	require.Equal(t, CloudflareIngressModeZoneAccessRules, settings.Mode)
	require.Equal(t, CloudflareIngressDefaultQueueCapacity, settings.QueueCapacity)
	require.Equal(t, CloudflareIngressDefaultWAFSyncIntervalSeconds, settings.WAFSyncIntervalSeconds)

	svc := NewCloudflareIngressSettingService(repo, encryptor, edge, &config.Config{})
	_, err = svc.Update(t.Context(), validCloudflareSettingsInput())
	require.ErrorContains(t, err, "enable api_key_auth_cache.invalid_abuse")

	svc = NewCloudflareIngressSettingService(repo, encryptor, edge, enabledCloudflareSettingsConfig())
	input := validCloudflareSettingsInput()
	input.ZoneID = "not-a-zone"
	_, err = svc.Update(t.Context(), input)
	require.ErrorContains(t, err, "32-character hexadecimal")

	input = validCloudflareSettingsInput()
	input.Mode = CloudflareIngressModeWAFCustomRules
	_, err = svc.Update(t.Context(), input)
	require.ErrorContains(t, err, "WAF hostname is required")

	input.WAFHostnames = []string{"api.example.com"}
	input.WAFRuleIDs = []string{"not-a-rule"}
	_, err = svc.Update(t.Context(), input)
	require.ErrorContains(t, err, "WAF rule IDs")
}
