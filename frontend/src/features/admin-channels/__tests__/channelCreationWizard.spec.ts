import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import enCommon from '@/core/i18n/locales/en/common'
import enChannels from '@/core/i18n/locales/en/admin/channels'
import enOverview from '@/core/i18n/locales/en/admin/overview'
import ChannelsPage from '../presentation/pages/ChannelsPage.vue'

const { create, groups, syncModels, showError } = vi.hoisted(() => ({
  create: vi.fn(), groups: vi.fn(), syncModels: vi.fn(), showError: vi.fn(),
}))
vi.mock('@/api/admin', () => ({
  adminAPI: {
    channels: { list: async () => ({ items: [], total: 0 }), create, syncPricingModels: syncModels },
    settings: { getWebSearchEmulationConfig: async () => ({ enabled: true, providers: [{}] }) },
  },
}))
vi.mock('@/features/admin-groups/data/datasources/adminGroupQueries', () => ({ getAll: groups }))
vi.mock('@/core/stores/appStore', () => ({ useAppStore: () => ({ showError, showSuccess: vi.fn() }) }))

enableAutoUnmount(afterEach)
beforeEach(() => {
  vi.resetAllMocks()
  groups.mockResolvedValue([
    { id: 1, name: 'GPT group', platform: 'openai' },
    { id: 2, name: 'GLM group', platform: 'zhipu' },
  ])
})

function openPage() {
  const wrapper = mount(ChannelsPage, { global: {
    plugins: [createI18n({ legacy: false, locale: 'en', messages: {
      en: { ...enCommon, admin: { ...enChannels, ...enOverview } },
    } })],
    stubs: {
      AppLayout: { template: '<main><slot /></main>' },
      TablePageLayout: { template: '<div><slot name="filters" /></div>' },
      BaseDialog: { props: ['show'], template: '<div v-if="show"><slot name="after-title" /><slot /><slot name="footer" /></div>' },
      ConfirmDialog: true, Select: true,
    },
  } })
  function button(text: string) {
    const match = wrapper.findAll('button').find(button => button.isVisible() && button.text().includes(text))
    if (!match) throw new Error(`Missing button: ${text}`)
    return match
  }
  return { wrapper, button }
}

it('saves only the chosen provider even if a replaced provider finishes a model sync late', async () => {
  let finishSync!: (result: { models: string[] }) => void
  syncModels.mockReturnValue(new Promise(resolve => { finishSync = resolve }))
  const { wrapper, button } = openPage()
  await button('Create Channel').trigger('click')
  expect(wrapper.find('button[type="submit"]').exists()).toBe(false)
  await button('OpenAI').trigger('click')
  await wrapper.get('#channel-name').setValue('Custom channel')
  await wrapper.get('fieldset input[value="1"]').setValue(true)
  await wrapper.get('[aria-label="Codex Image Generation Bridge"]').trigger('click')
  await button('Models & mapping').trigger('click')
  await button(enChannels.channels.form.syncLatestModels).trigger('click')
  expect(syncModels).toHaveBeenCalledWith('openai')
  await button('Change provider').trigger('click')
  await button('Zhipu GLM').trigger('click')
  finishSync({ models: ['gpt-test'] })
  await flushPromises()

  expect(wrapper.get<HTMLInputElement>('#channel-name').element.value).toBe('Custom channel')
  expect(wrapper.find('fieldset input[value="1"]').exists()).toBe(false)
  expect(wrapper.find('[aria-label="Codex Image Generation Bridge"]').exists()).toBe(false)
  await wrapper.get('form').trigger('submit')
  expect(showError).toHaveBeenCalled()
  expect(create).not.toHaveBeenCalled()
  await wrapper.get('fieldset input[value="2"]').setValue(true)
  await wrapper.get('form').trigger('submit')
  expect(create).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({
    name: 'Custom channel', group_ids: [2], model_pricing: [], features_config: { bedrock_cc_compat: false, expose_response_model: false },
  }))
})

it('preserves the draft while retrying a failed group lookup', async () => {
  groups.mockRejectedValueOnce(new Error('Group list unavailable'))
  const { wrapper, button } = openPage()
  await button('Create Channel').trigger('click')
  await button('OpenAI').trigger('click')
  await flushPromises()
  expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
  await wrapper.get('#channel-name').setValue('Retry this draft')
  await button('Retry').trigger('click')
  await flushPromises()
  expect(wrapper.get<HTMLInputElement>('#channel-name').element.value).toBe('Retry this draft')
  expect(wrapper.get('fieldset input[value="1"]').exists()).toBe(true)
  expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeUndefined()
})
