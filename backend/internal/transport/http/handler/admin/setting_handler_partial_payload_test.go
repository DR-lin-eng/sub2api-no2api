//go:build unit

package admin

import (
	"maps"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler/dto"

	"github.com/stretchr/testify/require"
)

func TestSettingKeyJSONAliasesAreCompleteAndValid(t *testing.T) {
	require.Equal(t, map[string]string{
		"smtp_from_email": service.SettingKeySMTPFrom,
	}, settingKeyJSONAliases)

	requestType := reflect.TypeOf(UpdateSettingsRequest{})
	requestFields := make(map[string]reflect.StructField, requestType.NumField())
	for i := 0; i < requestType.NumField(); i++ {
		field := requestType.Field(i)
		jsonName, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if jsonName != "" && jsonName != "-" {
			requestFields[jsonName] = field
		}
	}
	for jsonName, settingKey := range settingKeyJSONAliases {
		field, ok := requestFields[jsonName]
		require.Truef(t, ok, "alias %q must name an UpdateSettingsRequest field", jsonName)
		require.NotEqual(t, reflect.Ptr, field.Type.Kind(), "pointer fields already carry presence")
		require.NotEqual(t, jsonName, settingKey, "identity mappings do not belong in the alias table")
		require.Equal(t, settingKey, settingKeyByJSONName[jsonName])
	}
}

func TestOmittedUpdateSettingsMergeCoverage(t *testing.T) {
	require.Equal(t, map[string]struct{}{
		"PaymentEnabledTypes":              {},
		"AuthSourceEmailPlatformQuotas":    {},
		"AuthSourceLinuxDoPlatformQuotas":  {},
		"AuthSourceOIDCPlatformQuotas":     {},
		"AuthSourceWeChatPlatformQuotas":   {},
		"AuthSourceGitHubPlatformQuotas":   {},
		"AuthSourceGooglePlatformQuotas":   {},
		"AuthSourceDingTalkPlatformQuotas": {},
	}, omittedUpdateSettingsMergeExclusions)
	require.Len(t, omittedUpdateSettingsFieldConverters, 5)
	require.Contains(t, omittedUpdateSettingsFieldConverters, "RegistrationEmailSuffixWhitelist")
	require.Contains(t, omittedUpdateSettingsFieldConverters, "LoginAgreementDocuments")
	require.Contains(t, omittedUpdateSettingsFieldConverters, "TablePageSizeOptions")
	require.Contains(t, omittedUpdateSettingsFieldConverters, "DefaultSubscriptions")
	require.Contains(t, omittedUpdateSettingsFieldConverters, "DefaultPlatformQuotas")

	requestType := reflect.TypeOf(UpdateSettingsRequest{})
	settingsType := reflect.TypeOf(service.SystemSettings{})
	for i := 0; i < requestType.NumField(); i++ {
		requestField := requestType.Field(i)
		if requestField.Type.Kind() == reflect.Ptr {
			continue
		}
		jsonName, _, _ := strings.Cut(requestField.Tag.Get("json"), ",")
		if jsonName == "" || jsonName == "-" {
			continue
		}

		settingsField, hasSettingsField := settingsType.FieldByName(requestField.Name)
		_, excluded := omittedUpdateSettingsMergeExclusions[requestField.Name]
		converter, converted := omittedUpdateSettingsFieldConverters[requestField.Name]
		switch {
		case excluded:
			require.Falsef(t, hasSettingsField, "excluded field %s now has a SystemSettings counterpart", requestField.Name)
			require.Falsef(t, converted, "excluded field %s must not also define a converter", requestField.Name)
		case converted:
			require.Truef(t, hasSettingsField, "converted field %s must have a SystemSettings source", requestField.Name)
			convertedValue := reflect.ValueOf(converter(&service.SystemSettings{}))
			require.Truef(t, convertedValue.IsValid(), "converter for %s returned an invalid value", requestField.Name)
			require.Truef(t, convertedValue.Type().AssignableTo(requestField.Type),
				"converter for %s returned %s, want %s", requestField.Name, convertedValue.Type(), requestField.Type)
		case !hasSettingsField:
			t.Errorf("value field %s must be assignable, converted, or explicitly excluded", requestField.Name)
		default:
			require.Truef(t, settingsField.Type.AssignableTo(requestField.Type),
				"SystemSettings.%s has type %s, want %s", requestField.Name, settingsField.Type, requestField.Type)
			require.NotContainsf(t, []reflect.Kind{reflect.Slice, reflect.Map}, requestField.Type.Kind(),
				"container field %s must define a cloning converter", requestField.Name)
		}
	}
}

