import { ACCOUNT_FILTER_STORAGE_KEY, loadInitialAccountFilterState, saveAccountFilterState } from '../presentation/accountFilterState'

describe('account filter preferences', () => {
  it('round trips only platform and status while leaving transient filters out', () => {
    const data = new Map<string, string>()
    const storage = { getItem: (key: string) => data.get(key) ?? null, setItem: (key: string, value: string) => { data.set(key, value) } }
    saveAccountFilterState({ platform: 'gemini', status: 'rate_limited', search: 'private' } as { platform: string; status: string }, storage)
    expect(JSON.parse(data.get(ACCOUNT_FILTER_STORAGE_KEY)!)).toEqual({ platform: 'gemini', status: 'rate_limited' })
    expect(loadInitialAccountFilterState(storage)).toEqual({ platform: 'gemini', status: 'rate_limited' })
  })
  it.each(['{', 'null', '[]', '42', '{"platform":"unsupported","status":"future"}'])('rejects invalid preferences %s', raw => {
    expect(loadInitialAccountFilterState({ getItem: () => raw })).toEqual({ platform: '', status: '' })
  })
  it('keeps filtering usable when storage throws', () => {
    expect(loadInitialAccountFilterState({ getItem: () => { throw new Error('blocked') } })).toEqual({ platform: '', status: '' })
    expect(() => saveAccountFilterState({ platform: 'gemini', status: 'active' }, { setItem: () => { throw new Error('blocked') } })).not.toThrow()
  })
})
