//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIFailureCounterStub struct {
	count        int64
	resetCalls   int
	incrementErr error
}

func (c *openAIFailureCounterStub) IncrementOpenAIFailureCount(context.Context, int64) (int64, error) {
	if c.incrementErr != nil {
		return 0, c.incrementErr
	}
	c.count++
	return c.count, nil
}

func (c *openAIFailureCounterStub) ResetOpenAIFailureCount(context.Context, int64) error {
	c.count = 0
	c.resetCalls++
	return nil
}

type openAIFailureAutoDisableRepo struct {
	rateLimitAccountRepoStub
	schedulableCalls []bool
	extraUpdates     []map[string]any
}

func (r *openAIFailureAutoDisableRepo) SetSchedulable(_ context.Context, _ int64, schedulable bool) error {
	r.schedulableCalls = append(r.schedulableCalls, schedulable)
	return nil
}

func (r *openAIFailureAutoDisableRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.extraUpdates = append(r.extraUpdates, updates)
	return nil
}

func openAIFailurePolicySettingService(t *testing.T, enabled bool, threshold int) *SettingService {
	t.Helper()
	repo := newMockSettingRepo()
	payload, err := json.Marshal(RateLimit429CooldownSettings{
		Enabled:              true,
		CooldownSeconds:      5,
		AutoDisableEnabled:   enabled,
		AutoDisableThreshold: threshold,
	})
	require.NoError(t, err)
	repo.data[SettingKeyRateLimit429CooldownSettings] = string(payload)
	return NewSettingService(repo, &config.Config{})
}

func TestRateLimitService_AutoDisablesOpenAIAccountAtConfiguredConsecutiveFailureThreshold(t *testing.T) {
	repo := &openAIFailureAutoDisableRepo{}
	counter := &openAIFailureCounterStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetSettingService(openAIFailurePolicySettingService(t, true, 3))
	service.SetOpenAIFailureCounterCache(counter)
	account := &Account{
		ID:          9001,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
	}

	for _, status := range []int{http.StatusBadGateway, http.StatusTooManyRequests} {
		require.False(t, service.maybeAutoDisableOpenAIAccountOnFailure(context.Background(), account, status, nil))
		require.True(t, account.Schedulable)
	}
	require.True(t, service.maybeAutoDisableOpenAIAccountOnFailure(context.Background(), account, http.StatusTooManyRequests, nil))
	require.False(t, account.Schedulable)
	require.Equal(t, []bool{false}, repo.schedulableCalls)
	require.Equal(t, "Automatically paused after 3 consecutive OpenAI OAuth upstream 429/502 failures (last status 429). Re-enable scheduling manually after checking the account quota and upstream health.", account.SchedulingDisabledReason())
	require.NotEmpty(t, repo.extraUpdates)
	require.Equal(t, int64(3), counter.count, "the counter remains armed until success or manual recovery")

	// The threshold crossing is idempotent: later failures do not enqueue a
	// second persistent update for the same counter window.
	require.False(t, service.maybeAutoDisableOpenAIAccountOnFailure(context.Background(), account, http.StatusBadGateway, nil))
	require.Equal(t, []bool{false}, repo.schedulableCalls)
}

func TestRateLimitService_AutoDisableDisabledDoesNotCount(t *testing.T) {
	repo := &openAIFailureAutoDisableRepo{}
	counter := &openAIFailureCounterStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetSettingService(openAIFailurePolicySettingService(t, false, 1))
	service.SetOpenAIFailureCounterCache(counter)
	account := &Account{ID: 9002, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true}

	require.False(t, service.maybeAutoDisableOpenAIAccountOnFailure(context.Background(), account, http.StatusBadGateway, nil))
	require.Zero(t, counter.count)
	require.Empty(t, repo.schedulableCalls)
}

func TestOpenAIGatewayService_SuccessResetsCrossInstanceFailureCounter(t *testing.T) {
	counter := &openAIFailureCounterStub{count: 2}
	rateLimits := &RateLimitService{openAIFailureCounterCache: counter}
	rateLimits.SetSettingService(openAIFailurePolicySettingService(t, true, 3))
	svc := &OpenAIGatewayService{rateLimitService: rateLimits}

	svc.ReportOpenAIAccountScheduleResult(9003, "gpt-5.5", true, nil)

	require.Equal(t, 1, counter.resetCalls)
	require.Zero(t, counter.count)
}

