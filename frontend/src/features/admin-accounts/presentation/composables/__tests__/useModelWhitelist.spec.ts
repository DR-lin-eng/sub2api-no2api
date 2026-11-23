import { describe, expect, it, vi } from 'vitest'

vi.mock('@/features/admin-accounts/data/datasources/adminAccountQueries', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
}))

import { allModels, buildModelMappingObject, getModelsByPlatform, getPresetMappingsByPlatform, splitModelMappingObject } from '../useModelWhitelist'

describe('useModelWhitelist', () => {
  it('exposes Carbon with its declared model ID and identity preset', () => {
    expect(getModelsByPlatform('gemini')).toContain('gemini-4-carbon')
    expect(getModelsByPlatform('antigravity')).toContain('gemini-4-carbon')
    expect(getModelsByPlatform('gemini')).not.toContain('gemini-4-carbon-preview')
    expect(getPresetMappingsByPlatform('antigravity')).toEqual(expect.arrayContaining([
      expect.objectContaining({ from: 'gemini-4-carbon', to: 'gemini-4-carbon' })
    ]))
  })
  it('exposes Hydrogen in Gemini while preserving the declared Antigravity catalog', () => {
    expect(getModelsByPlatform('gemini')).toContain('gemini-4-hydrogen')
    expect(getModelsByPlatform('antigravity')).not.toContain('gemini-4-hydrogen')
  })

  it('exposes the latest Gemini 4 model in the Gemini selector', () => {
    expect(getModelsByPlatform('gemini')).toContain('gemini-4-neon')
    expect(getPresetMappingsByPlatform('gemini')).toEqual(expect.arrayContaining([
      expect.objectContaining({ from: 'gemini-4-neon', to: 'gemini-4-neon' })
    ]))
  })

  it('OpenAI 渠道默认模型与指定的 Codex 清单和顺序一致', () => {
    const models = getModelsByPlatform('openai')

    expect(models).toEqual([
      'codex-auto-review',
      'gpt-5.6-luna', 'gpt-5.6-sol', 'gpt-5.6-terra',
      'gpt-6-astra', 'gpt-6-luna', 'gpt-6-sol',
      'gpt-6.1-sol', 'gpt-6.1-terra',
      'gpt-6.2-astra', 'gpt-6.2-sol',
      'gpt-image-1', 'gpt-image-1.5', 'gpt-image-2',
      'gpt-image-2.5-flare', 'gpt-image-2.5-sunburst',
      'gpt-image-2.6-flare', 'gpt-image-2.6-sunburst',
      'gpt-reserve'
    ])
    for (const id of ['gpt-6.2-astra', 'gpt-6.2-sol']) {
      expect(getPresetMappingsByPlatform('openai')).toEqual(expect.arrayContaining([
        expect.objectContaining({ from: id, to: id })
      ]))
    }
  })

  it('渠道清单调整保留总目录中的旧模型，渠道专属项不写入总目录', () => {
    const globalIds = allModels.map(model => model.value)
    for (const id of [
      'gpt-5.2', 'gpt-5.2-2025-12-11', 'gpt-5.2-chat-latest',
      'gpt-5.2-pro', 'gpt-5.2-pro-2025-12-11',
      'gpt-5.6', 'gpt-5.5',
      'gpt-5.4', 'gpt-5.4-mini', 'gpt-5.4-2026-03-05',
      'gpt-5.3-codex-spark', 'gpt-4o-audio-preview', 'gpt-4o-realtime-preview'
    ]) {
      expect(globalIds).toContain(id)
      expect(getModelsByPlatform('openai')).not.toContain(id)
    }
    expect(globalIds).not.toContain('gpt-reserve')
  })

  it('openai 模型列表不再暴露已下线的 ChatGPT 登录 Codex 模型', () => {
    const models = getModelsByPlatform('openai')

    expect(models).not.toContain('gpt-5')
    expect(models).not.toContain('gpt-5.1')
    expect(models).not.toContain('gpt-5.1-codex')
    expect(models).not.toContain('gpt-5.1-codex-max')
    expect(models).not.toContain('gpt-5.1-codex-mini')
    expect(models).not.toContain('gpt-5.2-codex')
  })

  it('antigravity 模型列表包含图片模型兼容项', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models).toContain('gemini-2.5-flash-image')
    expect(models).toContain('gemini-3.1-flash-image')
    expect(models).toContain('gemini-3-pro-image')
  })

  it('Claude 5.5 models and provider-specific presets stay aligned', () => {
    for (const family of ['opus', 'sonnet', 'haiku']) {
      const id = `claude-${family}-5-5`
      expect(getModelsByPlatform('claude')).toContain(id)
      expect(getPresetMappingsByPlatform('claude')).toEqual(expect.arrayContaining([expect.objectContaining({ from: id, to: id })]))
    }
    for (const family of ['opus', 'sonnet']) {
      const id = `claude-${family}-5-5`
      for (const tier of ['low', 'medium', 'high']) expect(getModelsByPlatform('antigravity')).toContain(`${id}-${tier}`)
      expect(getPresetMappingsByPlatform('antigravity')).toEqual(expect.arrayContaining([expect.objectContaining({ from: id, to: `${id}-high` })]))
    }
  })

  it('Claude 模型列表包含新发布的 Claude 模型', () => {
    expect(getModelsByPlatform('claude')).toContain('claude-fable-5-1')
    expect(getModelsByPlatform('antigravity')).toContain('claude-fable-5-1')
    expect(getModelsByPlatform('claude')).toContain('claude-fable-5')
    expect(getModelsByPlatform('antigravity')).toContain('claude-fable-5')
    expect(getModelsByPlatform('claude')).toContain('claude-opus-4-8')
    expect(getModelsByPlatform('antigravity')).toContain('claude-opus-4-8')
  })

  it('xAI 模型列表包含 Grok 4.6、4.5 官方模型和别名', () => {
    const models = getModelsByPlatform('grok')

    expect(models).toContain('grok-4.6')
    expect(models).toContain('grok-4.6-latest')
    expect(models).toContain('grok-4.5')
    expect(models).toContain('grok-4.5-latest')
    expect(models).toContain('grok-build-latest')
    expect(models).toContain('grok-imagine-video-1.5-preview')
  })

  it('combined 模式支持 Grok 4.5 官方别名映射', () => {
    const mapping = buildModelMappingObject(
      'combined',
      ['grok-4.5'],
      [
        { from: 'grok-latest', to: 'grok-4.5' },
        { from: 'grok-4.5-latest', to: 'grok-4.5' },
        { from: 'grok-build-latest', to: 'grok-4.5' }
      ]
    )

    expect(mapping).toEqual({
      'grok-4.5': 'grok-4.5',
      'grok-latest': 'grok-4.5',
      'grok-4.5-latest': 'grok-4.5',
      'grok-build-latest': 'grok-4.5'
    })
  })

  it('grok 模型列表包含 Composer 默认项和兼容别名', () => {
    const models = getModelsByPlatform('grok')

    expect(models).toContain('grok-composer-2.5-fast')
    expect(models).toContain('grok-composer')
    expect(models).toContain('composer-2.5')
  })

  it('gemini 模型列表包含原生生图模型', () => {
    const models = getModelsByPlatform('gemini')

    expect(models).toContain('gemini-2.5-flash-image')
    expect(models).toContain('gemini-3.1-flash-image')
    expect(models.indexOf('gemini-3.1-flash-image')).toBeLessThan(models.indexOf('gemini-2.0-flash'))
    expect(models.indexOf('gemini-2.5-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash'))
  })

  it('antigravity 模型列表会把新的 Gemini 图片模型排在前面', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models.indexOf('gemini-3.1-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash'))
    expect(models.indexOf('gemini-2.5-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash-lite'))
  })

  it('antigravity 模型列表包含 Gemini 3.1 Pro 通用别名', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models).toContain('gemini-3.1-pro')
  })

  it('Gemini 4 Argon 在 gemini 与 antigravity 平台使用对应真实模型 ID 与预设映射', () => {
    expect(getModelsByPlatform('gemini')).toContain('gemini-4-argon')
    const agModels = getModelsByPlatform('antigravity')
    expect(agModels).not.toContain('gemini-4-argon')
    expect(agModels).toContain('gemini-4-argon-high')
    expect(agModels).toContain('gemini-4-argon-medium')
    expect(agModels).toContain('gemini-4-argon-low')
    expect(getPresetMappingsByPlatform('antigravity')).toEqual(expect.arrayContaining([
      expect.objectContaining({ from: 'gemini-4-argon', to: 'gemini-4-argon-high' }),
      expect.objectContaining({ from: 'gemini-4-argon-high', to: 'gemini-4-argon-high' }),
      expect.objectContaining({ from: 'gemini-4-argon-medium', to: 'gemini-4-argon-medium' }),
      expect.objectContaining({ from: 'gemini-4-argon-low', to: 'gemini-4-argon-low' })
    ]))
  })

  it('whitelist 模式会忽略通配符条目', () => {
    const mapping = buildModelMappingObject('whitelist', ['claude-*', 'gemini-3.1-flash-image'], [])
    expect(mapping).toEqual({
      'gemini-3.1-flash-image': 'gemini-3.1-flash-image'
    })
  })

  it('whitelist 模式会保留 GPT-5.4 官方快照的精确映射', () => {
    const mapping = buildModelMappingObject('whitelist', ['gpt-5.4-2026-03-05'], [])

    expect(mapping).toEqual({
      'gpt-5.4-2026-03-05': 'gpt-5.4-2026-03-05'
    })
  })

  it('whitelist keeps GPT-5.4 mini exact mappings', () => {
    const mapping = buildModelMappingObject('whitelist', ['gpt-5.4-mini'], [])

    expect(mapping).toEqual({
      'gpt-5.4-mini': 'gpt-5.4-mini'
    })
  })

  it('combined 模式会同时保留白名单身份映射和模型映射', () => {
    const mapping = buildModelMappingObject(
      'combined',
      ['gpt-5.4', 'claude-*'],
      [
        { from: 'gpt-latest', to: 'gpt-5.4' },
        { from: 'gpt-5.4', to: 'gpt-5.4-mini' }
      ]
    )

    expect(mapping).toEqual({
      'gpt-5.4': 'gpt-5.4-mini',
      'gpt-latest': 'gpt-5.4'
    })
  })

  it('splitModelMappingObject 会把身份映射还原成白名单，其余保留为映射', () => {
    const parsed = splitModelMappingObject({
      'gpt-5.4': 'gpt-5.4',
      'gpt-latest': 'gpt-5.4',
      ' ': 'gpt-empty',
      broken: 123
    })

    expect(parsed).toEqual({
      allowedModels: ['gpt-5.4'],
      modelMappings: [{ from: 'gpt-latest', to: 'gpt-5.4' }]
    })
  })
})
