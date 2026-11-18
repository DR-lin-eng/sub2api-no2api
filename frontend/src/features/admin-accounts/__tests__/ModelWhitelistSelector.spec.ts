import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const syncMock = vi.hoisted(() => vi.fn())
vi.mock('../data/datasources/adminAccountActions', () => ({ syncUpstreamModels: syncMock, syncUpstreamModelsPreview: syncMock }))

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

const messages = vi.hoisted(() => ({ showError: vi.fn(), showSuccess: vi.fn(), showInfo: vi.fn() }))
vi.mock('@/core/stores/appStore', () => ({ useAppStore: () => messages }))

vi.mock('@/common/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard
  })
}))

import ModelWhitelistSelector from '../presentation/widgets/ModelWhitelistSelector.vue'
import { allModels } from '../presentation/composables/useModelWhitelist'

function mountSelector(overrides: Record<string, unknown> = {}) {
  return mount(ModelWhitelistSelector, {
    props: {
      modelValue: [],
      platform: 'openai',
      ...overrides
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
    .find(candidate => candidate.text().includes(modelId))

  if (!row) {
    throw new Error(`Model row not found: ${modelId}`)
  }

  return row
}

describe('ModelWhitelistSelector', () => {
  beforeEach(() => {
    copyToClipboard.mockClear()
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

  it.each(['openai', 'multimodal'])('lists both GPT Image 2.6 models for %s accounts', async (platform) => {
    const wrapper = mountSelector({ platform })
    await wrapper.get('div.cursor-pointer').trigger('click')
    expect(findModelRow(wrapper, 'gpt-image-2.6-flare').exists()).toBe(true)
    expect(findModelRow(wrapper, 'gpt-image-2.6-sunburst').exists()).toBe(true)
  })

  it('offers the 19 OpenAI defaults in channel order and selects gpt-reserve', async () => {
    const wrapper = mountSelector()
    await wrapper.get('div.cursor-pointer').trigger('click')

    const ids = wrapper.findAll('[data-testid="select-model"]').map(button => button.text())
    expect(ids).toHaveLength(19)
    expect(ids.slice(0, 4)).toEqual(['codex-auto-review', 'gpt-5.6-luna', 'gpt-5.6-sol', 'gpt-5.6-terra'])
    expect(ids.at(-1)).toBe('gpt-reserve')
    expect(ids).not.toContain('gpt-5.4')
    expect(ids).not.toContain('gpt-5.2')
    await findModelRow(wrapper, 'gpt-reserve').get('[data-testid="select-model"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[['gpt-reserve']]])
  })

  it('fills OpenAI defaults while preserving existing custom and legacy selections', async () => {
    const selected = ['custom-openai-model', 'gpt-5.4', 'gpt-6.2-sol']
    const wrapper = mountSelector({ modelValue: selected })
    const fill = wrapper.findAll('button').find(button => button.text() === 'admin.accounts.fillRelatedModels')!
    await fill.trigger('click')

    const models = wrapper.emitted('update:modelValue')![0][0] as string[]
    expect(models.slice(0, 3)).toEqual(selected)
    expect(models).toHaveLength(21)
    expect(models.filter(model => model === 'gpt-6.2-sol')).toHaveLength(1)
    expect(models.at(-1)).toBe('gpt-reserve')
    expect(models).not.toContain('gpt-5.2')
    expect(selected).toEqual(['custom-openai-model', 'gpt-5.4', 'gpt-6.2-sol'])
  })

  it('keeps the complete catalog when no platform is selected', async () => {
    const wrapper = mountSelector({ platform: undefined })
    await wrapper.get('div.cursor-pointer').trigger('click')

    const ids = wrapper.findAll('[data-testid="select-model"]').map(button => button.text())
    expect(ids).toEqual(allModels.map(model => model.value))
    expect(ids).toContain('gpt-5.4')
    expect(ids).toContain('gpt-5.2')
    expect(ids).not.toContain('gpt-reserve')

    await wrapper.setProps({ platforms: ['openai', 'grok'] })
    const platformIds = wrapper.findAll('[data-testid="select-model"]').map(button => button.text())
    expect(platformIds).toContain('gpt-reserve')
    expect(platformIds).toContain('grok-4.6')
    expect(platformIds).not.toContain('gpt-5.2')
  })
})


it('preserves a mapped target when adding the same source as a whitelist model', async () => {
  const wrapper = mountSelector({ modelMappings: [{ from: 'gpt-latest', to: 'deepseek-chat' }] })
  const input = wrapper.get('input[placeholder="admin.accounts.enterCustomModelName"]')
  await input.setValue(' gpt-latest ')
  await input.trigger('keydown.enter')
  expect(wrapper.emitted('update:modelValue')).toBeUndefined()
})
it('allows an identity mapping in the whitelist', async () => {
  const wrapper = mountSelector({ modelMappings: [{ from: 'gpt-latest', to: 'gpt-latest' }] })
  const input = wrapper.get('input[placeholder="admin.accounts.enterCustomModelName"]')
  await input.setValue('gpt-latest')
  await input.trigger('keydown.enter')
  expect(wrapper.emitted('update:modelValue')).toEqual([[['gpt-latest']]])
})

it('offers Grok catalog IDs without changing the selected whitelist', async () => {
  syncMock.mockResolvedValue({ models: ['grok-custom-new'] })
  const wrapper = mountSelector({ platform: 'grok', accountId: 1, modelValue: ['grok-existing-custom'] })
  const sync = wrapper.findAll('button').find(button => button.text() === 'admin.accounts.syncUpstreamModels')!
  await sync.trigger('click'); await flushPromises()
  expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  await wrapper.get('div.cursor-pointer').trigger('click')
  expect(wrapper.text()).toContain('grok-custom-new')
  expect(wrapper.text()).toContain('grok-existing-custom')
  await wrapper.setProps({ accountId: 2 })
  expect(wrapper.findAll('[data-testid="model-option"]').some(row => row.text().includes('grok-custom-new'))).toBe(false)
})

it('ignores an upstream result after the selector unmounts', async () => {
  vi.clearAllMocks()
  let resolve!: (value: { models: string[] }) => void
  syncMock.mockReturnValueOnce(new Promise(done => { resolve = done }))
  const wrapper = mountSelector({ platform: 'grok', accountId: 1 })
  const sync = wrapper.findAll('button').find(button => button.text() === 'admin.accounts.syncUpstreamModels')!
  await sync.trigger('click'); wrapper.unmount()
  resolve({ models: ['grok-4.6'] }); await flushPromises()
  expect(messages.showInfo).not.toHaveBeenCalled()
  expect(messages.showSuccess).not.toHaveBeenCalled()
  expect(wrapper.emitted('update:modelValue')).toBeUndefined()
})
