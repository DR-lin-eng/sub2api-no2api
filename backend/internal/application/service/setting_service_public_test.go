//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

type settingPublicRepoStub struct {
	mu     sync.Mutex
	values map[string]string
	err    error
	delay  time.Duration
	calls  int
}

func (s *settingPublicRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *settingPublicRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	panic("unexpected GetValue call")
}

func (s *settingPublicRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *settingPublicRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *settingPublicRepoStub) setValues(values map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values = values
}

func (s *settingPublicRepoStub) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestSettingService_IsUserUsageDetailViewAllowed_DefaultsOffAndRequiresExplicitTrue(t *testing.T) {
	tests := []struct {
		name    string
		values  map[string]string
		err     error
		allowed bool
	}{
		{name: "missing", values: map[string]string{}, allowed: false},
		{name: "explicit false", values: map[string]string{SettingKeyAllowUserViewUsageDetails: "false"}, allowed: false},
		{name: "invalid value", values: map[string]string{SettingKeyAllowUserViewUsageDetails: "1"}, allowed: false},
		{name: "explicit true", values: map[string]string{SettingKeyAllowUserViewUsageDetails: "true"}, allowed: true},
		{name: "repository error", values: map[string]string{}, err: errors.New("database unavailable"), allowed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewSettingService(&settingPublicRepoStub{values: tt.values, err: tt.err}, &config.Config{})
			require.Equal(t, tt.allowed, svc.IsUserUsageDetailViewAllowed(context.Background()))
		})
	}
}

func TestSettingService_GetModelPlazaRuntime_AutoPublicModelsRequiresExplicitTrue(t *testing.T) {
	ctx := context.Background()
	enabled := NewSettingService(&settingPublicRepoStub{values: map[string]string{
		SettingKeyModelPlazaEnabled:          "true",
		SettingKeyModelPlazaRequireAuth:      "true",
		SettingKeyModelPlazaAutoPublicModels: "true",
		SettingKeyModelPlazaDescription:      "pricing notes",
	}}, &config.Config{}).GetModelPlazaRuntime(ctx)
	require.True(t, enabled.Enabled)
	require.True(t, enabled.RequireAuth)
	require.True(t, enabled.AutoPublicModels)
	require.Equal(t, "pricing notes", enabled.Description)

	missing := NewSettingService(
		&settingPublicRepoStub{values: map[string]string{SettingKeyModelPlazaEnabled: "true"}},
		&config.Config{},
	).GetModelPlazaRuntime(ctx)
	require.True(t, missing.Enabled)
	require.False(t, missing.AutoPublicModels)

	failed := NewSettingService(
		&settingPublicRepoStub{err: errors.New("database unavailable")},
		&config.Config{},
	).GetModelPlazaRuntime(ctx)
	require.False(t, failed.Enabled)
	require.False(t, failed.AutoPublicModels)
}

func TestSettingService_GetChannelMonitorPublicShareRuntime_OptInAndRequiresMonitorEnabled(t *testing.T) {
	ctx := context.Background()

	enabled := NewSettingService(&settingPublicRepoStub{values: map[string]string{
		SettingKeyChannelMonitorEnabled:                "true",
		SettingKeyChannelMonitorPublicShareEnabled:     "true",
		SettingKeyChannelMonitorPublicShareRequireAuth: "true",
	}}, &config.Config{}).GetChannelMonitorPublicShareRuntime(ctx)
	require.True(t, enabled.Enabled)
	require.True(t, enabled.RequireAuth)

	missingShareSwitch := NewSettingService(&settingPublicRepoStub{values: map[string]string{
		SettingKeyChannelMonitorEnabled: "true",
	}}, &config.Config{}).GetChannelMonitorPublicShareRuntime(ctx)
	require.False(t, missingShareSwitch.Enabled)

	monitorDisabled := NewSettingService(&settingPublicRepoStub{values: map[string]string{
		SettingKeyChannelMonitorEnabled:            "false",
		SettingKeyChannelMonitorPublicShareEnabled: "true",
	}}, &config.Config{}).GetChannelMonitorPublicShareRuntime(ctx)
	require.False(t, monitorDisabled.Enabled)

	failed := NewSettingService(
		&settingPublicRepoStub{err: errors.New("database unavailable")},
		&config.Config{},
	).GetChannelMonitorPublicShareRuntime(ctx)
	require.False(t, failed.Enabled)
}