func TestOpenAIGatewayService_SuccessSkipsFailureCounterWhenPolicyDisabled(t *testing.T) {
	counter := &openAIFailureCounterStub{count: 2}
	rateLimits := &RateLimitService{openAIFailureCounterCache: counter}
	rateLimits.SetSettingService(openAIFailurePolicySettingService(t, false, 3))
	svc := &OpenAIGatewayService{rateLimitService: rateLimits}

	svc.ReportOpenAIAccountScheduleResult(9003, "gpt-5.5", true, nil)

	require.Zero(t, counter.resetCalls)
	require.Equal(t, int64(2), counter.count)
}

func TestRateLimitService_AutoDisableIgnoresNonAccountScopedFailures(t *testing.T) {
	repo := &openAIFailureAutoDisableRepo{}
	counter := &openAIFailureCounterStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetSettingService(openAIFailurePolicySettingService(t, true, 1))
	service.SetOpenAIFailureCounterCache(counter)

	for _, tc := range []struct {
		name    string
		account *Account
		status  int
	}{
		{name: "grok", account: &Account{ID: 9004, Platform: PlatformGrok, Type: AccountTypeOAuth, Schedulable: true}, status: http.StatusBadGateway},
		{name: "internal server error", account: &Account{ID: 9005, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Schedulable: true}, status: http.StatusInternalServerError},
		{name: "client error", account: &Account{ID: 9006, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Schedulable: true}, status: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.False(t, service.maybeAutoDisableOpenAIAccountOnFailure(context.Background(), tc.account, tc.status, nil))
		})
	}
	require.Zero(t, counter.count)
	require.Empty(t, repo.schedulableCalls)
}

func TestRateLimitService_AutoDisablePolicyNormalizesInvalidThreshold(t *testing.T) {
	settingRepo := newMockSettingRepo()
	settingRepo.data[SettingKeyRateLimit429CooldownSettings] = `{"enabled":true,"cooldown_seconds":5,"auto_disable_enabled":true,"auto_disable_threshold":0}`
	settings, err := NewSettingService(settingRepo, &config.Config{}).GetRateLimit429CooldownSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, 3, settings.AutoDisableThreshold)
	require.True(t, settings.AutoDisableEnabled)
}

func TestOpenAIStreamDisconnect502UsesAutoDisablePolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &openAIFailureAutoDisableRepo{}
	counter := &openAIFailureCounterStub{}
	rateLimits := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	rateLimits.SetSettingService(openAIFailurePolicySettingService(t, true, 1))
	rateLimits.SetOpenAIFailureCounterCache(counter)
	svc := &OpenAIGatewayService{rateLimitService: rateLimits}
	account := &Account{ID: 9007, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	failoverErr := svc.newOpenAIStreamFailoverError(
		c, account, false, "", nil,
		"OpenAI stream disconnected before completion: upstream connection error",
	)

	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.False(t, account.Schedulable)
	require.Contains(t, account.SchedulingDisabledReason(), "last status 502")
}

func TestOpenAILocalStreamStaging502DoesNotUseAutoDisablePolicy(t *testing.T) {
	repo := &openAIFailureAutoDisableRepo{}
	counter := &openAIFailureCounterStub{}
	rateLimits := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	rateLimits.SetSettingService(openAIFailurePolicySettingService(t, true, 1))
	rateLimits.SetOpenAIFailureCounterCache(counter)
	svc := &OpenAIGatewayService{rateLimitService: rateLimits}
	account := &Account{ID: 9008, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}

	svc.newOpenAIStreamFailoverError(nil, account, false, "", nil, "OpenAI first-output staging limit exceeded")

	require.True(t, account.Schedulable)
	require.Empty(t, repo.schedulableCalls)
}

func TestRateLimitService_AutoDisableNeverCountsOpenAIAPIKeyAccounts(t *testing.T) {
	repo := &openAIFailureAutoDisableRepo{}
	counter := &openAIFailureCounterStub{}
	service := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	service.SetSettingService(openAIFailurePolicySettingService(t, true, 1))
	service.SetOpenAIFailureCounterCache(counter)
	account := &Account{
		ID:          9009,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
	}

	for _, statusCode := range []int{http.StatusTooManyRequests, http.StatusBadGateway} {
		require.False(t, service.maybeAutoDisableOpenAIAccountOnFailure(context.Background(), account, statusCode, nil))
	}
	require.Zero(t, counter.count)
	require.True(t, account.Schedulable)
	require.Empty(t, repo.schedulableCalls)
}
