import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import CompositeRoutesModal from '@/features/admin-groups/presentation/widgets/CompositeRoutesDialog.vue'

const { listRoutes, createRoute, updateRoute, deleteRoute, showError, showSuccess } = vi.hoisted(() => ({
  listRoutes: vi.fn(),
  createRoute: vi.fn(),
  updateRoute: vi.fn(),
  deleteRoute: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/features/admin-groups/data/datasources/adminGroupQueries', () => ({
  listCompositeRoutes: listRoutes,
  previewCompositeRoute: vi.fn(),
}))

vi.mock('@/features/admin-groups/data/datasources/adminGroupActions', () => ({
  createCompositeRoute: createRoute,
  updateCompositeRoute: updateRoute,
  deleteCompositeRoute: deleteRoute,
}))

vi.mock('@/core/stores/appStore', () => ({
  useAppStore: () => ({ showError, showSuccess }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

const group = { id: 7, name: 'Composite', platform: 'composite' } as any

function mountModal() {
  return mount(CompositeRoutesModal, {
    props: { show: true, group },
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        Select: true,
        PlatformIcon: true,
        Icon: true,
      },
    },
  })
}

describe('CompositeRoutesModal', () => {
  beforeEach(() => {
    listRoutes.mockReset()
    createRoute.mockReset()
    updateRoute.mockReset()
    deleteRoute.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('labels an empty prefix upstream model as request passthrough', async () => {
    listRoutes.mockResolvedValueOnce([
      {
        id: 1,
        group_id: 7,
        public_model: 'deepseek-v4',
        match_type: 'prefix',
        target_platform: 'openai',
        upstream_model: '',
        endpoint: 'responses',
        priority: 100,
        enabled: true,
        notes: '',
      },
    ])

    const wrapper = mountModal()
    await flushPromises()

    expect(listRoutes).toHaveBeenCalledWith(7)
    expect(wrapper.text()).toContain('admin.groups.compositeRoutes.passthroughRequestedModel')
    expect(wrapper.text()).toContain('admin.groups.compositeRoutes.upstreamModelHint')
  })

  it('keeps an empty upstream model in the create payload', async () => {
    listRoutes.mockResolvedValue([])
    createRoute.mockResolvedValue({})
    const wrapper = mountModal()
    await flushPromises()

    await wrapper.get('[data-testid="composite-route-public-model"]').setValue('deepseek-v4')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(createRoute).toHaveBeenCalledWith(
      7,
      expect.objectContaining({ public_model: 'deepseek-v4', upstream_model: '' }),
    )
  })

  it('does not let a stale create reset or reload a newly opened group', async () => {
    let resolveCreate!: (value: unknown) => void
    listRoutes.mockResolvedValue([])
    createRoute.mockReturnValue(new Promise((resolve) => { resolveCreate = resolve }))
    const wrapper = mountModal()
    await flushPromises()

    await wrapper.get('[data-testid="composite-route-public-model"]').setValue('group-a-model')
    await wrapper.get('form').trigger('submit.prevent')
    expect(createRoute).toHaveBeenCalledWith(
      7,
      expect.objectContaining({ public_model: 'group-a-model' }),
    )

    const nextGroup = { ...group, id: 8, name: 'Composite B' }
    await wrapper.setProps({ show: false, group: null })
    await wrapper.setProps({ show: true, group: nextGroup })
    await flushPromises()
    await wrapper.get('[data-testid="composite-route-public-model"]').setValue('group-b-draft')

    resolveCreate({})
    await flushPromises()

    expect(
      (wrapper.get('[data-testid="composite-route-public-model"]').element as HTMLInputElement)
        .value,
    ).toBe('group-b-draft')
    expect(listRoutes.mock.calls.filter(([groupId]) => groupId === 8)).toHaveLength(1)
    expect(showSuccess).not.toHaveBeenCalled()
  })

  it('does not let a stale delete clear or reload a newly opened group', async () => {
    let resolveDelete!: (value: unknown) => void
    listRoutes.mockImplementation(async (groupId: number) =>
      groupId === 7
        ? [{
            id: 1,
            group_id: 7,
            public_model: 'group-a-model',
            match_type: 'exact',
            target_platform: 'openai',
            upstream_model: 'gpt-test',
            endpoint: 'responses',
            priority: 100,
            enabled: true,
            notes: '',
          }]
        : [],
    )
    deleteRoute.mockReturnValue(new Promise((resolve) => { resolveDelete = resolve }))
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const wrapper = mountModal()
    await flushPromises()

    const deleteButton = wrapper
      .findAll('button')
      .find((button) => button.attributes('title') === 'common.delete')
    expect(deleteButton).toBeDefined()
    await deleteButton?.trigger('click')
    expect(deleteRoute).toHaveBeenCalledWith(7, 1)

    const nextGroup = { ...group, id: 8, name: 'Composite B' }
    await wrapper.setProps({ show: false, group: null })
    await wrapper.setProps({ show: true, group: nextGroup })
    await flushPromises()
    await wrapper.get('[data-testid="composite-route-public-model"]').setValue('group-b-draft')

    resolveDelete({})
    await flushPromises()

    expect(
      (wrapper.get('[data-testid="composite-route-public-model"]').element as HTMLInputElement)
        .value,
    ).toBe('group-b-draft')
    expect(listRoutes.mock.calls.filter(([groupId]) => groupId === 8)).toHaveLength(1)
    expect(showSuccess).not.toHaveBeenCalled()
  })
})