func TestSettingService_GetChannelMonitorPublicShareRuntime_CachesAndRefreshesImmediately(t *testing.T) {
	repo := &settingPublicRepoStub{values: map[string]string{
		SettingKeyChannelMonitorEnabled:            "true",
		SettingKeyChannelMonitorPublicShareEnabled: "true",
	}}
	svc := NewSettingService(repo, &config.Config{})

	require.True(t, svc.GetChannelMonitorPublicShareRuntime(context.Background()).Enabled)
	repo.setValues(map[string]string{
		SettingKeyChannelMonitorEnabled:            "true",
		SettingKeyChannelMonitorPublicShareEnabled: "false",
	})
	require.True(t, svc.GetChannelMonitorPublicShareRuntime(context.Background()).Enabled)
	require.Equal(t, 1, repo.callCount(), "unexpired reads must not query the settings store")

	svc.refreshCachedSettings(&SystemSettings{
		ChannelMonitorEnabled:                true,
		ChannelMonitorPublicShareEnabled:     false,
		ChannelMonitorPublicShareRequireAuth: true,
	})
	refreshed := svc.GetChannelMonitorPublicShareRuntime(context.Background())
	require.False(t, refreshed.Enabled)
	require.True(t, refreshed.RequireAuth)
	require.Equal(t, 1, repo.callCount(), "settings updates must refresh the cache without another read")
}

func TestSettingService_GetChannelMonitorPublicShareRuntime_CollapsesConcurrentReads(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyChannelMonitorEnabled:            "true",
			SettingKeyChannelMonitorPublicShareEnabled: "true",
		},
		delay: 20 * time.Millisecond,
	}
	svc := NewSettingService(repo, &config.Config{})

	const readers = 32
	start := make(chan struct{})
	results := make(chan ChannelMonitorPublicShareRuntime, readers)
	var wg sync.WaitGroup
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- svc.GetChannelMonitorPublicShareRuntime(context.Background())
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	for result := range results {
		require.True(t, result.Enabled)
	}
	require.Equal(t, 1, repo.callCount())
}

func TestSettingService_GetChannelMonitorPublicShareRuntime_CachesFailuresClosed(t *testing.T) {
	repo := &settingPublicRepoStub{err: errors.New("database unavailable")}
	svc := NewSettingService(repo, &config.Config{})

	require.False(t, svc.GetChannelMonitorPublicShareRuntime(context.Background()).Enabled)
	require.False(t, svc.GetChannelMonitorPublicShareRuntime(context.Background()).Enabled)
	require.Equal(t, 1, repo.callCount(), "transient failures should be cached briefly")
}

func (s *settingPublicRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	panic("unexpected SetMultiple call")
}

func (s *settingPublicRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *settingPublicRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

func TestSettingService_GetPublicSettings_ExposesRegistrationEmailSuffixWhitelist(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyRegistrationEnabled:              "true",
			SettingKeyEmailVerifyEnabled:               "true",
			SettingKeyRegistrationEmailSuffixWhitelist: `["@EXAMPLE.com"," @foo.bar ","*.EDU.CN","@invalid_domain",""]`,
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"@example.com", "@foo.bar", "*.edu.cn"}, settings.RegistrationEmailSuffixWhitelist)
	require.False(t, settings.SupportChatEnabled)
}

