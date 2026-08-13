package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/antigravity"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

// OmittedSettingKeys marks setting keys the caller's payload never carried.
// SystemSettings is a plain struct, so a field the caller omitted arrives as a
// zero value and is indistinguishable from a deliberate clear. Listing the key
// here drops it from the write, leaving the stored value in place.
//
// A nil or empty set keeps whole-document semantics: every key is written.
type OmittedSettingKeys map[string]struct{}

func (o OmittedSettingKeys) dropFrom(updates map[string]string) {
	for key := range o {
		delete(updates, key)
	}
}

// UpdateSettings 更新系统设置
func (s *SettingService) UpdateSettings(ctx context.Context, settings *SystemSettings) error {
	return s.UpdateSettingsOmitting(ctx, settings, nil)
}

// UpdateSettingsOmitting persists system settings, leaving the keys in omitted
// at their stored value.
func (s *SettingService) UpdateSettingsOmitting(ctx context.Context, settings *SystemSettings, omitted OmittedSettingKeys) error {
	updates, err := s.buildSystemSettingsUpdates(ctx, settings)
	if err != nil {
		return err
	}
	omitted.dropFrom(updates)

	if err := s.settingRepo.SetMultiple(ctx, updates); err != nil {
		return err
	}
	return s.applySettingsRuntimeAfterWrite(ctx, settings, omitted)
}

// UpdateSettingsWithAuthSourceDefaults persists system settings and auth-source defaults in a single write.
func (s *SettingService) UpdateSettingsWithAuthSourceDefaults(ctx context.Context, settings *SystemSettings, authDefaults *AuthSourceDefaultSettings) error {
	return s.UpdateSettingsWithAuthSourceDefaultsOmitting(ctx, settings, authDefaults, nil)
}

// UpdateSettingsWithAuthSourceDefaultsOmitting persists system settings and
// auth-source defaults in a single write, leaving the keys in omitted at their
// stored value.
func (s *SettingService) UpdateSettingsWithAuthSourceDefaultsOmitting(ctx context.Context, settings *SystemSettings, authDefaults *AuthSourceDefaultSettings, omitted OmittedSettingKeys) error {
	updates, err := s.buildSystemSettingsUpdates(ctx, settings)
	if err != nil {
		return err
	}

	authSourceUpdates, err := s.buildAuthSourceDefaultUpdates(ctx, authDefaults)
	if err != nil {
		return err
	}
	for key, value := range authSourceUpdates {
		updates[key] = value
	}
	omitted.dropFrom(updates)

	if err := s.settingRepo.SetMultiple(ctx, updates); err != nil {
		return err
	}
	return s.applySettingsRuntimeAfterWrite(ctx, settings, omitted)
}

// applySettingsRuntimeAfterWrite preserves the runtime side effects of the
// existing whole-document update path. Partial requests refresh caches from the
// stored document because omitted value fields are zero in the request struct.
func (s *SettingService) applySettingsRuntimeAfterWrite(ctx context.Context, settings *SystemSettings, omitted OmittedSettingKeys) error {
	previousRequestPriority := s.RequestPriorityAdmissionSettingsSnapshot()
	settingsForRuntime := settings
	if len(omitted) > 0 {
		stored, err := s.getPersistedSystemSettings(ctx)
		if err != nil {
			slog.Warn("load merged settings after partial update failed", "error", err)
			if s.onUpdate != nil {
				s.onUpdate()
			}
			return nil
		}
		settingsForRuntime = stored
	}

	if err := s.applySchedulerEngineSettings(ctx, settingsForRuntime); err != nil {
		return err
	}

	s.refreshCachedSettings(settingsForRuntime)
	if previousRequestPriority != s.RequestPriorityAdmissionSettingsSnapshot() {
		s.publishRequestPriorityAdmissionSettingsUpdate(ctx)
	}
	return nil
}

