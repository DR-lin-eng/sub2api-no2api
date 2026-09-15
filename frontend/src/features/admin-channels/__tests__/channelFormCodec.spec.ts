import { describe, expect, it } from 'vitest'
import type {
  AccountStatsPricingRule,
  Channel,
  ChannelModelPricing,
} from '../data/datasources/adminChannelsDatasource'
import type { PricingFormEntry } from '../presentation/adminChannelSignals'
import {
  buildAccountStatsPricingRules,
  buildChannelAPIFields,
  channelToPlatformSections,
  distributeAccountStatsPricingRules,
  type PlatformSection,
} from '../presentation/channelFormCodec'
import type { AdminGroup, GroupPlatform } from '@/types'

function makePricing(overrides: Partial<PricingFormEntry> = {}): PricingFormEntry {
  return {
    models: ['model-a'],
    billing_mode: 'token',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_input_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: [],
    ...overrides,
  }
}

function makeSection(
  platform: GroupPlatform,
  overrides: Partial<PlatformSection> = {},
): PlatformSection {
  return {
    platform,
    enabled: true,
    collapsed: false,
    group_ids: [],
    model_mapping: {},
    model_pricing: [],
    web_search_emulation: false,
    codex_image_generation_bridge: false,
    bedrock_cc_compat: false,
    account_stats_pricing_rules: [],
    ...overrides,
  }
}

function makeAPIpricing(
  platform: string,
  overrides: Partial<ChannelModelPricing> = {},
): ChannelModelPricing {
  return {
    platform,
    models: ['model-a'],
    billing_mode: 'token',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_input_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: [],
    ...overrides,
  }
}

