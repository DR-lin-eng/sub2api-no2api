import { describe, expect, it } from 'vitest'
import { buildOpsErrorTimeParams } from '../presentation/opsErrorParams'

describe('buildOpsErrorTimeParams', () => {
  it('uses explicit timestamps for a complete custom range', () => {
    expect(buildOpsErrorTimeParams('custom', '2026-08-01T00:00:00Z', '2026-08-02T00:00:00Z')).toEqual({
      start_time: '2026-08-01T00:00:00Z',
      end_time: '2026-08-02T00:00:00Z'
    })
  })

  it('preserves presets and falls back for an incomplete custom range', () => {
    expect(buildOpsErrorTimeParams('24h')).toEqual({ time_range: '24h' })
    expect(buildOpsErrorTimeParams('custom', null, null)).toEqual({ time_range: '1h' })
  })
})