func (s *SettingService) applySchedulerEngineSettings(ctx context.Context, settings *SystemSettings) error {
	if s.schedulerEngineSwitcher == nil {
		return nil
	}
	enabled := settings.SchedulerV2Enabled
	current := s.schedulerEngineSwitcher.SchedulerEngineState(ctx)
	previousCandidateLimit, previousScanLimit := s.schedulerEngineSwitcher.SchedulerV2Limits()
	limitsChanged := previousCandidateLimit != settings.SchedulerV2CandidateLimit || previousScanLimit != settings.SchedulerV2ScanLimit
	engineChanged := current.V2Enabled() != enabled || enabled && current.Status == SchedulerEngineStatusFailed
	if engineChanged {
		if err := s.schedulerEngineSwitcher.SetSchedulerV2Enabled(ctx, enabled, settings.SchedulerV2CandidateLimit, settings.SchedulerV2ScanLimit); err != nil {
			// SetMultiple has already persisted the requested target. Restore the
			// previous target so a restart cannot unexpectedly switch engines after
			// a failed runtime transition.
			_ = s.settingRepo.SetMultiple(ctx, map[string]string{
				SettingKeySchedulerV2Enabled:        strconv.FormatBool(current.V2Enabled()),
				SettingKeySchedulerV2CandidateLimit: strconv.Itoa(previousCandidateLimit),
				SettingKeySchedulerV2ScanLimit:      strconv.Itoa(previousScanLimit),
			})
			return fmt.Errorf("switch scheduler engine: %w", err)
		}
	} else if limitsChanged {
		if err := s.schedulerEngineSwitcher.ConfigureSchedulerV2Limits(ctx, settings.SchedulerV2CandidateLimit, settings.SchedulerV2ScanLimit); err != nil {
			_ = s.settingRepo.SetMultiple(ctx, map[string]string{
				SettingKeySchedulerV2CandidateLimit: strconv.Itoa(previousCandidateLimit),
				SettingKeySchedulerV2ScanLimit:      strconv.Itoa(previousScanLimit),
			})
			return fmt.Errorf("configure scheduler v2 limits: %w", err)
		}
	}
	return nil
}

func (s *SettingService) buildSystemSettingsUpdates(ctx context.Context, settings *SystemSettings) (map[string]string, error) {
	// Helper order preserves the legacy first-error and conditional-write sequence.
	clientIPTrustedProxiesJSON, err := s.prepareSystemSettingsUpdate(ctx, settings)
	if err != nil {
		return nil, err
	}

	updates := make(map[string]string)
	if err := writeRegistrationSystemSettingUpdates(updates, settings); err != nil {
		return nil, err
	}
	writeAccessSystemSettingUpdates(updates, settings, clientIPTrustedProxiesJSON)

	writeIdentitySystemSettingUpdates(updates, settings)

	if err := writeProductSystemSettingUpdates(updates, settings); err != nil {
		return nil, err
	}

	writeFeatureSystemSettingUpdates(updates, settings)

	if err := writeGatewaySystemSettingUpdates(updates, settings); err != nil {
		return nil, err
	}

	if err := writeNotificationSystemSettingUpdates(updates, settings); err != nil {
		return nil, err
	}

	return updates, nil
}

// validateDefaultPlatformQuotaMap 校验 platform quota map 的合法性：
// 平台名须在 AllowedQuotaPlatforms 白名单内，每个非 nil 上限须 finite 且 >= 0。
// 系统层和 auth-source 层共用此 helper。
func validateDefaultPlatformQuotaMap(m map[string]*DefaultPlatformQuotaSetting) error {
	for platform, pq := range m {
		if !IsAllowedQuotaPlatform(platform) {
			return infraerrors.BadRequest("INVALID_DEFAULT_PLATFORM_QUOTA", fmt.Sprintf("unknown platform %q", platform))
		}
		if pq == nil {
			continue
		}
		for _, v := range []*float64{pq.DailyLimitUSD, pq.WeeklyLimitUSD, pq.MonthlyLimitUSD} {
			if v != nil && (*v < 0 || math.IsNaN(*v) || math.IsInf(*v, 0)) {
				return infraerrors.BadRequest("INVALID_DEFAULT_PLATFORM_QUOTA", "platform quota limit must be a finite non-negative number")
			}
		}
	}
	return nil
}

