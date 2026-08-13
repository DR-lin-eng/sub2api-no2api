import { describe, expect, it } from 'vitest'
import {
  getCodexFingerprintModeOptions,
  normalizeCodexFingerprintMode,
} from '../presentation/accountFormPolicy'

describe('Codex fingerprint form policy', () => {
  it.each([
    [undefined, 'off'],
    [null, 'off'],
    ['', 'off'],
    ['unexpected', 'off'],
    [' OFF ', 'off'],
    ['DEVICE', 'device'],
    [' session ', 'session'],
    ['full', 'full'],
  ])('normalizes %j to %s', (input, expected) => {
    expect(normalizeCodexFingerprintMode(input)).toBe(expected)
  })

  it('keeps off first and exposes every supported mode', () => {
    expect(getCodexFingerprintModeOptions((key) => key).map((option) => option.value)).toEqual([
      'off',
      'device',
      'session',
      'full',
    ])
  })
})