func TestSettingService_GetPublicSettingsExposesOnlyTencentPublicFields(t *testing.T) {
	repo := &settingPublicRepoStub{values: map[string]string{
		SettingKeyTencentCaptchaEnabled:        "true",
		SettingKeyTencentCaptchaAppID:          "123456789",
		SettingKeyTencentCaptchaAppSecretKey:   "app-secret-canary",
		SettingKeyTencentCaptchaCloudSecretID:  "cloud-id-canary",
		SettingKeyTencentCaptchaCloudSecretKey: "cloud-secret-canary",
		SettingKeyTencentCaptchaRegion:         TencentCaptchaRegionINTL,
	}}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.TencentCaptchaEnabled)
	require.Equal(t, "123456789", settings.TencentCaptchaAppID)
	require.Equal(t, TencentCaptchaRegionINTL, settings.TencentCaptchaRegion)

	payload, err := json.Marshal(settings)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "app-secret-canary")
	require.NotContains(t, string(payload), "cloud-id-canary")
	require.NotContains(t, string(payload), "cloud-secret-canary")
	require.NotContains(t, string(payload), "tencent_captcha_app_secret_key")
	require.NotContains(t, string(payload), "tencent_captcha_cloud_secret")
}

func TestSettingServiceGetPublicSettingsExposesOnlyAliyunBrowserConfiguration(t *testing.T) {
	repo := &settingPublicRepoStub{values: map[string]string{
		SettingKeyAliyunCaptchaEnabled:         "true",
		SettingKeyAliyunCaptchaAccessKeyID:     "ak-id-canary",
		SettingKeyAliyunCaptchaAccessKeySecret: "ak-secret-canary",
		SettingKeyAliyunCaptchaSceneID:         "scene-1",
		SettingKeyAliyunCaptchaPrefix:          "prefix-1",
		SettingKeyAliyunCaptchaRegion:          AliyunCaptchaRegionSGP,
	}}

	settings, err := NewSettingService(repo, &config.Config{}).GetPublicSettings(context.Background())

	require.NoError(t, err)
	require.True(t, settings.AliyunCaptchaEnabled)
	require.Equal(t, "scene-1", settings.AliyunCaptchaSceneID)
	require.Equal(t, "prefix-1", settings.AliyunCaptchaPrefix)
	require.Equal(t, AliyunCaptchaRegionSGP, settings.AliyunCaptchaRegion)
	payload, err := json.Marshal(settings)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "ak-id-canary")
	require.NotContains(t, string(payload), "ak-secret-canary")
	require.NotContains(t, string(payload), "aliyun_captcha_access_key")
}

func TestSettingService_GetPublicSettings_SupportChatDefaultsToDisabledUnlessExplicitlyEnabled(t *testing.T) {
	svcEnabled := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, &config.Config{})
	enabled, err := svcEnabled.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.False(t, enabled.SupportChatEnabled)

	svcExplicitlyEnabled := NewSettingService(&settingPublicRepoStub{values: map[string]string{
		SettingKeySupportChatEnabled: "true",
	}}, &config.Config{})
	explicitlyEnabled, err := svcExplicitlyEnabled.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, explicitlyEnabled.SupportChatEnabled)
}

func TestSettingService_GetPublicSettings_IPv6EgressUIDefaultsOffAndIsInjectedWhenEnabled(t *testing.T) {
	disabled, err := NewSettingService(
		&settingPublicRepoStub{values: map[string]string{}},
		&config.Config{},
	).GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.False(t, disabled.IPv6EgressUIEnabled)

	svc := NewSettingService(&settingPublicRepoStub{values: map[string]string{
		SettingKeyIPv6EgressUIEnabled: "true",
	}}, &config.Config{})
	enabled, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, enabled.IPv6EgressUIEnabled)

	injected, err := svc.GetPublicSettingsForInjection(context.Background())
	require.NoError(t, err)
	payload, ok := injected.(*PublicSettingsInjectionPayload)
	require.True(t, ok)
	require.True(t, payload.IPv6EgressUIEnabled)
}

func TestSettingService_GetPublicSettings_ExposesTablePreferences(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyTableDefaultPageSize: "50",
			SettingKeyTablePageSizeOptions: "[20,50,100]",
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, 50, settings.TableDefaultPageSize)
	require.Equal(t, []int{20, 50, 100}, settings.TablePageSizeOptions)
}

func TestSettingService_GetPublicSettings_ExposesCompactHomeEnabled(t *testing.T) {
	repo := &settingPublicRepoStub{values: map[string]string{
		SettingKeyCompactHomeEnabled: "true",
	}}
	settings, err := NewSettingService(repo, &config.Config{}).GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.CompactHomeEnabled)

	missing, err := NewSettingService(
		&settingPublicRepoStub{values: map[string]string{}},
		&config.Config{},
	).GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.False(t, missing.CompactHomeEnabled)
}