func (s *SettingService) buildAuthSourceDefaultUpdates(ctx context.Context, settings *AuthSourceDefaultSettings) (map[string]string, error) {
	if settings == nil {
		return nil, nil
	}

	for _, subscriptions := range [][]DefaultSubscriptionSetting{
		settings.Email.Subscriptions,
		settings.LinuxDo.Subscriptions,
		settings.OIDC.Subscriptions,
		settings.WeChat.Subscriptions,
		settings.GitHub.Subscriptions,
		settings.Google.Subscriptions,
		settings.DingTalk.Subscriptions,
	} {
		if err := s.validateDefaultSubscriptionGroups(ctx, subscriptions); err != nil {
			return nil, err
		}
	}

	// 校验各 auth source 的 platform quota map（改动 C：对等系统层校验）
	for _, pgs := range []struct {
		name string
		pq   map[string]*DefaultPlatformQuotaSetting
	}{
		{"email", settings.Email.PlatformQuotas},
		{"linuxdo", settings.LinuxDo.PlatformQuotas},
		{"oidc", settings.OIDC.PlatformQuotas},
		{"wechat", settings.WeChat.PlatformQuotas},
		{"github", settings.GitHub.PlatformQuotas},
		{"google", settings.Google.PlatformQuotas},
		{"dingtalk", settings.DingTalk.PlatformQuotas},
	} {
		if pgs.pq != nil {
			if err := validateDefaultPlatformQuotaMap(pgs.pq); err != nil {
				return nil, err
			}
		}
	}

	updates := make(map[string]string, 36)
	writeProviderDefaultGrantUpdates(updates, emailAuthSourceDefaultKeys, settings.Email)
	writeProviderDefaultGrantUpdates(updates, linuxDoAuthSourceDefaultKeys, settings.LinuxDo)
	writeProviderDefaultGrantUpdates(updates, oidcAuthSourceDefaultKeys, settings.OIDC)
	writeProviderDefaultGrantUpdates(updates, weChatAuthSourceDefaultKeys, settings.WeChat)
	writeProviderDefaultGrantUpdates(updates, gitHubAuthSourceDefaultKeys, settings.GitHub)
	writeProviderDefaultGrantUpdates(updates, googleAuthSourceDefaultKeys, settings.Google)
	writeProviderDefaultGrantUpdates(updates, dingTalkAuthSourceDefaultKeys, settings.DingTalk)
	updates[SettingKeyForceEmailOnThirdPartySignup] = strconv.FormatBool(settings.ForceEmailOnThirdPartySignup)
	return updates, nil
}

