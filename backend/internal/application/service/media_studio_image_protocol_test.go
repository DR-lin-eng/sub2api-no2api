//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type mediaImageProtocolRepo struct {
	AccountRepository
	accounts []Account
}

func (r *mediaImageProtocolRepo) ListSchedulableByGroupIDAndPlatform(context.Context, int64, string) ([]Account, error) {
	return r.accounts, nil
}

func TestChatGPTAppImagesMediaNativeOnlyAndMixedGroups(t *testing.T) {
	for _, tc := range []struct {
		name     string
		accounts []Account
		want     string
	}{
		{"API key only", []Account{{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}}, "legacy"},
		{"OAuth", []Account{{Platform: PlatformOpenAI, Type: AccountTypeOAuth}}, "chatgpt_app"},
		{"SIWC", []Account{{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"auth_mode": "siwc"}}}, "legacy"},
		{"mixed", []Account{{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, {Platform: PlatformOpenAI, Type: AccountTypeOAuth}}, "chatgpt_app"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &MediaStudioService{gatewayService: &OpenAIGatewayService{accountRepo: &mediaImageProtocolRepo{accounts: tc.accounts}}}
			require.Equal(t, tc.want, svc.imageProtocolForGroup(t.Context(), MediaStudioGroupOption{GroupID: 1, Platform: PlatformOpenAI}, "chatgpt_app"))
		})
	}
}

type image26IndependentProtocolRepo struct{ AccountRepository }

func (image26IndependentProtocolRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, _ int64, _ string) ([]Account, error) {
	accounts := []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: false},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusError, Schedulable: true},
	}
	var eligible []Account
	for _, a := range accounts {
		if a.IsSchedulableWithContext(ctx) {
			eligible = append(eligible, a)
		}
	}
	return eligible, nil
}

func TestGPTImage26NativeControlsUseIndependentImagePool(t *testing.T) {
	settings := NewSettingService(&openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{SettingKeyImageIndependentSchedulingEnabled: "true"}}, nil)
	svc := &MediaStudioService{gatewayService: &OpenAIGatewayService{accountRepo: image26IndependentProtocolRepo{}, settingService: settings}}
	require.Equal(t, "legacy", svc.imageProtocolForGroup(t.Context(), MediaStudioGroupOption{GroupID: 1, Platform: PlatformOpenAI}, "chatgpt_app"), "healthy paused native accounts support the SVG/GIF controls; errored OAuth accounts do not force App mode")
}