func TestMergeOmittedUpdateSettingsRequestConvertsDTOFields(t *testing.T) {
	dailyLimit := 12.5
	previous := &service.SystemSettings{
		RegistrationEmailSuffixWhitelist: []string{"@example.com"},
		LoginAgreementDocuments:          []service.LoginAgreementDocument{{ID: "terms", Title: "Terms", ContentMD: "Body"}},
		TablePageSizeOptions:             []int{10, 20},
		DefaultSubscriptions:             []service.DefaultSubscriptionSetting{{GroupID: 42, ValidityDays: 30}},
		DefaultPlatformQuotas: map[string]*service.DefaultPlatformQuotaSetting{
			"openai": {DailyLimitUSD: &dailyLimit},
		},
	}
	var req UpdateSettingsRequest

	mergeOmittedUpdateSettingsRequest(&req, previous, nil)

	require.Equal(t, []dto.LoginAgreementDocument{{ID: "terms", Title: "Terms", ContentMD: "Body"}}, req.LoginAgreementDocuments)
	require.Equal(t, []dto.DefaultSubscriptionSetting{{GroupID: 42, ValidityDays: 30}}, req.DefaultSubscriptions)

	req.RegistrationEmailSuffixWhitelist[0] = "@changed.example"
	req.LoginAgreementDocuments[0].Title = "Changed"
	req.TablePageSizeOptions[0] = 100
	req.DefaultSubscriptions[0].ValidityDays = 60
	*req.DefaultPlatformQuotas["openai"].DailyLimitUSD = 99
	delete(req.DefaultPlatformQuotas, "openai")

	require.Equal(t, []string{"@example.com"}, previous.RegistrationEmailSuffixWhitelist)
	require.Equal(t, "Terms", previous.LoginAgreementDocuments[0].Title)
	require.Equal(t, []int{10, 20}, previous.TablePageSizeOptions)
	require.Equal(t, 30, previous.DefaultSubscriptions[0].ValidityDays)
	require.Equal(t, 12.5, *previous.DefaultPlatformQuotas["openai"].DailyLimitUSD)
}

// Saving settings is a whole-document PUT. A client that sends only the field it
// cares about must not reset everything else: a payload as small as
// `{"risk_control_enabled":true}` used to clear site_name, after which
// getStringOrDefault rendered the empty value as the built-in default and the
// login page silently changed name.

func TestUpdateSettingsPartialPayloadKeepsUnsentKeys(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeySiteName:           "Example Gateway",
		service.SettingKeySiteSubtitle:       "Example Gateway Platform",
		service.SettingKeySMTPHost:           "smtp.example.com",
		service.SettingKeySMTPFrom:           "noreply@example.com",
		service.SettingKeyTurnstileEnabled:   "true",
		service.SettingKeyTurnstileSiteKey:   "site-key",
		service.SettingKeyTurnstileSecretKey: "secret-key",
	})

	rec := doUpdateSettings(t, h, map[string]any{"risk_control_enabled": true}, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, "true", repo.values[service.SettingKeyRiskControlEnabled],
		"the field the caller actually sent must be written")

	require.Equal(t, "Example Gateway", repo.values[service.SettingKeySiteName])
	require.Equal(t, "Example Gateway Platform", repo.values[service.SettingKeySiteSubtitle])
	require.Equal(t, "smtp.example.com", repo.values[service.SettingKeySMTPHost])
	require.Equal(t, "noreply@example.com", repo.values[service.SettingKeySMTPFrom])
	require.Equal(t, "true", repo.values[service.SettingKeyTurnstileEnabled])
}

func TestUpdateSettingsPartialPayloadValidatesAgainstStoredCaptchaProvider(t *testing.T) {
	stored := map[string]string{
		service.SettingKeyTurnstileEnabled:   "true",
		service.SettingKeyTurnstileSiteKey:   "site-key",
		service.SettingKeyTurnstileSecretKey: "secret-key",
	}
	h, repo := newStepUpSwitchTestHandler(t, maps.Clone(stored))

	rec := doUpdateSettings(t, h, map[string]any{"recaptcha_enabled": true}, nil)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Only one human verification provider")
	require.Equal(t, stored, repo.values)
	require.Nil(t, repo.lastUpdates)
}