func (s *SettingService) refreshCachedSettings(settings *SystemSettings) {
	if settings == nil {
		return
	}

	// 先使 inflight singleflight 失效，再刷新缓存，缩小旧值覆盖新值的竞态窗口
	versionBoundsSF.Forget("version_bounds")
	versionBoundsCache.Store(&cachedVersionBounds{
		min:       settings.MinClaudeCodeVersion,
		max:       settings.MaxClaudeCodeVersion,
		expiresAt: time.Now().Add(versionBoundsCacheTTL).UnixNano(),
	})
	backendModeSF.Forget("backend_mode")
	backendModeCache.Store(&cachedBackendMode{
		value:     settings.BackendModeEnabled,
		expiresAt: time.Now().Add(backendModeCacheTTL).UnixNano(),
	})
	s.supportChatCacheMu.Lock()
	s.supportChatSF.Forget(supportChatRefreshKey)
	s.supportChatCache.Store(&cachedSupportChatEnabled{
		value:     settings.SupportChatEnabled,
		expiresAt: time.Now().Add(supportChatCacheTTL).UnixNano(),
	})
	s.supportChatCacheMu.Unlock()
	s.channelMonitorPublicShareCacheMu.Lock()
	s.channelMonitorPublicShareSF.Forget(channelMonitorPublicShareRefreshKey)
	s.channelMonitorPublicShareCache.Store(&cachedChannelMonitorPublicShareRuntime{
		value: ChannelMonitorPublicShareRuntime{
			Enabled:     settings.ChannelMonitorEnabled && settings.ChannelMonitorPublicShareEnabled,
			RequireAuth: settings.ChannelMonitorPublicShareRequireAuth,
		},
		expiresAt: time.Now().Add(channelMonitorPublicShareCacheTTL).UnixNano(),
	})
	s.channelMonitorPublicShareCacheMu.Unlock()
	s.streamModePerformanceEnabled.Store(settings.StreamModePerformanceEnabled)
	s.streamModePerformanceLoaded.Store(time.Now().UnixNano())
	s.openAIWSModeRouterV2Enabled.Store(settings.OpenAIWSModeRouterV2Enabled)
	s.openAIWSModeRouterV2Loaded.Store(time.Now().UnixNano())
	s.storeRequestPriorityAdmissionSettings(RequestPriorityAdmissionSettings{
		Enabled:                 settings.RequestPriorityAdmissionEnabled,
		PendingLimitPerInstance: settings.RequestPriorityPendingLimitPerInstance,
		PendingMiBPerInstance:   settings.RequestPriorityPendingMiBPerInstance,
	})
	gatewayForwardingSF.Forget("gateway_forwarding")
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{
		fingerprintUnification:           settings.EnableFingerprintUnification,
		metadataPassthrough:              settings.EnableMetadataPassthrough,
		cchSigning:                       settings.EnableCCHSigning,
		claudeOAuthSystemPromptInjection: settings.EnableClaudeOAuthSystemPromptInjection,
		claudeOAuthSystemPrompt:          settings.ClaudeOAuthSystemPrompt,
		claudeOAuthSystemPromptBlocks:    settings.ClaudeOAuthSystemPromptBlocks,
		anthropicCacheTTL1hInjection:     settings.EnableAnthropicCacheTTL1hInjection,
		rewriteMessageCacheControl:       settings.RewriteMessageCacheControl,
		clientDatelineNormalization:      settings.EnableClientDatelineNormalization,
		openAIVisibleOutputTTFT:          settings.OpenAIVisibleOutputTTFTEnabled,
		expiresAt:                        time.Now().Add(gatewayForwardingCacheTTL).UnixNano(),
	})
	s.antigravityUAVersionSF.Forget("antigravity_user_agent_version")
	antigravityUserAgentVersion := antigravity.NormalizeUserAgentVersion(settings.AntigravityUserAgentVersion)
	if antigravityUserAgentVersion == "" {
		antigravityUserAgentVersion = antigravity.GetDefaultUserAgentVersion()
	}
	s.antigravityUAVersionCache.Store(&cachedAntigravityUserAgentVersion{
		version:   antigravityUserAgentVersion,
		expiresAt: time.Now().Add(antigravityUserAgentVersionCacheTTL).UnixNano(),
	})
	s.openAICodexUASF.Forget("openai_codex_user_agent")
	codexUA := strings.TrimSpace(settings.OpenAICodexUserAgent)
	if codexUA == "" {
		codexUA = DefaultOpenAICodexUserAgent
	}
	s.openAICodexUACache.Store(&cachedOpenAICodexUserAgent{
		value:     codexUA,
		expiresAt: time.Now().Add(openAICodexUserAgentCacheTTL).UnixNano(),
	})
	// The synchronized value is owned by the background task and is not part of
	// this update snapshot, so invalidate instead of rebuilding from stale data.
	s.InvalidateOpenAICodexClientVersionCache()
	openAIAdvancedSchedulerSettingSF.Forget(openAIAdvancedSchedulerSettingKey)
	openAIAdvancedSchedulerSettingCache.Store(&cachedOpenAIAdvancedSchedulerSetting{
		lowUpstreamRatePriorityEnabled: settings.OpenAILowUpstreamRatePriorityEnabled,
		oauthSchedulingRateMultiplier:  settings.OpenAIOAuthSchedulingRateMultiplier,
		contentSessionBurstBalance:     settings.OpenAIContentSessionBurstBalanceEnabled,
		enabled:                        settings.OpenAIAdvancedSchedulerEnabled,
		stickyWeightedEnabled:          settings.OpenAIAdvancedSchedulerStickyWeightedEnabled,
		subscriptionPriorityEnabled:    settings.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled,
		lbTopKOverride:                 parsePositiveIntOverride(settings.OpenAIAdvancedSchedulerLBTopK),
		weightOverrides: parseOpenAIAdvancedSchedulerWeightOverrides(map[string]string{
			SettingKeyOpenAIAdvancedSchedulerWeightPriority:         settings.OpenAIAdvancedSchedulerWeightPriority,
			SettingKeyOpenAIAdvancedSchedulerWeightLoad:             settings.OpenAIAdvancedSchedulerWeightLoad,
			SettingKeyOpenAIAdvancedSchedulerWeightQueue:            settings.OpenAIAdvancedSchedulerWeightQueue,
			SettingKeyOpenAIAdvancedSchedulerWeightErrorRate:        settings.OpenAIAdvancedSchedulerWeightErrorRate,
			SettingKeyOpenAIAdvancedSchedulerWeightTTFT:             settings.OpenAIAdvancedSchedulerWeightTTFT,
			SettingKeyOpenAIAdvancedSchedulerWeightReset:            settings.OpenAIAdvancedSchedulerWeightReset,
			SettingKeyOpenAIAdvancedSchedulerWeightQuotaHeadroom:    settings.OpenAIAdvancedSchedulerWeightQuotaHeadroom,
			SettingKeyOpenAIAdvancedSchedulerWeightUpstreamCost:     settings.OpenAIAdvancedSchedulerWeightUpstreamCost,
			SettingKeyOpenAIAdvancedSchedulerWeightPreviousResponse: settings.OpenAIAdvancedSchedulerWeightPreviousResponse,
			SettingKeyOpenAIAdvancedSchedulerWeightSessionSticky:    settings.OpenAIAdvancedSchedulerWeightSessionSticky,
		}),
		expiresAt: time.Now().Add(openAIAdvancedSchedulerSettingCacheTTL).UnixNano(),
	})
	// Invalidate the quota auto-pause cache and let the next read trigger a fresh load.
	// We can't know from here whether ops_advanced_settings was also touched, so be
	// defensive: store an expired entry — GetOpenAIQuotaAutoPauseSettings will serve
	// stale and kick off an async refresh, never blocking the request that follows.
	s.openAIQuotaAutoPauseSettingsSF.Forget(openAIQuotaAutoPauseSettingsRefreshKey)
	if cached, _ := s.openAIQuotaAutoPauseSettingsCache.Load().(*cachedOpenAIQuotaAutoPauseSettings); cached != nil {
		s.openAIQuotaAutoPauseSettingsCache.Store(&cachedOpenAIQuotaAutoPauseSettings{
			settings:  cached.settings,
			expiresAt: 0,
		})
	}
	if s.clientIPResolver != nil {
		_ = s.clientIPResolver.Configure(settings.ClientIPResolutionMode, settings.ClientIPTrustedProxies)
	}
	// codex_cli_only 加固策略缓存：设置更新后强制下次重载（涉及 4 个键 + JSON 解析，直接置过期）。
	s.codexRestrictionPolicySF.Forget("codex_restriction_policy")
	s.codexRestrictionPolicyCache.Store(&cachedCodexRestrictionPolicy{expiresAt: 0})
	if s.onUpdate != nil {
		s.onUpdate() // Invalidate cache after settings update
	}
}

