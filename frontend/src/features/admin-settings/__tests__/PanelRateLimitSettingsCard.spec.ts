import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import PanelRateLimitSettingsCard from '@/features/admin-settings/presentation/widgets/PanelRateLimitSettingsCard.vue'

const { getSettings, updateSettings, showError, showSuccess } = vi.hoisted(() => ({
  getSettings: vi.fn(),
  updateSettings: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/features/admin-settings/data/datasources/adminSettingsQueries', () => ({
  getPanelRateLimitSettings: getSettings,
}))

vi.mock('@/features/admin-settings/data/datasources/adminSettingsActions', () => ({
  updatePanelRateLimitSettings: updateSettings,
}))

vi.mock('@/core/stores/appStore', () => ({
  useAppStore: () => ({ showError, showSuccess }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

describe('PanelRateLimitSettingsCard', () => {
  beforeEach(() => {
    getSettings.mockReset()
    updateSettings.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
  })

  it('uses the disabled state returned by the backend without mounting numeric defaults', async () => {
    getSettings.mockResolvedValue({
      enabled: false,
      user_rpm: 240,
      heavy_rpm: 60,
      exempt_admin: true,
      public_ip_rpm: 300,
    })

    const wrapper = mount(PanelRateLimitSettingsCard)
    expect(wrapper.find('[data-testid="panel-rate-limit-enabled"]').exists()).toBe(false)
    await flushPromises()

    expect(wrapper.get('[data-testid="panel-rate-limit-enabled"]').attributes('aria-checked')).toBe('false')
    expect(wrapper.find('[data-testid="panel-rate-limit-user-rpm"]').exists()).toBe(false)
  })

  it('saves edited settings', async () => {
    getSettings.mockResolvedValue({
      enabled: true,
      user_rpm: 240,
      heavy_rpm: 60,
      exempt_admin: true,
      public_ip_rpm: 300,
    })
    updateSettings.mockImplementation(async (settings) => settings)

    const wrapper = mount(PanelRateLimitSettingsCard)
    await flushPromises()
    await wrapper.get('[data-testid="panel-rate-limit-user-rpm"]').setValue('120')
    await wrapper.get('[data-testid="panel-rate-limit-save"]').trigger('click')
    await flushPromises()

    expect(updateSettings).toHaveBeenCalledWith({
      enabled: true,
      user_rpm: 120,
      heavy_rpm: 60,
      exempt_admin: true,
      public_ip_rpm: 300,
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.settings.panelRateLimit.saved')
  })

  it('saves locally on Enter without bubbling to the settings form', async () => {
    const outerKeydown = vi.fn()
    getSettings.mockResolvedValue({
      enabled: true,
      user_rpm: 240,
      heavy_rpm: 60,
      exempt_admin: true,
      public_ip_rpm: 300,
    })
    updateSettings.mockImplementation(async (settings) => settings)

    const wrapper = mount(PanelRateLimitSettingsCard, {
      attrs: { onKeydown: outerKeydown },
    })
    await flushPromises()
    const input = wrapper.get('[data-testid="panel-rate-limit-user-rpm"]')
    await input.setValue('120')
    await input.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(updateSettings).toHaveBeenCalledTimes(1)
    expect(updateSettings).toHaveBeenCalledWith({
      enabled: true,
      user_rpm: 120,
      heavy_rpm: 60,
      exempt_admin: true,
      public_ip_rpm: 300,
    })
    expect(outerKeydown).not.toHaveBeenCalled()
  })

  it('turns a null response into the complete disabled compatibility defaults', async () => {
    getSettings.mockResolvedValue(null)
    updateSettings.mockImplementation(async (settings) => settings)

    const wrapper = mount(PanelRateLimitSettingsCard)
    await flushPromises()

    expect(wrapper.get('[data-testid="panel-rate-limit-enabled"]').attributes('aria-checked')).toBe('false')
    await wrapper.get('[data-testid="panel-rate-limit-save"]').trigger('click')
    await flushPromises()

    expect(updateSettings).toHaveBeenCalledWith({
      enabled: false,
      user_rpm: 240,
      heavy_rpm: 60,
      exempt_admin: true,
      public_ip_rpm: 300,
    })
  })

  it('fills missing and invalid fields before saving an explicitly enabled partial response', async () => {
    getSettings.mockResolvedValue({
      enabled: true,
      user_rpm: '120',
      heavy_rpm: -1,
      exempt_admin: null,
    })
    updateSettings.mockImplementation(async (settings) => settings)

    const wrapper = mount(PanelRateLimitSettingsCard)
    await flushPromises()

    expect((wrapper.get('[data-testid="panel-rate-limit-user-rpm"]').element as HTMLInputElement).value).toBe('240')
    await wrapper.get('[data-testid="panel-rate-limit-save"]').trigger('click')
    await flushPromises()

    expect(updateSettings).toHaveBeenCalledWith({
      enabled: true,
      user_rpm: 240,
      heavy_rpm: 60,
      exempt_admin: true,
      public_ip_rpm: 300,
    })
  })

  it('silently hides itself when an older backend does not expose the endpoint', async () => {
    getSettings.mockRejectedValue({ status: 404, message: 'not found' })

    const wrapper = mount(PanelRateLimitSettingsCard)
    await flushPromises()

    expect(wrapper.find('.card').exists()).toBe(false)
    expect(showError).not.toHaveBeenCalled()
    expect(updateSettings).not.toHaveBeenCalled()
  })
})
