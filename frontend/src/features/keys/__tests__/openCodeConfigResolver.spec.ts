import { createHash } from 'node:crypto'
import { describe, expect, it } from 'vitest'
import {
  generateOpenCodeConfig,
  type OpenCodeProviderId
} from '../presentation/resolvers/openCodeConfigResolver'

interface OpenCodeBaseline {
  platform: OpenCodeProviderId
  bytes: number
  sha256: string
  providerKeys: string[]
}

const baselines: OpenCodeBaseline[] = [
  {
    platform: 'anthropic',
    bytes: 2047,
    sha256: 'beb277783144fc4fa8a1c4e2d1574fa601ba4e37452a8752adb6050be2cc45ac',
    providerKeys: ['options', 'npm', 'models']
  },
  {
    platform: 'openai',
    bytes: 6888,
    sha256: 'ac14ec38120fbf2b3d29f9d465f3e4a83fecb975fae3e3e65d0e494e79e72eae',
    providerKeys: ['options', 'models']
  },
  {
    platform: 'gemini',
    bytes: 3331,
    sha256: '58e586cdcc97f9506ce57b7aee6623134a2cb09a1643a2a143335cb371d6df56',
    providerKeys: ['options', 'npm', 'models']
  },
  {
    platform: 'antigravity-claude',
    bytes: 2327,
    sha256: '43d6e7b7809d3911bacff829c96f6de43b356a47db4b9799baf98e6eb1cc8aa1',
    providerKeys: ['options', 'npm', 'name', 'models']
  },
  {
    platform: 'antigravity-gemini',
    bytes: 4471,
    sha256: '47b486d2995c7c41b0598aa776a2d7f1258d6a3257dfc33910799f624a035640',
    providerKeys: ['options', 'npm', 'name', 'models']
  },
  {
    platform: 'grok',
    bytes: 943,
    sha256: 'bc7dd784b03d1792243f39cde154c4b8a885e17f9b1cd100d5966a220a91e95e',
    providerKeys: ['options', 'npm', 'name', 'models']
  }
]

describe('OpenCode config resolver', () => {
  it.each(baselines)('preserves the $platform serialized template byte-for-byte', ({
    platform,
    bytes,
    sha256,
    providerKeys
  }) => {
    const result = generateOpenCodeConfig({
      platform,
      baseUrl: 'https://example.com/base',
      apiKey: 'sk-byte-test',
      pathLabel: `${platform}.json`,
      hint: 'keys.useKeyModal.opencode.hint'
    })
    const parsed = JSON.parse(result.content)

    expect(result.path).toBe(`${platform}.json`)
    expect(result.hint).toBe('keys.useKeyModal.opencode.hint')
    expect(Buffer.byteLength(result.content)).toBe(bytes)
    expect(createHash('sha256').update(result.content).digest('hex')).toBe(sha256)
    expect(Object.keys(parsed)).toEqual(
      platform === 'openai' ? ['provider', 'agent', '$schema'] : ['provider', '$schema']
    )
    expect(Object.keys(parsed.provider)).toEqual([platform])
    expect(Object.keys(parsed.provider[platform])).toEqual(providerKeys)
  })

  it('keeps the default path and caller-provided translated hint', () => {
    const result = generateOpenCodeConfig({
      platform: 'anthropic',
      baseUrl: 'https://example.com/v1',
      apiKey: 'sk-test',
      hint: 'translated hint'
    })

    expect(result.path).toBe('opencode.json')
    expect(result.hint).toBe('translated hint')
  })

  it('exposes Claude 5.2 adaptive effort variants only on its supported native provider', () => {
    const config = (platform: OpenCodeProviderId) => JSON.parse(generateOpenCodeConfig({
      platform, baseUrl: 'https://example.com/v1', apiKey: 'sk-test', hint: 'hint'
    }).content).provider[platform]

    const native = config('anthropic')
    const antigravity = config('antigravity-claude')
    for (const model of ['claude-fable-5-2', 'claude-mythos-5-2']) {
      expect(native.models[model].limit).toEqual({ context: 1000000, output: 128000 })
      expect(native.models[model].options).toEqual({ thinking: { type: 'adaptive' }, effort: 'high' })
      expect(native.models[model].variants.xhigh).toEqual({ effort: 'xhigh' })
      expect(antigravity.models[model]).toBeUndefined()
    }
  })
})