func (s *SettingService) defaultRewriteMessageCacheControl() bool {
	return false
}

func (s *SettingService) validateDefaultSubscriptionGroups(ctx context.Context, items []DefaultSubscriptionSetting) error {
	if len(items) == 0 {
		return nil
	}

	checked := make(map[int64]struct{}, len(items))
	for _, item := range items {
		if item.GroupID <= 0 {
			continue
		}
		if _, ok := checked[item.GroupID]; ok {
			return ErrDefaultSubGroupDuplicate.WithMetadata(map[string]string{
				"group_id": strconv.FormatInt(item.GroupID, 10),
			})
		}
		checked[item.GroupID] = struct{}{}
		if s.defaultSubGroupReader == nil {
			continue
		}

		group, err := s.defaultSubGroupReader.GetByID(ctx, item.GroupID)
		if err != nil {
			if errors.Is(err, ErrGroupNotFound) {
				return ErrDefaultSubGroupInvalid.WithMetadata(map[string]string{
					"group_id": strconv.FormatInt(item.GroupID, 10),
				})
			}
			return fmt.Errorf("get default subscription group %d: %w", item.GroupID, err)
		}
		if !group.IsSubscriptionType() {
			return ErrDefaultSubGroupInvalid.WithMetadata(map[string]string{
				"group_id": strconv.FormatInt(item.GroupID, 10),
			})
		}
	}

	return nil
}
