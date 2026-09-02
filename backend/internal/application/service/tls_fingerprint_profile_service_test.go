package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain/model"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
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
	require.Contains(t, profile.CipherSuites, uint16(0x1301))
	require.Contains(t, profile.CipherSuites, uint16(0x1302))
	require.Contains(t, profile.CipherSuites, uint16(0x1303))
	require.Less(t, len(profile.CipherSuites), len(tlsfingerprint.BuiltInCodexRustlsProfile().CipherSuites))
	require.ElementsMatch(t, tlsfingerprint.BuiltInCodexRustlsProfile().Curves, profile.Curves)
	require.NotEqual(t,
		tlsfingerprint.FingerprintKey(tlsfingerprint.BuiltInCodexRustlsProfile()),
		tlsfingerprint.FingerprintKey(profile),
	)
}

func TestResolveTLSProfileUsesDistinctAccountVariantsForOpenAIStableAssignment(t *testing.T) {
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
	require.NotEqual(t, service.ResolveTLSProfileKey(accountA), service.ResolveTLSProfileKey(accountB))
}

func TestResolveTLSProfileVariantsExplicitProfilePerOpenAIAccount(t *testing.T) {
	service := &TLSFingerprintProfileService{localCache: make(map[int64]*model.TLSFingerprintProfile)}
	service.setLocalCache([]*model.TLSFingerprintProfile{{
		ID:           11,
		Name:         "shared-admin-profile",
		CipherSuites: []uint16{0x1301, 0x1302, 0x1303, 0xc02b, 0xc02f},
		Curves:       []uint16{29, 23},
	}})
	accountA := &Account{ID: 301, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{
		TLSFingerprintEnabledExtraKey: true, TLSFingerprintProfileIDExtraKey: int64(11),
	}}
	accountB := &Account{ID: 302, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{
		TLSFingerprintEnabledExtraKey: true, TLSFingerprintProfileIDExtraKey: int64(11),
	}}

	require.NotEqual(t, service.ResolveTLSProfileKey(accountA), service.ResolveTLSProfileKey(accountB))
}

func TestSaltedTLSFingerprintVariantKeyIsStableAndSecretScoped(t *testing.T) {
	first := saltedTLSFingerprintVariantKey("1001", "persisted-secret-a")

	require.Len(t, first, 64)
	require.Equal(t, first, saltedTLSFingerprintVariantKey("1001", "persisted-secret-a"))
	require.NotEqual(t, first, saltedTLSFingerprintVariantKey("1002", "persisted-secret-a"))
	require.NotEqual(t, first, saltedTLSFingerprintVariantKey("1001", "persisted-secret-b"))
	require.Equal(t, "1001", saltedTLSFingerprintVariantKey("1001", ""))
}

func TestResolveTLSProfileDefaultBatchProducesDistinctAccountKeys(t *testing.T) {
	service := &TLSFingerprintProfileService{localCache: make(map[int64]*model.TLSFingerprintProfile)}
	seen := make(map[string]int64, 4096)

	for id := int64(1001); id < 5097; id++ {
		account := &Account{
			ID:       id,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{TLSFingerprintEnabledExtraKey: true},
		}
		key := service.ResolveTLSProfileKey(account)
		require.NotEmpty(t, key)
		if previousID, exists := seen[key]; exists {
			t.Fatalf("accounts %d and %d resolved to the same TLS fingerprint key %s", previousID, id, key)
		}
		seen[key] = id
	}
	require.Len(t, seen, 4096)
}
