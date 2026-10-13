import { describe, expect, it } from 'vitest'
import { buildTempUnschedRules, normalizeTempUnschedScope, parseTempUnschedRules } from '../presentation/accountFormPolicy'

describe('temporary pause scope compatibility', () => {
  const rule = { error_code: 503, keywords: 'busy', duration_minutes: 10, description: '' }
  it('omits the field on legacy and automatic rules', () => {
    expect(buildTempUnschedRules([rule])[0]).not.toHaveProperty('scope')
    expect(buildTempUnschedRules([{ ...rule, scope: '' }])[0]).not.toHaveProperty('scope')
  })
  it('preserves each explicit scope when saving credentials', () => {
    for (const scope of ['account', 'model'] as const) {
      expect(buildTempUnschedRules([{ ...rule, scope }])[0].scope).toBe(scope)
    }
  })
  it('hydrates and saves existing rules without losing explicit scope or keyword order', () => {
    const stored = [{ ...rule, keywords: ['busy', 'retry'], scope: 'model' }]
    expect(buildTempUnschedRules(parseTempUnschedRules(stored))).toEqual(stored)
  })
  it('normalizes stored values and falls back on invalid or future values', () => {
    expect(normalizeTempUnschedScope(' MODEL ')).toBe('model')
    for (const value of [null, 7, {}, 'future']) expect(normalizeTempUnschedScope(value)).toBe('')
  })
})