func TestSettingService_GetPublicSettings_ExposesForceEmailOnThirdPartySignup(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyForceEmailOnThirdPartySignup: "true",
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.ForceEmailOnThirdPartySignup)
}

func TestSettingService_GetPublicSettings_LocalCaptchaDefaultsOffAndCanBeEnabled(t *testing.T) {
	ctx := context.Background()
	disabled, err := NewSettingService(
		&settingPublicRepoStub{values: map[string]string{}},
		&config.Config{},
	).GetPublicSettings(ctx)
	require.NoError(t, err)
	require.False(t, disabled.LocalCaptchaEnabled)

	enabled, err := NewSettingService(
		&settingPublicRepoStub{values: map[string]string{SettingKeyLocalCaptchaEnabled: "true"}},
		&config.Config{},
	).GetPublicSettings(ctx)
	require.NoError(t, err)
	require.True(t, enabled.LocalCaptchaEnabled)
}

func TestSettingService_GetConnectSrcOrigins_IncludesEnabledCapEndpoint(t *testing.T) {
	svc := NewSettingService(&settingPublicRepoStub{values: map[string]string{
		SettingKeyCapEnabled:     "true",
		SettingKeyCapAPIEndpoint: "https://cap.example.com/site-key",
	}}, &config.Config{})

	origins, err := svc.GetConnectSrcOrigins(context.Background())
	require.NoError(t, err)
	require.Contains(t, origins, "https://cap.example.com")
}

func TestSettingService_GetConnectSrcOrigins_IncludesExactAliyunCaptchaEndpoints(t *testing.T) {
	tests := []struct {
		name     string
		region   string
		expected []string
	}{
		{
			name:   "mainland china",
			region: AliyunCaptchaRegionCN,
			expected: []string{
				"https://tenant-1.captcha-open.aliyuncs.com",
				"https://tenant-1.captcha-open-b.aliyuncs.com",
				"https://upload.captcha-open.aliyuncs.com",
				"https://cloudauth-device.aliyuncs.com",
				"https://cloudauth-device-dualstack.cn-shanghai.aliyuncs.com",
				"https://cn-shanghai.device.saf.aliyuncs.com",
			},
		},
		{
			name:   "singapore",
			region: AliyunCaptchaRegionSGP,
			expected: []string{
				"https://tenant-1.captcha-open-southeast.aliyuncs.com",
				"https://tenant-1.captcha-open-southeast-b.aliyuncs.com",
				"https://upload.captcha-open-southeast.aliyuncs.com",
				"https://cloudauth-device.ap-southeast-1.aliyuncs.com",
				"https://cloudauth-device-dualstack.ap-southeast-1.aliyuncs.com",
				"https://ap-southeast-1.device.saf.aliyuncs.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewSettingService(&settingPublicRepoStub{values: map[string]string{
				SettingKeyAliyunCaptchaEnabled: "true",
				SettingKeyAliyunCaptchaPrefix:  "tenant-1",
				SettingKeyAliyunCaptchaRegion:  tt.region,
			}}, &config.Config{})

			origins, err := svc.GetConnectSrcOrigins(context.Background())

			require.NoError(t, err)
			require.ElementsMatch(t, tt.expected, origins)
		})
	}
}

func TestSettingService_GetConnectSrcOrigins_RejectsUnsafeAliyunPrefix(t *testing.T) {
	svc := NewSettingService(&settingPublicRepoStub{values: map[string]string{
		SettingKeyAliyunCaptchaEnabled: "true",
		SettingKeyAliyunCaptchaPrefix:  "tenant; script-src *",
	}}, &config.Config{})

	origins, err := svc.GetConnectSrcOrigins(context.Background())

	require.NoError(t, err)
	require.Empty(t, origins)
}

func TestSettingService_GetPublicSettings_ExposesAllowUserViewErrorRequests(t *testing.T) {
	repo := &settingPublicRepoStub{
		values: map[string]string{
			SettingKeyAllowUserViewErrorRequests: "true",
		},
	}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.AllowUserViewErrorRequests)
}

