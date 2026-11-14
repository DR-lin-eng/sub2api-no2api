package admin

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler/dto"
)

// settingKeyJSONAliases contains request fields whose JSON name differs from
// the setting key written by SettingService. All other value-typed fields use
// their JSON name as the persisted key.
var settingKeyJSONAliases = map[string]string{
	"smtp_from_email": service.SettingKeySMTPFrom,
}

// settingKeyByJSONName is built from the request tags so newly added
// value-typed fields participate in presence tracking automatically. Pointer
// fields already distinguish omission from an explicit zero value and retain
// their existing merge semantics in UpdateSettings.
var settingKeyByJSONName = buildSettingKeyByJSONName()

// These value-typed fields are owned by dedicated settings services. Their
// nil value carries presence semantics, so they must not be filled from
// SystemSettings when a partial system-settings payload omits them.
var omittedUpdateSettingsMergeExclusions = map[string]struct{}{
	"PaymentEnabledTypes":              {},
	"AuthSourceEmailPlatformQuotas":    {},
	"AuthSourceLinuxDoPlatformQuotas":  {},
	"AuthSourceOIDCPlatformQuotas":     {},
	"AuthSourceWeChatPlatformQuotas":   {},
	"AuthSourceGitHubPlatformQuotas":   {},
	"AuthSourceGooglePlatformQuotas":   {},
	"AuthSourceDingTalkPlatformQuotas": {},
}

type omittedUpdateSettingsFieldConverter func(*service.SystemSettings) any

// Most request and service fields have identical Go types. Converters keep DTO
// boundaries explicit and clone reference-typed fields so later normalization
// cannot mutate the previous-settings snapshot through shared storage.
var omittedUpdateSettingsFieldConverters = map[string]omittedUpdateSettingsFieldConverter{
	"RegistrationEmailSuffixWhitelist": func(settings *service.SystemSettings) any {
		return slices.Clone(settings.RegistrationEmailSuffixWhitelist)
	},
	"LoginAgreementDocuments": func(settings *service.SystemSettings) any {
		return loginAgreementDocumentsToDTO(settings.LoginAgreementDocuments)
	},
	"TablePageSizeOptions": func(settings *service.SystemSettings) any {
		return slices.Clone(settings.TablePageSizeOptions)
	},
	"DefaultSubscriptions": func(settings *service.SystemSettings) any {
		return defaultSubscriptionsToDTO(settings.DefaultSubscriptions)
	},
	"DefaultPlatformQuotas": func(settings *service.SystemSettings) any {
		return cloneDefaultPlatformQuotas(settings.DefaultPlatformQuotas)
	},
}

func buildSettingKeyByJSONName() map[string]string {
	t := reflect.TypeOf(UpdateSettingsRequest{})
	out := make(map[string]string, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Type.Kind() == reflect.Ptr {
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		if alias, ok := settingKeyJSONAliases[name]; ok {
			out[name] = alias
			continue
		}
		out[name] = name
	}
	return out
}

// mergeOmittedUpdateSettingsRequest overlays the current settings document on
// value-typed request fields that were absent from a partial payload. This must
// run before validation and side effects so cross-field rules see the state
// that will exist after the write. Pointer fields retain their existing
// omission semantics and are merged at their individual call sites.
func mergeOmittedUpdateSettingsRequest(
	req *UpdateSettingsRequest,
	previous *service.SystemSettings,
	sentFields map[string]json.RawMessage,
) {
	if req == nil || previous == nil {
		return
	}

	requestValue := reflect.ValueOf(req).Elem()
	requestType := requestValue.Type()
	previousValue := reflect.ValueOf(previous).Elem()
	for i := 0; i < requestType.NumField(); i++ {
		field := requestType.Field(i)
		if field.Type.Kind() == reflect.Ptr {
			continue
		}
		jsonName, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if jsonName == "" || jsonName == "-" {
			continue
		}
		if _, sent := sentFields[jsonName]; sent {
			continue
		}
		if _, excluded := omittedUpdateSettingsMergeExclusions[field.Name]; excluded {
			continue
		}

		destination := requestValue.Field(i)
		if converter, ok := omittedUpdateSettingsFieldConverters[field.Name]; ok {
			converted := reflect.ValueOf(converter(previous))
			if converted.IsValid() && converted.Type().AssignableTo(destination.Type()) {
				destination.Set(converted)
			}
			continue
		}
		source := previousValue.FieldByName(field.Name)
		if source.IsValid() && source.Type().AssignableTo(destination.Type()) {
			destination.Set(source)
		}
	}
}

func cloneDefaultPlatformQuotas(
	items map[string]*service.DefaultPlatformQuotaSetting,
) map[string]*service.DefaultPlatformQuotaSetting {
	if items == nil {
		return nil
	}
	result := make(map[string]*service.DefaultPlatformQuotaSetting, len(items))
	for platform, item := range items {
		if item == nil {
			result[platform] = nil
			continue
		}
		cloned := *item
		cloned.DailyLimitUSD = cloneFloat64Pointer(item.DailyLimitUSD)
		cloned.WeeklyLimitUSD = cloneFloat64Pointer(item.WeeklyLimitUSD)
		cloned.MonthlyLimitUSD = cloneFloat64Pointer(item.MonthlyLimitUSD)
		result[platform] = &cloned
	}
	return result
}

func cloneFloat64Pointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func defaultSubscriptionsToDTO(items []service.DefaultSubscriptionSetting) []dto.DefaultSubscriptionSetting {
	result := make([]dto.DefaultSubscriptionSetting, 0, len(items))
	for _, item := range items {
		result = append(result, dto.DefaultSubscriptionSetting{
			GroupID:      item.GroupID,
			ValidityDays: item.ValidityDays,
		})
	}
	return result
}

// omittedSettingKeys reports setting keys that the JSON payload did not carry.
// An explicitly present zero value remains a deliberate write.
func omittedSettingKeys(sentFields map[string]json.RawMessage) service.OmittedSettingKeys {
	omitted := make(service.OmittedSettingKeys, len(settingKeyByJSONName))
	for jsonName, settingKey := range settingKeyByJSONName {
		if _, sent := sentFields[jsonName]; !sent {
			omitted[settingKey] = struct{}{}
		}
	}
	// These runtime switches also use presence at the persistence boundary:
	// an unrelated save prepared before another admin's toggle must not write
	// the earlier snapshot back over that toggle.
	for _, key := range []string{service.SettingKeyOpenAIOAuthAttestationEnabled, service.SettingKeyOpenAILenovoAttributionEnabled} {
		if raw, sent := sentFields[key]; !sent || strings.TrimSpace(string(raw)) == "null" {
			omitted[key] = struct{}{}
		}
	}
	return omitted
}
