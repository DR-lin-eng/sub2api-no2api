import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import MonitorFormDialog from '@/components/admin/monitor/MonitorFormDialog.vue'
import {
  DEFAULT_GROK_ENDPOINT,
  DEFAULT_GROK_MODEL,
  PROVIDERS,
  PROVIDER_GROK,
} from '@/constants/channelMonitor'

const { createMonitor, listChannels, listTemplates } = vi.hoisted(() => ({
  createMonitor: vi.fn(),
  listChannels: vi.fn(),
  listTemplates: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    channelMonitor: {
      create: createMonitor,
      update: vi.fn(),
    },
    channels: {
      list: listChannels,
    },
    channelMonitorTemplate: {
      list: listTemplates,
    },
  },
}))

vi.mock('@/api/keys', () => ({
  keysAPI: { list: vi.fn() },
}))

vi.mock('@/api/groups', () => ({
  userGroupsAPI: { getUserGroupRates: vi.fn() },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    cachedPublicSettings: null,
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const BaseDialogStub = defineComponent({
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})

const SelectStub = defineComponent({
  props: {
    modelValue: { type: [String, Number], default: '' },
    options: { type: Array, default: () => [] },
    placeholder: { type: String, default: '' },
  },
  emits: ['update:modelValue'],
  setup(_props, { emit }) {
    return {
      onChange(event: Event) {
        emit('update:modelValue', (event.target as HTMLSelectElement).value)
      },
    }
  },
  template: `
    <select
      :data-testid="'select-' + placeholder"
      :value="modelValue"
      @change="onChange"
    >
      <option value=""></option>
      <option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option>
    </select>
  `,
})

function mountDialog() {
  return mount(MonitorFormDialog, {
    props: { show: true, monitor: null },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Toggle: true,
        Select: SelectStub,
        ModelTagInput: true,
        MonitorKeyPickerDialog: true,
        MonitorAdvancedRequestConfig: true,
      },
    },
  })
}

describe('channel monitor Grok provider', () => {
  beforeEach(() => {
    createMonitor.mockReset().mockResolvedValue({})
    listChannels.mockReset().mockResolvedValue({ items: [{ id: 7, name: 'Primary channel' }] })
    listTemplates.mockReset().mockResolvedValue({ items: [] })
  })

  it('offers Grok in the responsive provider grid and prefills its official defaults', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    expect(PROVIDERS).toContain(PROVIDER_GROK)
    const providerButtons = wrapper.findAll('[data-testid^="monitor-provider-"]')
    expect(providerButtons).toHaveLength(4)
    expect(providerButtons[0].element.parentElement?.className).toContain('grid-cols-2')
    expect(providerButtons[0].element.parentElement?.className).toContain('sm:grid-cols-4')

    const grokButton = wrapper.get('[data-testid="monitor-provider-grok"]')
    expect(grokButton.find('svg').exists()).toBe(true)
    expect(grokButton.text()).toContain('monitorCommon.providers.grok')
    await grokButton.trigger('click')
    expect(grokButton.classes().join(' ')).toContain('zinc')

    const endpoint = wrapper.get('[data-testid="monitor-endpoint"]')
    const model = wrapper.get('[data-testid="monitor-primary-model"]')
    expect((endpoint.element as HTMLInputElement).value).toBe(DEFAULT_GROK_ENDPOINT)
    expect((model.element as HTMLInputElement).value).toBe(DEFAULT_GROK_MODEL)

    await wrapper.get('[data-testid="monitor-provider-anthropic"]').trigger('click')
    expect((endpoint.element as HTMLInputElement).value).toBe('')
    expect((model.element as HTMLInputElement).value).toBe('')

    await grokButton.trigger('click')
    await endpoint.setValue('https://gateway.example.com')
    await model.setValue('grok-custom')
    await wrapper.get('[data-testid="monitor-provider-openai"]').trigger('click')
    expect((endpoint.element as HTMLInputElement).value).toBe('https://gateway.example.com')
    expect((model.element as HTMLInputElement).value).toBe('grok-custom')
  })

  it('prefills only empty Grok fields and preserves existing provider values', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    const endpoint = wrapper.get('[data-testid="monitor-endpoint"]')
    const model = wrapper.get('[data-testid="monitor-primary-model"]')
    const grokButton = wrapper.get('[data-testid="monitor-provider-grok"]')
    const anthropicButton = wrapper.get('[data-testid="monitor-provider-anthropic"]')

    await endpoint.setValue('https://gateway.example.com')
    await grokButton.trigger('click')
    expect((endpoint.element as HTMLInputElement).value).toBe('https://gateway.example.com')
    expect((model.element as HTMLInputElement).value).toBe(DEFAULT_GROK_MODEL)

    await anthropicButton.trigger('click')
    expect((endpoint.element as HTMLInputElement).value).toBe('https://gateway.example.com')
    expect((model.element as HTMLInputElement).value).toBe('')

    await endpoint.setValue('')
    await model.setValue('grok-custom')
    await grokButton.trigger('click')
    expect((endpoint.element as HTMLInputElement).value).toBe(DEFAULT_GROK_ENDPOINT)
    expect((model.element as HTMLInputElement).value).toBe('grok-custom')
  })

  it('submits passive monitors without probe credentials', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    await wrapper.get('[data-testid="monitor-mode-passive"]').trigger('click')
    await flushPromises()

    expect(listChannels).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-testid="monitor-endpoint"]').exists()).toBe(false)
    expect(wrapper.find('input[type="password"]').exists()).toBe(false)

    await wrapper.get('[placeholder="admin.channelMonitor.form.namePlaceholder"]').setValue('Passive request monitor')
    await wrapper.get('[data-testid="monitor-primary-model"]').setValue('gpt-5.4')
    await wrapper.get('[data-testid="select-admin.channelMonitor.form.channelPlaceholder"]').setValue('7')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(createMonitor).toHaveBeenCalledWith(expect.objectContaining({
      name: 'Passive request monitor',
      monitor_mode: 'passive',
      channel_id: 7,
      primary_model: 'gpt-5.4',
      jitter_seconds: 0,
    }))
    const payload = createMonitor.mock.calls[0][0]
    expect(payload).not.toHaveProperty('endpoint')
    expect(payload).not.toHaveProperty('api_key')
  })
})
