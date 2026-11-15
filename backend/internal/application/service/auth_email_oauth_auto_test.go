//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func newEmailOAuthAutoAuthService(
	userRepo UserRepository,
	settings map[string]string,
	quotaRepo UserPlatformQuotaRepository,
) *AuthService {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:                   "test-secret",
			ExpireHour:               1,
			AccessTokenExpireMinutes: 60,
			RefreshTokenExpireDays:   7,
		},
		Default: config.DefaultConfig{
			UserBalance:     3.5,
			UserConcurrency: 2,
		},
	}

	settingService := NewSettingService(&settingRepoStub{values: settings}, cfg)

	return NewAuthService(
		nil, // entClient — nil, updateUserSignupSource early return
		userRepo,
		nil, // redeemRepo — invitationCode="" 时不触发
		&refreshTokenCacheStub{},
		cfg,
		settingService,
		nil, // emailService
		nil, // turnstileService
		nil, // emailQueueService
		nil, // promoService
		nil, // defaultSubAssigner — nil, assignSubscriptions early return
		nil, // affiliateService — nil, bindOAuthAffiliate early return
		quotaRepo,
	)
}

func TestEmailOAuthAuto_SnapshotsPlatformQuotaDefaults(t *testing.T) {
	userRepo := &userRepoStub{nextID: 88}
	quotaRepo := &userPlatformQuotaRepoStub{}

	svc := newEmailOAuthAutoAuthService(
		userRepo,
		map[string]string{
			SettingKeyRegistrationEnabled:   "true",
			SettingKeyDefaultPlatformQuotas: `{"gemini": {"monthly": 100.0}}`,
		},
		quotaRepo,
	)

	user, err := svc.createEmailOAuthUser(
		context.Background(),
		"newoauth@example.com",
		"newoauth",
		"github",
		"", // invitationCode
		"", // affiliateCode
	)
	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, int64(88), user.ID)
	require.Equal(t, 1, userRepo.guardedCreates)

	require.Len(t, quotaRepo.bulkInsertCalls, 1, "createEmailOAuthUser must snapshot platform quotas via BulkInsertInitial")

	records := quotaRepo.bulkInsertCalls[0]
	var geminiRecord *UserPlatformQuotaRecord
	for i := range records {
		if records[i].Platform == "gemini" {
			geminiRecord = &records[i]
			break
		}
	}
	require.NotNil(t, geminiRecord, "expected gemini platform record")
	require.NotNil(t, geminiRecord.MonthlyLimitUSD)
	require.InDelta(t, 100.0, *geminiRecord.MonthlyLimitUSD, 0.0001)
}

func TestEmailOAuthAuto_RejectsAliasOnlyCollision(t *testing.T) {
	userRepo := &userRepoStub{aliasExists: true}
	quotaRepo := &userPlatformQuotaRepoStub{}
	svc := newEmailOAuthAutoAuthService(userRepo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
	}, quotaRepo)

	user, err := svc.createEmailOAuthUser(
		context.Background(),
		"some.one+oidc@gmail.com",
		"alias-attempt",
		"oidc",
		"",
		"",
	)

	require.Nil(t, user)
	require.ErrorIs(t, err, ErrEmailExists)
	require.Equal(t, 1, userRepo.guardedCreates)
	require.Empty(t, userRepo.created)
	require.Empty(t, quotaRepo.bulkInsertCalls)
}

func TestEmailOAuthAuto_ResumesExactConcurrentRegistration(t *testing.T) {
	existing := &User{ID: 91, Email: "exact@example.com", Status: StatusActive}
	userRepo := &userRepoStub{
		aliasExists:  true,
		usersByEmail: map[string]*User{existing.Email: existing},
	}
	svc := newEmailOAuthAutoAuthService(userRepo, map[string]string{
		SettingKeyRegistrationEnabled: "true",
	}, &userPlatformQuotaRepoStub{})

	user, err := svc.createEmailOAuthUser(
		context.Background(),
		existing.Email,
		"exact",
		"google",
		"",
		"",
	)

	require.NoError(t, err)
	require.Same(t, existing, user)
	require.Equal(t, 1, userRepo.guardedCreates)
}