func TestSettingService_GetPublicSettings_ExposesAllowUserViewUsageDetails(t *testing.T) {
	t.Run("disabled by default", func(t *testing.T) {
		settings, err := NewSettingService(
			&settingPublicRepoStub{values: map[string]string{}},
			&config.Config{},
		).GetPublicSettings(context.Background())
		require.NoError(t, err)
		require.False(t, settings.AllowUserViewUsageDetails)
	})

	t.Run("enabled explicitly and included in injection", func(t *testing.T) {
		svc := NewSettingService(
			&settingPublicRepoStub{values: map[string]string{SettingKeyAllowUserViewUsageDetails: "true"}},
			&config.Config{},
		)
		settings, err := svc.GetPublicSettings(context.Background())
		require.NoError(t, err)
		require.True(t, settings.AllowUserViewUsageDetails)

		injected, err := svc.GetPublicSettingsForInjection(context.Background())
		require.NoError(t, err)
		payload, ok := injected.(*PublicSettingsInjectionPayload)
		require.True(t, ok)
		require.True(t, payload.AllowUserViewUsageDetails)
	})
}

func TestSettingService_GetPublicSettings_ExposesWeChatOAuthModeCapabilities(t *testing.T) {
	svc := NewSettingService(&settingPublicRepoStub{
		values: map[string]string{
			SettingKeyWeChatConnectEnabled:             "true",
			SettingKeyWeChatConnectAppID:               "wx-mp-app",
			SettingKeyWeChatConnectAppSecret:           "wx-mp-secret",
			SettingKeyWeChatConnectMode:                "mp",
			SettingKeyWeChatConnectScopes:              "snsapi_base",
			SettingKeyWeChatConnectOpenEnabled:         "true",
			SettingKeyWeChatConnectMPEnabled:           "true",
			SettingKeyWeChatConnectRedirectURL:         "https://api.example.com/api/v1/auth/oauth/wechat/callback",
			SettingKeyWeChatConnectFrontendRedirectURL: "/auth/wechat/callback",
		},
	}, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.WeChatOAuthEnabled)
	require.True(t, settings.WeChatOAuthOpenEnabled)
	require.True(t, settings.WeChatOAuthMPEnabled)
}

func TestSettingService_GetPublicSettings_DoesNotExposeMobileOnlyWeChatAsWebOAuthAvailable(t *testing.T) {
	svc := NewSettingService(&settingPublicRepoStub{
		values: map[string]string{
			SettingKeyWeChatConnectEnabled:             "true",
			SettingKeyWeChatConnectMobileEnabled:       "true",
			SettingKeyWeChatConnectMode:                "mobile",
			SettingKeyWeChatConnectMobileAppID:         "wx-mobile-app",
			SettingKeyWeChatConnectMobileAppSecret:     "wx-mobile-secret",
			SettingKeyWeChatConnectFrontendRedirectURL: "/auth/wechat/callback",
		},
	}, &config.Config{})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.WeChatOAuthEnabled)
	require.False(t, settings.WeChatOAuthOpenEnabled)
	require.False(t, settings.WeChatOAuthMPEnabled)
	require.True(t, settings.WeChatOAuthMobileEnabled)
}

func TestSettingService_GetPublicSettings_FallsBackToConfigForWeChatOAuthCapabilities(t *testing.T) {
	svc := NewSettingService(&settingPublicRepoStub{values: map[string]string{}}, &config.Config{
		WeChat: config.WeChatConnectConfig{
			Enabled:             true,
			OpenEnabled:         true,
			OpenAppID:           "wx-open-config",
			OpenAppSecret:       "wx-open-secret",
			FrontendRedirectURL: "/auth/wechat/config-callback",
		},
	})

	settings, err := svc.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.WeChatOAuthEnabled)
	require.True(t, settings.WeChatOAuthOpenEnabled)
	require.False(t, settings.WeChatOAuthMPEnabled)
	require.False(t, settings.WeChatOAuthMobileEnabled)
}
