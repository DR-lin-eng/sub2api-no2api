package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain/model"
	"github.com/stretchr/testify/require"
)

func TestResolveTLSProfileUsesStableAccountAssignment(t *testing.T) {
	service := &TLSFingerprintProfileService{localCache: make(map[int64]*model.TLSFingerprintProfile)}
	service.setLocalCache([]*model.TLSFingerprintProfile{
		{ID: 11, Name: "profile-a"},
		{ID: 22, Name: "profile-b"},
		{ID: 33, Name: "profile-c"},
	})

	account := &Account{
		ID:       42,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"enable_tls_fingerprint": true, "tls_fingerprint_profile_id": -1},
	}
	first := service.ResolveTLSProfile(account)
	second := service.ResolveTLSProfile(account)

	require.NotNil(t, first)
	require.Equal(t, first.Name, second.Name)
	require.Equal(t, service.ResolveTLSProfileKey(account), service.ResolveTLSProfileKey(account))
}

func TestResolveTLSProfileDefaultAlsoUsesStableConfiguredProfile(t *testing.T) {
	service := &TLSFingerprintProfileService{localCache: make(map[int64]*model.TLSFingerprintProfile)}
	service.setLocalCache([]*model.TLSFingerprintProfile{
		{ID: 11, Name: "profile-a"},
		{ID: 22, Name: "profile-b"},
	})

	account := &Account{
		ID:       7,
		Platform: PlatformAnthropic,
		Type:     AccountTypeSetupToken,
		Extra:    map[string]any{"enable_tls_fingerprint": true},
	}
	first := service.ResolveTLSProfile(account)
	second := service.ResolveTLSProfile(account)

	require.NotNil(t, first)
	require.Equal(t, first.Name, second.Name)
}

func TestResolveTLSProfileUsesCodexRustlsDefaultsForOpenAI(t *testing.T) {
	service := &TLSFingerprintProfileService{localCache: make(map[int64]*model.TLSFingerprintProfile)}
	account := &Account{
		ID:       9,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"enable_tls_fingerprint": true},
	}

	profile := service.ResolveTLSProfile(account)
	require.NotNil(t, profile)
	require.Equal(t, "Built-in Codex Rustls (aws-lc-rs)", profile.Name)
	require.Equal(t, []string{"h2", "http/1.1"}, profile.ALPNProtocols)
	require.Equal(t, []uint16{0x1302, 0x1301, 0x1303}, profile.CipherSuites[:3])
	require.Equal(t, []uint16{0x0503, 0x0403, 0x0603}, profile.SignatureAlgorithms[:3])
}

func TestResolveTLSProfileUsesVirtualPrincipalForOpenAIStableAssignment(t *testing.T) {
	service := &TLSFingerprintProfileService{localCache: make(map[int64]*model.TLSFingerprintProfile)}
	service.setLocalCache([]*model.TLSFingerprintProfile{
		{ID: 11, Name: "profile-a"},
		{ID: 22, Name: "profile-b"},
		{ID: 33, Name: "profile-c"},
	})
	accountA := &Account{
		ID:       101,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"chatgpt_account_id": "shared-principal",
		},
		Extra: map[string]any{
			"enable_tls_fingerprint":     true,
			"tls_fingerprint_profile_id": int64(-1),
		},
	}
	accountB := &Account{
		ID:       202,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"chatgpt_account_id": "shared-principal",
		},
		Extra: map[string]any{
			"enable_tls_fingerprint":     true,
			"tls_fingerprint_profile_id": int64(-1),
		},
	}

	first := service.ResolveTLSProfile(accountA)
	second := service.ResolveTLSProfile(accountB)
	require.NotNil(t, first)
	require.NotNil(t, second)
	require.Equal(t, first.Name, second.Name)
	require.Equal(t, service.ResolveTLSProfileKey(accountA), service.ResolveTLSProfileKey(accountB))
}
