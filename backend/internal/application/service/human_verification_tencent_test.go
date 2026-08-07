//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type humanVerificationSettingRepoSpy struct {
	SettingRepository
	values map[string]string
	err    error
	calls  int
	keys   []string
}

func (s *humanVerificationSettingRepoSpy) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	s.calls++
	s.keys = append([]string(nil), keys...)
	if s.err != nil {
		return nil, s.err
	}
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

type tencentCaptchaVerifierSpy struct {
	calls       int
	credentials TencentCaptchaCredentials
	proof       TencentCaptchaProof
	remoteIP    string
	result      *TencentCaptchaVerifyResponse
	err         error
}

func (s *tencentCaptchaVerifierSpy) VerifyTicket(
	_ context.Context,
	credentials TencentCaptchaCredentials,
	proof TencentCaptchaProof,
	remoteIP string,
) (*TencentCaptchaVerifyResponse, error) {
	s.calls++
	s.credentials = credentials
	s.proof = proof
	s.remoteIP = remoteIP
	return s.result, s.err
}

func TestGetHumanVerificationConfigUsesOneSettingsSnapshot(t *testing.T) {
	repo := &humanVerificationSettingRepoSpy{values: map[string]string{
		SettingKeyTencentCaptchaEnabled:        "true",
		SettingKeyTencentCaptchaAppID:          " 123456 ",
		SettingKeyTencentCaptchaAppSecretKey:   " app-secret ",
		SettingKeyTencentCaptchaCloudSecretID:  " cloud-id ",
		SettingKeyTencentCaptchaCloudSecretKey: " cloud-secret ",
		SettingKeyTencentCaptchaRegion:         " INTL ",
	}}
	settingService := NewSettingService(repo, nil)

	config, err := settingService.GetHumanVerificationConfig(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, repo.calls)
	require.ElementsMatch(t, humanVerificationSettingKeys, repo.keys)
	require.Equal(t, HumanVerificationProviderTencent, config.Provider)
	require.Equal(t, TencentCaptchaConfig{
		Enabled:        true,
		AppID:          "123456",
		AppSecretKey:   "app-secret",
		CloudSecretID:  "cloud-id",
		CloudSecretKey: "cloud-secret",
		Region:         TencentCaptchaRegionINTL,
	}, config.Tencent)
}

func TestGetHumanVerificationConfigRejectsDirtyProviderState(t *testing.T) {
	repo := &humanVerificationSettingRepoSpy{values: map[string]string{
		SettingKeyRecaptchaEnabled:      "true",
		SettingKeyTencentCaptchaEnabled: "true",
		SettingKeyLocalCaptchaEnabled:   "true",
	}}
	settingService := NewSettingService(repo, nil)

	config, err := settingService.GetHumanVerificationConfig(context.Background())

	require.NoError(t, err)
	require.Equal(t, HumanVerificationProviderInvalid, config.Provider)
}

func TestGetHumanVerificationConfigPreservesLegacyTurnstileLocalPrecedence(t *testing.T) {
	repo := &humanVerificationSettingRepoSpy{values: map[string]string{
		SettingKeyTurnstileEnabled:    "true",
		SettingKeyLocalCaptchaEnabled: "true",
	}}
	settingService := NewSettingService(repo, nil)

	config, err := settingService.GetHumanVerificationConfig(context.Background())

	require.NoError(t, err)
	require.Equal(t, HumanVerificationProviderTurnstile, config.Provider)
}

