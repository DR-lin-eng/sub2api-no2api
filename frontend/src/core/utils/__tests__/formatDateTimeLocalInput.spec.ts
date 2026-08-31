import { describe, expect, it } from 'vitest'

import {
  formatDateTimeLocalInput,
  getBrowserTimeZone,
  parseDateTimeLocalInput
} from '@/core/utils/format'

describe('formatDateTimeLocalInput', () => {
  it('round-trips a minute-precision local timestamp', () => {
    const timestamp = Math.floor(new Date(2026, 7, 30, 7, 24).getTime() / 1000)

    expect(formatDateTimeLocalInput(timestamp)).toBe('2026-08-30T07:24')
    expect(parseDateTimeLocalInput('2026-08-30T07:24')).toBe(timestamp)
  })
})

describe('parseDateTimeLocalInput', () => {
  it('uses local components and rejects timezone-bearing values', () => {
    const expected = Math.floor(new Date(2026, 7, 30, 7, 24, 10).getTime() / 1000)

    expect(parseDateTimeLocalInput('2026-08-30T07:24:10')).toBe(expected)
    expect(parseDateTimeLocalInput('2026-08-30T07:24:10+08:00')).toBeNull()
    expect(parseDateTimeLocalInput('2026-08-30T07:24:10Z')).toBeNull()
  })

  it('rejects malformed and overflowing calendar values', () => {
    expect(parseDateTimeLocalInput('')).toBeNull()
    expect(parseDateTimeLocalInput('2026-08-30 07:24')).toBeNull()
    expect(parseDateTimeLocalInput('2026-02-30T07:24')).toBeNull()
    expect(parseDateTimeLocalInput('2026-08-30T24:00')).toBeNull()
    expect(parseDateTimeLocalInput('2026-08-30T07:60')).toBeNull()
  })
})

describe('getBrowserTimeZone', () => {
  it('returns an identifier or the UTC fallback', () => {
    expect(getBrowserTimeZone()).toBeTruthy()
  })
})
