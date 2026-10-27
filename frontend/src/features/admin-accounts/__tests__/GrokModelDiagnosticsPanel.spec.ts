import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
const mocks = vi.hoisted(() => ({ getById: vi.fn(), syncUpstreamModels: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('../data/datasources/adminAccountQueries', () => ({ getById: mocks.getById }))
vi.mock('../data/datasources/adminAccountActions', () => ({ syncUpstreamModels: mocks.syncUpstreamModels }))
import Panel from '../presentation/widgets/GrokModelDiagnosticsPanel.vue'
const diag = { state: 'fresh', models: ['grok-4.6'], mappings: [{ requested: 'alias', upstream: 'grok-4.5', status: 'not_listed' }] }
describe('Grok catalog diagnostics', () => {
  beforeEach(() => { vi.resetAllMocks(); mocks.getById.mockResolvedValue({ grok_model_diagnostics: diag }) })
  it('shows saved diagnostics, refreshes explicitly and preserves the old snapshot on failure', async () => {
    const wrapper = mount(Panel, { props: { accountId: 1 } })
    await flushPromises()
    expect(mocks.getById).toHaveBeenCalledWith(1)
    expect(mocks.syncUpstreamModels).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('grok-4.6')
    mocks.syncUpstreamModels.mockRejectedValue(new Error('offline'))
    await wrapper.get('button').trigger('click'); await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('grok-4.6')
  })
  it('supports older servers and ignores late responses from a different account', async () => {
    let resolveOld!: (value: unknown) => void
    mocks.getById.mockReturnValueOnce(new Promise(resolve => { resolveOld = resolve })).mockResolvedValueOnce({})
    const wrapper = mount(Panel, { props: { accountId: 1 } })
    await wrapper.setProps({ accountId: 2 }); await flushPromises()
    resolveOld({ grok_model_diagnostics: diag }); await flushPromises()
    expect(wrapper.find('[data-testid="catalog-state"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('grokCatalog.unavailable')
  })
})
