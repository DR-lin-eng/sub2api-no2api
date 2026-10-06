import { describe, expect, it } from 'vitest'
import { defaultAPIKeyBaseURL } from '@/core/constants/account'
import { isHeaderOverrideCapable } from '../presentation/credentialsBuilder'
import { areUpstreamBillingProbeTargetsEligible, isUpstreamBillingProbeEligible } from '../presentation/upstreamBillingProbeEligibility'

describe('TypeSafe account capabilities', () => {
  it('uses its official endpoint and API-key-only header/probe controls', () => {
    expect(defaultAPIKeyBaseURL('typesafe')).toBe('https://api.typesafe.ai')
    expect(isHeaderOverrideCapable('typesafe', 'apikey')).toBe(true)
    expect(isHeaderOverrideCapable('typesafe', 'oauth')).toBe(false)
    expect(isUpstreamBillingProbeEligible('typesafe', 'apikey')).toBe(true)
    expect(isUpstreamBillingProbeEligible('typesafe', 'oauth')).toBe(false)
    expect(areUpstreamBillingProbeTargetsEligible(['typesafe', 'openai'], ['apikey'])).toBe(true)
    expect(areUpstreamBillingProbeTargetsEligible(['typesafe'], ['apikey', 'oauth'])).toBe(false)
  })
})