func TestUpdateSettingsTencentCaptchaPreservesStoredSecretsWhenBlank(t *testing.T) {
	stored := map[string]string{
		service.SettingKeyTencentCaptchaAppSecretKey:   "stored-app-secret",
		service.SettingKeyTencentCaptchaCloudSecretID:  "stored-cloud-id",
		service.SettingKeyTencentCaptchaCloudSecretKey: "stored-cloud-secret",
	}
	h, repo := newStepUpSwitchTestHandler(t, maps.Clone(stored))

	rec := doUpdateSettings(t, h, map[string]any{
		"tencent_captcha_enabled":          true,
		"tencent_captcha_app_id":           "123456789",
		"tencent_captcha_app_secret_key":   "",
		"tencent_captcha_cloud_secret_id":  "",
		"tencent_captcha_cloud_secret_key": "",
	}, nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "true", repo.values[service.SettingKeyTencentCaptchaEnabled])
	require.Equal(t, "123456789", repo.values[service.SettingKeyTencentCaptchaAppID])
	require.Equal(t, "stored-app-secret", repo.values[service.SettingKeyTencentCaptchaAppSecretKey])
	require.Equal(t, "stored-cloud-id", repo.values[service.SettingKeyTencentCaptchaCloudSecretID])
	require.Equal(t, "stored-cloud-secret", repo.values[service.SettingKeyTencentCaptchaCloudSecretKey])
}

func TestUpdateSettingsPartialPayloadPreservesTencentCaptchaRegion(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeyTencentCaptchaRegion: service.TencentCaptchaRegionINTL,
	})

	rec := doUpdateSettings(t, h, map[string]any{"risk_control_enabled": true}, nil)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, service.TencentCaptchaRegionINTL, repo.values[service.SettingKeyTencentCaptchaRegion])
}

func TestUpdateSettingsNormalizesTencentCaptchaRegion(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{})

	rec := doUpdateSettings(t, h, map[string]any{"tencent_captcha_region": "unexpected"}, nil)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, service.TencentCaptchaRegionCN, repo.values[service.SettingKeyTencentCaptchaRegion])
}

func TestUpdateSettingsTencentCaptchaRejectsStoredProviderConflict(t *testing.T) {
	stored := map[string]string{
		service.SettingKeyTurnstileEnabled:   "true",
		service.SettingKeyTurnstileSiteKey:   "site-key",
		service.SettingKeyTurnstileSecretKey: "secret-key",
	}
	h, repo := newStepUpSwitchTestHandler(t, maps.Clone(stored))

	rec := doUpdateSettings(t, h, map[string]any{
		"tencent_captcha_enabled":          true,
		"tencent_captcha_app_id":           "123456789",
		"tencent_captcha_app_secret_key":   "app-secret",
		"tencent_captcha_cloud_secret_id":  "cloud-id",
		"tencent_captcha_cloud_secret_key": "cloud-secret",
	}, nil)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Only one human verification provider")
	require.Equal(t, stored, repo.values)
	require.Nil(t, repo.lastUpdates)
}

func TestUpdateSettingsPartialPayloadPreservesAliyunCaptchaConfiguration(t *testing.T) {
	stored := map[string]string{
		service.SettingKeyAliyunCaptchaEnabled:         "true",
		service.SettingKeyAliyunCaptchaAccessKeyID:     "stored-ak-id",
		service.SettingKeyAliyunCaptchaAccessKeySecret: "stored-ak-secret",
		service.SettingKeyAliyunCaptchaSceneID:         "scene-1",
		service.SettingKeyAliyunCaptchaPrefix:          "prefix-1",
		service.SettingKeyAliyunCaptchaRegion:          service.AliyunCaptchaRegionSGP,
	}
	h, repo := newStepUpSwitchTestHandler(t, maps.Clone(stored))

	rec := doUpdateSettings(t, h, map[string]any{"risk_control_enabled": true}, nil)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "true", repo.values[service.SettingKeyAliyunCaptchaEnabled])
	require.Equal(t, "stored-ak-id", repo.values[service.SettingKeyAliyunCaptchaAccessKeyID])
	require.Equal(t, "stored-ak-secret", repo.values[service.SettingKeyAliyunCaptchaAccessKeySecret])
	require.Equal(t, "scene-1", repo.values[service.SettingKeyAliyunCaptchaSceneID])
	require.Equal(t, "prefix-1", repo.values[service.SettingKeyAliyunCaptchaPrefix])
	require.Equal(t, service.AliyunCaptchaRegionSGP, repo.values[service.SettingKeyAliyunCaptchaRegion])
}

