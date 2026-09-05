import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const copyToClipboard = vi.fn().mockResolvedValue(true)

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => (key === 'common.copy' ? '复制' : key)
    })
  }
})

vi.mock('@/core/stores/appStore', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn()
  })
}))

vi.mock('@/common/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard
  })
}))

import ModelWhitelistSelector from '../presentation/widgets/ModelWhitelistSelector.vue'

const expectedOpenAIModels = [
  'codex-auto-review',
  'gpt-5.4-mini',
  'gpt-5.5',
  'gpt-5.6-luna',
  'gpt-5.6-sol',
  'gpt-5.6-terra',
  'gpt-6-astra',
  'gpt-reserve',
  'gpt-image-1',
  'gpt-image-1.5',
  'gpt-image-2'
]

function mountSelector(props: { modelValue?: string[]; platform?: string; platforms?: string[] } = {}) {
  return mount(ModelWhitelistSelector, {
    props: {
      modelValue: [],
      platform: 'openai',
      ...props
    },
    global: {
      stubs: {
        ModelIcon: true
      }
    }
  })
}

function findModelRow(wrapper: ReturnType<typeof mountSelector>, modelId: string) {
  const row = wrapper
    .findAll('[data-testid="model-option"]')
    .find(candidate => candidate.get('[data-testid="select-model"]').text() === modelId)

  if (!row) {
    throw new Error(`Model row not found: ${modelId}`)
  }

  return row
}

describe('ModelWhitelistSelector', () => {
  beforeEach(() => {
    copyToClipboard.mockClear()
  })

  it.each([
    { label: 'single-account', props: { platform: 'openai' } },
    { label: 'bulk-edit', props: { platform: undefined, platforms: ['openai'] } }
  ])('keeps legacy models available alongside current defaults for $label', async ({ props }) => {
    const wrapper = mountSelector(props)
    await wrapper.get('div.cursor-pointer').trigger('click')

    const candidates = wrapper.findAll('[data-testid="select-model"]').map(row => row.text())
    expect(candidates).toHaveLength(22)
    expect(candidates).toEqual(expect.arrayContaining(expectedOpenAIModels))
    expect(candidates).toEqual(expect.arrayContaining([
      'gpt-5.2',
      'gpt-5.2-2025-12-11',
      'gpt-5.2-chat-latest',
      'gpt-5.2-pro',
      'gpt-5.2-pro-2025-12-11',
      'gpt-5.6',
      'gpt-5.4',
      'gpt-5.4-2026-03-05',
      'gpt-5.3-codex-spark',
      'gpt-4o-audio-preview',
      'gpt-4o-realtime-preview'
    ]))
  })

  it.each([
    { label: 'single-account', props: { platform: 'openai' } },
    { label: 'bulk-edit', props: { platform: undefined, platforms: ['openai'] } }
  ])('syncs only current defaults and image models for $label', async ({ props }) => {
    const wrapper = mountSelector(props)
    const fillButton = wrapper.findAll('button').find(button => button.text() === 'admin.accounts.fillRelatedModels')!

    await fillButton.trigger('click')

    expect(wrapper.emitted('update:modelValue')).toEqual([[expectedOpenAIModels]])
  })

  it('preserves saved legacy and custom models while filling defaults without duplicates', async () => {
    const existingModels = ['custom-openai-model', 'gpt-5.4', 'gpt-reserve']
    const wrapper = mountSelector({ modelValue: existingModels })
    const fillButton = wrapper.findAll('button').find(button => button.text() === 'admin.accounts.fillRelatedModels')!

    await fillButton.trigger('click')

    const expectedModels = [...existingModels, ...expectedOpenAIModels.filter(model => !existingModels.includes(model))]
    expect(wrapper.emitted('update:modelValue')).toEqual([[expectedModels]])
    expect(existingModels).toEqual(['custom-openai-model', 'gpt-5.4', 'gpt-reserve'])

    await wrapper.setProps({ modelValue: expectedModels })
    await fillButton.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[1]).toEqual([expectedModels])
  })

  it('allows manually selecting a legacy model that is not a default', async () => {
    const wrapper = mountSelector()
    await wrapper.get('div.cursor-pointer').trigger('click')

    await findModelRow(wrapper, 'gpt-5.4').get('[data-testid="select-model"]').trigger('click')

    expect(wrapper.emitted('update:modelValue')).toEqual([[['gpt-5.4']]])
    expect(expectedOpenAIModels).not.toContain('gpt-5.4')
  })

  it('copies a model ID without selecting the model', async () => {
    const wrapper = mountSelector()
    await wrapper.get('div.cursor-pointer').trigger('click')

    const row = findModelRow(wrapper, 'gpt-5.6-sol')

    const copyButton = row.get('[data-testid="copy-model-id"]')
    expect(copyButton.attributes('aria-label')).toBe('复制 gpt-5.6-sol')

    await copyButton.trigger('click')
    await flushPromises()

    expect(copyToClipboard).toHaveBeenCalledWith('gpt-5.6-sol')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('keeps the existing model selection behavior', async () => {
    const wrapper = mountSelector()
    await wrapper.get('div.cursor-pointer').trigger('click')

    const row = findModelRow(wrapper, 'gpt-5.6-sol')
    await row.get('[data-testid="select-model"]').trigger('click')

    expect(wrapper.emitted('update:modelValue')).toEqual([[['gpt-5.6-sol']]])
    expect(copyToClipboard).not.toHaveBeenCalled()
  })
})
