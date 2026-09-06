import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

const { previewFromCpa, syncFromCpa, showError, showSuccess } = vi.hoisted(() => ({
  previewFromCpa: vi.fn(),
  syncFromCpa: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/features/admin-accounts/data/datasources/adminAccountQueries', () => ({
  previewFromCpa
}))

vi.mock('@/features/admin-accounts/data/datasources/adminAccountActions', () => ({
  syncFromCpa
}))

vi.mock('@/core/stores/appStore', () => ({
  useAppStore: () => ({ showError, showSuccess })
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key, te: () => false })
}))

import SyncFromCpaDialog from '@/features/admin-accounts/presentation/widgets/SyncFromCpaDialog.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const GroupSelectorStub = defineComponent({
  name: 'GroupSelector',
  props: { modelValue: { type: Array, required: true }, groups: { type: Array, required: true }, platform: { type: String, required: false } },
  emits: ['update:modelValue'],
  template: '<div data-testid="group-selector"><button type="button" data-testid="choose-group" @click="$emit(\'update:modelValue\', [10])">choose</button></div>'
})

const groups = [
  { id: 10, name: 'OpenAI group', platform: 'openai', status: 'active' },
  { id: 11, name: 'Disabled OpenAI group', platform: 'openai', status: 'disabled' },
  { id: 20, name: 'Anthropic group', platform: 'anthropic', status: 'active' }
] as any

function mountDialog() {
  return mount(SyncFromCpaDialog, {
    props: { show: true, groups },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        GroupSelector: GroupSelectorStub
      }
    }
  })
}

describe('SyncFromCpaDialog', () => {
  beforeEach(() => {
    previewFromCpa.mockReset()
    syncFromCpa.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
  })

  it('previews healthy accounts, keeps the selected group and OAuth options, then syncs', async () => {
    previewFromCpa.mockResolvedValue({
      accounts: [{ file_name: 'one.json', name: 'One', email: 'one@example.test', platform: 'openai', existing: false }],
      total: 2,
      skipped: 1,
      skip_reasons: { abnormal: 1 },
      batch_size: 100
    })
    syncFromCpa.mockResolvedValue({
      created: 1,
      updated: 0,
      skipped: 0,
      failed: 0,
      items: [{ file_name: 'one.json', action: 'created', account_id: 42 }]
    })

    const wrapper = mountDialog()
    await wrapper.find('#cpa-base-url').setValue(' https://cpa.example.com/ ')
    await wrapper.find('#cpa-password').setValue('handler-fixture-password')
    await wrapper.get('[data-testid="choose-group"]').trigger('click')
    await wrapper.get('[data-testid="cpa-option-prewarm_continuation"]').setValue(true)

    await wrapper.find('form#sync-from-cpa-form').trigger('submit')
    await flushPromises()

    expect(previewFromCpa).toHaveBeenCalledTimes(1)
    expect(previewFromCpa.mock.calls[0]![0]).toEqual({
      base_url: 'https://cpa.example.com/',
      management_password: 'handler-fixture-password',
      platform: 'openai'
    })
    expect(previewFromCpa.mock.calls[0]![1]).toBeInstanceOf(AbortSignal)
    expect(wrapper.find('[data-testid="cpa-preview-summary"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="cpa-account-selection"]')).toHaveLength(1)
    expect((wrapper.find('[data-testid="cpa-account-selection"]').element as HTMLInputElement).checked).toBe(true)

    await wrapper.get('[data-testid="cpa-sync"]').trigger('click')
    await flushPromises()

    expect(syncFromCpa).toHaveBeenCalledTimes(1)
    expect(syncFromCpa.mock.calls[0]![0]).toEqual(expect.objectContaining({
      base_url: 'https://cpa.example.com/',
      management_password: 'handler-fixture-password',
      platform: 'openai',
      selected_files: ['one.json'],
      group_ids: [10],
      apply_settings_to_existing: false,
      oauth_options: expect.objectContaining({
        tls_fingerprint: false,
        prewarm_continuation: true,
        ws_mode: 'off'
      })
    }))
    expect(syncFromCpa.mock.calls[0]![1]).toBeInstanceOf(AbortSignal)
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.syncCompleted')
    expect(wrapper.find('[data-testid="cpa-result-summary"]').exists()).toBe(true)

    await wrapper.find('button.btn-secondary').trigger('click')
    expect(wrapper.emitted('synced')).toHaveLength(1)
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    expect((wrapper.get('#cpa-password').element as HTMLInputElement).value).toBe('')
  })

  it('shows only platform-specific OAuth options after changing platform', async () => {
    const wrapper = mountDialog()

    await wrapper.get('#cpa-platform').setValue('anthropic')

    expect(wrapper.find('[data-testid="cpa-option-session_id_masking"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="cpa-option-intercept_warmup"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="cpa-option-prewarm_continuation"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="cpa-option-ws_mode"]').exists()).toBe(false)
  })
})
