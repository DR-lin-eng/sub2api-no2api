import { describe, expect, it } from 'vitest'
import type { UsageLog } from '@/types'
import { buildUsageBillingCalculation } from '../usageBillingCalculation'

const usage = (overrides: Partial<UsageLog>): UsageLog => ({
  id: 1,
  user_id: 1,
  api_key_id: 1,
  account_id: 1,
  request_id: 'req-1',
  model: 'gpt-5.4',
  group_id: 1,
  subscription_id: null,
  input_tokens: 0,
  output_tokens: 0,
  cache_creation_tokens: 0,
  cache_read_tokens: 0,
  cache_creation_5m_tokens: 0,
  cache_creation_1h_tokens: 0,
  input_cost: 0,
  output_cost: 0,
  cache_creation_cost: 0,
  cache_read_cost: 0,
  total_cost: 0,
  actual_cost: 0,
  rate_multiplier: 1,
  long_context_billing_applied: false,
  billing_type: 0,
  stream: true,
  duration_ms: 1000,
  first_token_ms: 100,
  image_count: 0,
  image_size: null,
  image_input_size: null,
  image_output_size: null,
  image_size_source: null,
  image_size_breakdown: null,
  image_input_tokens: 0,
  image_input_cost: 0,
  image_output_tokens: 0,
  image_output_cost: 0,
  video_count: 0,
  video_resolution: null,
  video_duration_seconds: null,
  user_agent: null,
  cache_ttl_overridden: false,
  billing_mode: 'token',
  created_at: '2026-07-20T00:00:00Z',
  ...overrides,
})

describe('buildUsageBillingCalculation', () => {
  it('reconciles token component costs and a direct group multiplier', () => {
    const result = buildUsageBillingCalculation(usage({
      input_tokens: 1_000,
      output_tokens: 200,
      input_cost: 0.005,
      output_cost: 0.006,
      total_cost: 0.011,
      actual_cost: 0.0088,
      rate_multiplier: 0.8,
    }))

    expect(result.formulaKind).toBe('direct')
    expect(result.componentSubtotal).toBeCloseTo(0.011)
    expect(result.calculatedActual).toBeCloseTo(0.0088)
    expect(result.reconciled).toBe(true)
  })

  it('separates an independent image multiplier and marks the text rate as reconstructed', () => {
    const result = buildUsageBillingCalculation(usage({
      input_tokens: 1_100,
      image_input_tokens: 100,
      input_cost: 0.1,
      image_input_cost: 0.2,
      total_cost: 0.3,
      actual_cost: 0.45,
      rate_multiplier: 2,
    }))

    expect(result.formulaKind).toBe('split')
    expect(result.textRateMultiplier).toBeCloseTo(0.5)
    expect(result.imageRateMultiplier).toBe(2)
    expect(result.calculatedActual).toBeCloseTo(0.45)
    expect(result.reconciled).toBe(true)
  })

  it('calculates video pricing by generated video-seconds', () => {
    const result = buildUsageBillingCalculation(usage({
      billing_mode: 'video',
      video_count: 2,
      video_duration_seconds: 10,
      video_resolution: '720p',
      total_cost: 1.4,
      actual_cost: 2.1,
      rate_multiplier: 1.5,
    }))

    expect(result.lines[0]).toMatchObject({ quantity: 20, quantityUnit: 'video_seconds' })
    expect(result.lines[0].unitPrice).toBeCloseTo(0.07)
    expect(result.calculatedActual).toBeCloseTo(2.1)
    expect(result.reconciled).toBe(true)
  })

  it('calculates image pricing by generated image count', () => {
    const result = buildUsageBillingCalculation(usage({
      billing_mode: 'image',
      image_count: 2,
      image_size: '2K',
      total_cost: 0.4,
      actual_cost: 0.6,
      rate_multiplier: 1.5,
    }))

    expect(result.lines[0]).toMatchObject({ quantity: 2, quantityUnit: 'images' })
    expect(result.lines[0].unitPrice).toBeCloseTo(0.2)
    expect(result.calculatedActual).toBeCloseTo(0.6)
    expect(result.reconciled).toBe(true)
  })

  it('uses the recorded blended rate when an old row cannot be reconstructed from one snapshot rate', () => {
    const result = buildUsageBillingCalculation(usage({
      billing_mode: 'per_request',
      total_cost: 1,
      actual_cost: 0.75,
      rate_multiplier: 1,
    }))

    expect(result.formulaKind).toBe('effective')
    expect(result.effectiveRateMultiplier).toBeCloseTo(0.75)
    expect(result.reconciled).toBe(true)
  })
})