func TestTencentCaptchaVerificationUsesSnapshotCredentials(t *testing.T) {
	repo := &humanVerificationSettingRepoSpy{values: map[string]string{
		SettingKeyTencentCaptchaEnabled:        "true",
		SettingKeyTencentCaptchaAppID:          "123456",
		SettingKeyTencentCaptchaAppSecretKey:   "app-secret",
		SettingKeyTencentCaptchaCloudSecretID:  "cloud-id",
		SettingKeyTencentCaptchaCloudSecretKey: "cloud-secret",
		SettingKeyTencentCaptchaRegion:         TencentCaptchaRegionINTL,
	}}
	settingService := NewSettingService(repo, nil)
	verifier := &tencentCaptchaVerifierSpy{result: &TencentCaptchaVerifyResponse{CaptchaCode: 1}}
	humanVerification := NewHumanVerificationService(settingService, nil, nil, nil, verifier)

	err := humanVerification.VerifyProof(context.Background(), HumanVerificationProof{
		TencentTicket:  " ticket ",
		TencentRandstr: " rand ",
	}, "203.0.113.10", false)

	require.NoError(t, err)
	require.Equal(t, 1, repo.calls)
	require.Equal(t, 1, verifier.calls)
	require.Equal(t, TencentCaptchaCredentials{
		AppID:          123456,
		AppSecretKey:   "app-secret",
		CloudSecretID:  "cloud-id",
		CloudSecretKey: "cloud-secret",
		Endpoint:       tencentCaptchaEndpointINTL,
	}, verifier.credentials)
	require.Equal(t, TencentCaptchaProof{Ticket: "ticket", Randstr: "rand"}, verifier.proof)
	require.Equal(t, "203.0.113.10", verifier.remoteIP)
}

func TestTencentCaptchaRegionDefaultsToChineseMainland(t *testing.T) {
	repo := &humanVerificationSettingRepoSpy{values: map[string]string{
		SettingKeyTencentCaptchaEnabled:        "true",
		SettingKeyTencentCaptchaAppID:          "123456",
		SettingKeyTencentCaptchaAppSecretKey:   "app-secret",
		SettingKeyTencentCaptchaCloudSecretID:  "cloud-id",
		SettingKeyTencentCaptchaCloudSecretKey: "cloud-secret",
		SettingKeyTencentCaptchaRegion:         "unsupported",
	}}
	settingService := NewSettingService(repo, nil)
	verifier := &tencentCaptchaVerifierSpy{result: &TencentCaptchaVerifyResponse{CaptchaCode: 1}}
	humanVerification := NewHumanVerificationService(settingService, nil, nil, nil, verifier)

	err := humanVerification.VerifyProof(context.Background(), HumanVerificationProof{
		TencentTicket: "ticket", TencentRandstr: "rand",
	}, "203.0.113.10", false)

	require.NoError(t, err)
	require.Equal(t, tencentCaptchaEndpointCN, verifier.credentials.Endpoint)
}

func TestTencentCaptchaVerificationRejectsInvalidProofBeforeNetwork(t *testing.T) {
	repo := &humanVerificationSettingRepoSpy{values: map[string]string{
		SettingKeyTencentCaptchaEnabled:        "true",
		SettingKeyTencentCaptchaAppID:          "123456",
		SettingKeyTencentCaptchaAppSecretKey:   "app-secret",
		SettingKeyTencentCaptchaCloudSecretID:  "cloud-id",
		SettingKeyTencentCaptchaCloudSecretKey: "cloud-secret",
	}}
	settingService := NewSettingService(repo, nil)
	verifier := &tencentCaptchaVerifierSpy{}
	humanVerification := NewHumanVerificationService(settingService, nil, nil, nil, verifier)

	err := humanVerification.VerifyProof(context.Background(), HumanVerificationProof{
		TencentTicket:  "trerror_1001",
		TencentRandstr: "rand",
	}, "203.0.113.10", false)

	require.ErrorIs(t, err, ErrTencentCaptchaVerificationFailed)
	require.Zero(t, verifier.calls)
}

func TestHumanVerificationSettingsReadFailureFailsClosed(t *testing.T) {
	repo := &humanVerificationSettingRepoSpy{err: errors.New("database unavailable")}
	settingService := NewSettingService(repo, nil)
	humanVerification := NewHumanVerificationService(settingService, nil, nil, nil, &tencentCaptchaVerifierSpy{})

	err := humanVerification.VerifyProof(context.Background(), HumanVerificationProof{}, "", false)

	require.ErrorIs(t, err, ErrHumanVerificationUnavailable)
}
