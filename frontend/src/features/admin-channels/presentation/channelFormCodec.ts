import type {
  AccountStatsPricingRule,
  Channel,
  ChannelModelPricing,
} from '@/features/admin-channels/data/datasources/adminChannelsDatasource'
import {
  apiIntervalsToForm,
  formIntervalsToAPI,
  mTokToPerToken,
  perTokenToMTok,
  type PricingFormEntry,
} from '@/features/admin-channels/presentation/adminChannelSignals'
import type { AdminGroup, GroupPlatform } from '@/types'

export interface FormPricingRule {
  name: string
  group_ids: number[]
  account_ids: number[]
  pricing: PricingFormEntry[]
}

export interface PlatformSection {
  platform: GroupPlatform
  enabled: boolean
  collapsed: boolean
  group_ids: number[]
  model_mapping: Record<string, string>
  model_pricing: PricingFormEntry[]
  web_search_emulation: boolean
  codex_image_generation_bridge: boolean
  bedrock_cc_compat: boolean
  account_stats_pricing_rules: FormPricingRule[]
}

export interface ChannelAPIFields {
  group_ids: number[]
  model_pricing: ChannelModelPricing[]
  model_mapping: Record<string, Record<string, string>>
  features_config: Record<string, unknown>
}

export const channelPlatformOrder: GroupPlatform[] = [
  'anthropic',
  'openai',
  'gemini',
  'antigravity',
  'grok',
]

function pricingFormEntryToAPI(
  entry: PricingFormEntry,
  platform: GroupPlatform,
): ChannelModelPricing {
  return {
    platform,
    models: entry.models,
    billing_mode: entry.billing_mode,
    input_price: mTokToPerToken(entry.input_price),
    output_price: mTokToPerToken(entry.output_price),
    cache_write_price: mTokToPerToken(entry.cache_write_price),
    cache_read_price: mTokToPerToken(entry.cache_read_price),
    image_input_price: mTokToPerToken(entry.image_input_price),
    image_output_price: mTokToPerToken(entry.image_output_price),
    per_request_price:
      entry.per_request_price != null && entry.per_request_price !== ''
        ? Number(entry.per_request_price)
        : null,
    intervals: formIntervalsToAPI(entry.intervals || []),
    ...(entry.time_pricing && entry.time_pricing.periods.length > 0
      ? {
          time_pricing: {
            timezone: entry.time_pricing.timezone,
            periods: entry.time_pricing.periods.map(period => ({
              start_time: period.start_time,
              end_time: period.end_time,
              multiplier: Number(period.multiplier)
            }))
          }
        }
      : {}),
  }
}

function pricingAPIEntryToForm(
  entry: ChannelModelPricing,
  cloneModels = false,
): PricingFormEntry {
  return {
    models: cloneModels ? [...(entry.models || [])] : entry.models || [],
    billing_mode: entry.billing_mode,
    input_price: perTokenToMTok(entry.input_price),
    output_price: perTokenToMTok(entry.output_price),
    cache_write_price: perTokenToMTok(entry.cache_write_price),
    cache_read_price: perTokenToMTok(entry.cache_read_price),
    image_input_price: perTokenToMTok(entry.image_input_price),
    image_output_price: perTokenToMTok(entry.image_output_price),
    per_request_price: entry.per_request_price,
    intervals: apiIntervalsToForm(entry.intervals || []),
    time_pricing: entry.time_pricing
      ? {
          timezone: entry.time_pricing.timezone,
          periods: (entry.time_pricing.periods || []).map(period => ({ ...period }))
        }
      : null,
  }
}

export function buildAccountStatsPricingRules(
  sections: PlatformSection[],
): AccountStatsPricingRule[] {
  const rules: AccountStatsPricingRule[] = []
  for (const section of sections) {
    if (!section.enabled) continue
    for (const rule of section.account_stats_pricing_rules) {
      rules.push({
        name: rule.name,
        group_ids: rule.group_ids,
        account_ids: rule.account_ids,
        pricing: rule.pricing
          .filter(entry => entry.models.length > 0)
          .map(entry => pricingFormEntryToAPI(entry, section.platform)),
      })
    }
  }
  return rules
}