func TestUpdateSettingsRejectsAliyunCaptchaProviderConflictBeforeValidation(t *testing.T) {
	stored := map[string]string{
		service.SettingKeyTurnstileEnabled:   "true",
		service.SettingKeyTurnstileSiteKey:   "site-key",
		service.SettingKeyTurnstileSecretKey: "secret-key",
	}
	h, repo := newStepUpSwitchTestHandler(t, maps.Clone(stored))

	enabled := true
	rec := doUpdateSettings(t, h, map[string]any{
		"aliyun_captcha_enabled":           enabled,
		"aliyun_captcha_access_key_id":     "ak-id",
		"aliyun_captcha_access_key_secret": "ak-secret",
		"aliyun_captcha_scene_id":          "scene-1",
		"aliyun_captcha_prefix":            "prefix-1",
		"aliyun_captcha_region":            "cn",
	}, nil)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Only one human verification provider")
	require.Equal(t, stored, repo.values)
}

func TestUpdateSettingsPartialPayloadMergesStoredCrossFieldValues(t *testing.T) {
	stored := map[string]string{
		service.SettingKeyMinCodexVersion: "0.200.0",
	}
	h, repo := newStepUpSwitchTestHandler(t, maps.Clone(stored))

	rec := doUpdateSettings(t, h, map[string]any{"max_codex_version": "0.100.0"}, nil)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "max_codex_version must be greater than or equal to min_codex_version")
	require.Equal(t, stored, repo.values)
	require.Nil(t, repo.lastUpdates)
}

// A full payload keeps whole-document semantics: fields explicitly set to their
// zero value are still cleared.
func TestUpdateSettingsFullPayloadStillClearsSentEmptyFields(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeySiteName: "Example Gateway",
	})

	rec := doUpdateSettings(t, h, map[string]any{"site_name": ""}, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, "", repo.values[service.SettingKeySiteName],
		"an explicitly sent empty value is a deliberate clear, not an omission")
}

// smtp_from_email is the one request field whose JSON name differs from its
// setting key; the alias keeps it from being treated as always-omitted.
func TestUpdateSettingsSMTPFromAliasIsWritable(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeySMTPFrom: "old@example.com",
	})

	rec := doUpdateSettings(t, h, map[string]any{"smtp_from_email": "new@example.com"}, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, "new@example.com", repo.values[service.SettingKeySMTPFrom])
}

func TestUpdateSettingsPartialPayloadPreservesCodexVersions(t *testing.T) {
	stored := map[string]string{
		service.SettingKeyOpenAICodexClientVersion:       "0.150.0",
		service.SettingKeyOpenAICodexClientVersionSynced: "0.151.0",
	}
	h, repo := newStepUpSwitchTestHandler(t, maps.Clone(stored))

	rec := doUpdateSettings(t, h, map[string]any{"registration_enabled": true}, nil)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "0.150.0", repo.values[service.SettingKeyOpenAICodexClientVersion])
	require.Equal(t, "0.151.0", repo.values[service.SettingKeyOpenAICodexClientVersionSynced])
}

func TestUpdateSettingsRejectsInvalidCodexClientVersion(t *testing.T) {
	stored := map[string]string{
		service.SettingKeyOpenAICodexClientVersion: "0.150.0",
	}
	h, repo := newStepUpSwitchTestHandler(t, maps.Clone(stored))

	rec := doUpdateSettings(t, h, map[string]any{"openai_codex_client_version": "latest"}, nil)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "openai_codex_client_version must be empty or a valid version")
	require.Equal(t, stored, repo.values)
	require.Nil(t, repo.lastUpdates)
}

func TestUpdateSettingsCannotWriteSynchronizedCodexVersion(t *testing.T) {
	stored := map[string]string{
		service.SettingKeyOpenAICodexClientVersionSynced: "0.151.0",
	}
	h, repo := newStepUpSwitchTestHandler(t, maps.Clone(stored))

	rec := doUpdateSettings(t, h, map[string]any{
		"openai_codex_client_version_synced": "9.9.9",
		"registration_enabled":               true,
	}, nil)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "0.151.0", repo.values[service.SettingKeyOpenAICodexClientVersionSynced])
	require.NotContains(t, repo.lastUpdates, service.SettingKeyOpenAICodexClientVersionSynced)
}