function makeChannel(overrides: Partial<Channel> = {}): Channel {
  return {
    id: 1,
    name: 'Channel',
    description: '',
    status: 'active',
    billing_model_source: 'channel_mapped',
    restrict_models: false,
    group_ids: [],
    model_pricing: [],
    model_mapping: {},
    apply_pricing_to_account_stats: false,
    account_stats_pricing_rules: [],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeGroup(id: number, platform: GroupPlatform): AdminGroup {
  return { id, platform, name: `${platform}-${id}` } as AdminGroup
}

describe('channel form codec', () => {
  it('serializes enabled sections with stable keys, ordering, and price units', () => {
    const sections = [
      makeSection('anthropic', {
        group_ids: [1, 2],
        model_mapping: { 'claude-*': 'claude-sonnet' },
        model_pricing: [
          makePricing({
            models: ['claude-a', 'claude-b'],
            input_price: '5',
            output_price: 10,
            per_request_price: '0.25',
            intervals: [{
              min_tokens: 0,
              max_tokens: 1000,
              tier_label: 'base',
              input_price: 7,
              output_price: null,
              cache_write_price: null,
              cache_read_price: null,
              per_request_price: '0.10',
              sort_order: 0,
            }],
          }),
          makePricing({ models: [] }),
        ],
        web_search_emulation: true,
        bedrock_cc_compat: true,
      }),
      makeSection('openai', {
        group_ids: [2, 3],
        model_pricing: [makePricing({ models: ['gpt-a'] })],
        codex_image_generation_bridge: true,
      }),
      makeSection('gemini', {
        enabled: false,
        group_ids: [4],
        model_mapping: { ignored: 'ignored' },
        model_pricing: [makePricing({ models: ['ignored'] })],
      }),
    ]
    const existingFeatures = {
      untouched: { enabled: true },
      web_search_emulation: { stale: true },
      codex_image_generation_bridge: { stale: true },
      bedrock_cc_compat: { stale: true },
    }

    const result = buildChannelAPIFields(sections, existingFeatures)

    expect(Object.keys(result)).toStrictEqual([
      'group_ids',
      'model_pricing',
      'model_mapping',
      'features_config',
    ])
    expect(result.group_ids).toStrictEqual([1, 2, 3])
    expect(result.model_mapping).toStrictEqual({
      anthropic: { 'claude-*': 'claude-sonnet' },
    })
    expect(result.model_pricing.map(entry => [entry.platform, entry.models])).toStrictEqual([
      ['anthropic', ['claude-a', 'claude-b']],
      ['openai', ['gpt-a']],
    ])
    expect(Object.keys(result.model_pricing[0])).toStrictEqual([
      'platform',
      'models',
      'billing_mode',
      'input_price',
      'output_price',
      'cache_write_price',
      'cache_read_price',
      'image_input_price',
      'image_output_price',
      'per_request_price',
      'intervals',
    ])
    expect(result.model_pricing[0]).toMatchObject({
      input_price: 0.000005,
      output_price: 0.00001,
      per_request_price: 0.25,
    })
    expect(result.model_pricing[0].intervals[0]).toMatchObject({
      input_price: 0.000007,
      per_request_price: 0.1,
    })
    expect(result.features_config).toStrictEqual({
      untouched: { enabled: true },
      web_search_emulation: { anthropic: true },
      codex_image_generation_bridge: { openai: true },
      bedrock_cc_compat: { anthropic: true },
    })
    expect(result.features_config).not.toBe(existingFeatures)
  })

  it('removes stale managed feature keys when their platform is disabled', () => {
    const result = buildChannelAPIFields(
      [makeSection('anthropic', { enabled: false })],
      {
        untouched: true,
        web_search_emulation: { anthropic: true },
        codex_image_generation_bridge: { openai: true },
        bedrock_cc_compat: { anthropic: true },
      },
    )

    expect(result.features_config).toStrictEqual({ untouched: true })
  })

  it('serializes account rules in section and rule order while filtering empty pricing', () => {
    const rules = buildAccountStatsPricingRules([
      makeSection('anthropic', {
        account_stats_pricing_rules: [{
          name: 'first',
          group_ids: [2, 1],
          account_ids: [20, 10],
          pricing: [
            makePricing({ models: [] }),
            makePricing({ models: ['claude-*'], input_price: 3 }),
          ],
        }],
      }),
      makeSection('openai', {
        account_stats_pricing_rules: [{
          name: 'second',
          group_ids: [3],
          account_ids: [30],
          pricing: [makePricing({ models: ['gpt-*'], output_price: 9 })],
        }],
      }),
      makeSection('grok', {
        enabled: false,
        account_stats_pricing_rules: [{
          name: 'ignored',
          group_ids: [4],
          account_ids: [40],
          pricing: [makePricing()],
        }],
      }),
    ])

    expect(rules.map(rule => rule.name)).toStrictEqual(['first', 'second'])
    expect(Object.keys(rules[0])).toStrictEqual(['name', 'group_ids', 'account_ids', 'pricing'])
    expect(rules[0]).toMatchObject({
      group_ids: [2, 1],
      account_ids: [20, 10],
      pricing: [{ platform: 'anthropic', models: ['claude-*'], input_price: 0.000003 }],
    })
    expect(rules[1].pricing[0]).toMatchObject({
      platform: 'openai',
      models: ['gpt-*'],
      output_price: 0.000009,
    })
  })

  it('hydrates platform sections in fixed order without changing legacy feature reads', () => {
    const groups = [
      makeGroup(1, 'composite'),
      makeGroup(2, 'openai'),
      makeGroup(3, 'anthropic'),
    ]
    const channel = makeChannel({
      group_ids: [2, 1, 3],
      model_pricing: [
        makeAPIpricing('openai', { models: ['gpt-a'], input_price: 0.000002 }),
        makeAPIpricing('', { models: ['legacy-claude'], output_price: 0.000004 }),
      ],
      model_mapping: { openai: { 'gpt-*': 'gpt-a' } },
      features_config: {
        web_search_emulation: { anthropic: true },
        codex_image_generation_bridge: { openai: true },
        bedrock_cc_compat: true,
      },
    })

    const sections = channelToPlatformSections(channel, groups)

    expect(sections.map(section => section.platform)).toStrictEqual([
      'anthropic',
      'openai',
      'gemini',
      'antigravity',
        'grok',
        'kimi',
        'zhipu',
        'deepseek',
        'minimax',
    ])
    expect(sections.map(section => section.group_ids)).toStrictEqual([
      [1, 3],
      [2, 1],
        [1],
        [1],
        [1],
        [1],
        [1],
      [1],
      [1],
    ])
    expect(sections[0].model_pricing[0]).toMatchObject({
      models: ['legacy-claude'],
      output_price: 4,
    })
    expect(sections[1]).toMatchObject({
      model_mapping: { 'gpt-*': 'gpt-a' },
      codex_image_generation_bridge: true,
    })
    expect(sections[1].model_pricing[0].models).toBe(channel.model_pricing[0].models)
    expect(sections[1].model_mapping).not.toBe(channel.model_mapping.openai)
    expect(sections[0].web_search_emulation).toBe(true)
    expect(sections.every(section => section.bedrock_cc_compat)).toBe(true)

    const recordValuedBedrock = channelToPlatformSections(
      makeChannel({
        group_ids: [3],
        features_config: { bedrock_cc_compat: { anthropic: true } },
      }),
      groups,
    )
    expect(recordValuedBedrock[0].bedrock_cc_compat).toBe(false)
  })

  it('distributes rules by group order, then pricing platform, and clones editable arrays', () => {
    const sections = [makeSection('anthropic'), makeSection('openai')]
    const groups = [makeGroup(1, 'anthropic'), makeGroup(2, 'openai'), makeGroup(3, 'composite')]
    const apiRules: AccountStatsPricingRule[] = [
      {
        name: 'group-wins',
        group_ids: [1, 2],
        account_ids: [11],
        pricing: [makeAPIpricing('openai', { models: ['shared'], input_price: 0.000005 })],
      },
      {
        name: 'pricing-fallback',
        group_ids: [3],
        account_ids: [22],
        pricing: [makeAPIpricing('openai', { models: ['fallback'] })],
      },
      {
        name: 'unresolved',
        group_ids: [999],
        account_ids: [],
        pricing: [],
      },
    ]

    distributeAccountStatsPricingRules(sections, apiRules, groups)

    expect(sections[0].account_stats_pricing_rules.map(rule => rule.name)).toStrictEqual([
      'group-wins',
    ])
    expect(sections[1].account_stats_pricing_rules.map(rule => rule.name)).toStrictEqual([
      'pricing-fallback',
    ])
    expect(sections[0].account_stats_pricing_rules[0]).toMatchObject({
      group_ids: [1, 2],
      account_ids: [11],
      pricing: [{ models: ['shared'], input_price: 5 }],
    })

    sections[0].account_stats_pricing_rules[0].group_ids.push(9)
    sections[0].account_stats_pricing_rules[0].account_ids.push(99)
    sections[0].account_stats_pricing_rules[0].pricing[0].models.push('edited')
    expect(apiRules[0].group_ids).toStrictEqual([1, 2])
    expect(apiRules[0].account_ids).toStrictEqual([11])
    expect(apiRules[0].pricing[0].models).toStrictEqual(['shared'])
  })
})