export function buildChannelAPIFields(
  sections: PlatformSection[],
  existingFeaturesConfig?: Record<string, unknown>,
): ChannelAPIFields {
  const groupIds: number[] = []
  const modelPricing: ChannelModelPricing[] = []
  const modelMapping: Record<string, Record<string, string>> = {}
  const featuresConfig: Record<string, unknown> = existingFeaturesConfig
    ? { ...existingFeaturesConfig }
    : {}

  for (const section of sections) {
    if (!section.enabled) continue
    groupIds.push(...section.group_ids)

    if (Object.keys(section.model_mapping).length > 0) {
      modelMapping[section.platform] = { ...section.model_mapping }
    }

    for (const entry of section.model_pricing) {
      if (entry.models.length === 0) continue
      modelPricing.push(pricingFormEntryToAPI(entry, section.platform))
    }
  }

  const webSearchEmulation: Record<string, boolean> = {}
  const codexImageGenerationBridge: Record<string, boolean> = {}
  const bedrockCCCompat: Record<string, boolean> = {}
  for (const section of sections) {
    if (!section.enabled) continue
    if (section.platform === 'anthropic') {
      webSearchEmulation[section.platform] = !!section.web_search_emulation
      bedrockCCCompat[section.platform] = !!section.bedrock_cc_compat
    }
    if (section.platform === 'openai') {
      codexImageGenerationBridge[section.platform] = !!section.codex_image_generation_bridge
    }
  }

  if (Object.keys(webSearchEmulation).length > 0) {
    featuresConfig.web_search_emulation = webSearchEmulation
  } else {
    delete featuresConfig.web_search_emulation
  }
  if (Object.keys(codexImageGenerationBridge).length > 0) {
    featuresConfig.codex_image_generation_bridge = codexImageGenerationBridge
  } else {
    delete featuresConfig.codex_image_generation_bridge
  }
  if (Object.keys(bedrockCCCompat).length > 0) {
    featuresConfig.bedrock_cc_compat = bedrockCCCompat
  } else {
    delete featuresConfig.bedrock_cc_compat
  }

  return {
    group_ids: Array.from(new Set(groupIds)),
    model_pricing: modelPricing,
    model_mapping: modelMapping,
    features_config: featuresConfig,
  }
}

export function channelToPlatformSections(
  channel: Channel,
  groups: AdminGroup[],
  platformOrder: GroupPlatform[] = channelPlatformOrder,
): PlatformSection[] {
  const groupPlatformMap = new Map<number, GroupPlatform>()
  for (const group of groups) {
    groupPlatformMap.set(group.id, group.platform)
  }

  const activePlatforms = new Set<GroupPlatform>()
  for (const groupId of channel.group_ids || []) {
    const platform = groupPlatformMap.get(groupId)
    if (platform === 'composite') {
      platformOrder.forEach(candidate => activePlatforms.add(candidate))
    } else if (platform) {
      activePlatforms.add(platform)
    }
  }
  for (const pricing of channel.model_pricing || []) {
    if (pricing.platform) activePlatforms.add(pricing.platform as GroupPlatform)
  }
  for (const platform of Object.keys(channel.model_mapping || {})) {
    if (platformOrder.includes(platform as GroupPlatform)) {
      activePlatforms.add(platform as GroupPlatform)
    }
  }

  const sections: PlatformSection[] = []
  for (const platform of platformOrder) {
    if (!activePlatforms.has(platform)) continue

    const groupIds = (channel.group_ids || []).filter(groupId => {
      const groupPlatform = groupPlatformMap.get(groupId)
      return groupPlatform === platform || groupPlatform === 'composite'
    })
    const pricing = (channel.model_pricing || [])
      .filter(entry => (entry.platform || 'anthropic') === platform)
      .map(entry => pricingAPIEntryToForm(entry))
    const featuresConfig = channel.features_config
    const webSearchEmulation = featuresConfig?.web_search_emulation as
      | Record<string, boolean>
      | undefined
    const codexImageGenerationBridge = featuresConfig?.codex_image_generation_bridge as
      | Record<string, boolean>
      | undefined

    sections.push({
      platform,
      enabled: true,
      collapsed: false,
      group_ids: groupIds,
      model_mapping: { ...(channel.model_mapping || {})[platform] },
      model_pricing: pricing,
      web_search_emulation: webSearchEmulation?.[platform] === true,
      codex_image_generation_bridge: codexImageGenerationBridge?.[platform] === true,
      bedrock_cc_compat: featuresConfig?.bedrock_cc_compat === true,
      account_stats_pricing_rules: [],
    })
  }

  return sections
}

export function distributeAccountStatsPricingRules(
  sections: PlatformSection[],
  apiRules: AccountStatsPricingRule[],
  groups: AdminGroup[],
): void {
  const groupPlatformMap = new Map<number, GroupPlatform>()
  for (const group of groups) {
    groupPlatformMap.set(group.id, group.platform)
  }

  for (const apiRule of apiRules) {
    const platforms = new Set<GroupPlatform>()
    for (const groupId of apiRule.group_ids || []) {
      const platform = groupPlatformMap.get(groupId)
      if (platform && platform !== 'composite') platforms.add(platform)
    }
    if (platforms.size === 0 && apiRule.pricing?.length > 0) {
      const platform = apiRule.pricing[0].platform as GroupPlatform | undefined
      if (platform) platforms.add(platform)
    }
    const targetPlatform = platforms.size >= 1 ? [...platforms][0] : null
    if (!targetPlatform) continue

    const section = sections.find(candidate => candidate.platform === targetPlatform)
    if (!section) continue
    section.account_stats_pricing_rules.push({
      name: apiRule.name || '',
      group_ids: [...(apiRule.group_ids || [])],
      account_ids: [...(apiRule.account_ids || [])],
      pricing: (apiRule.pricing || []).map(entry => pricingAPIEntryToForm(entry, true)),
    })
  }
}
