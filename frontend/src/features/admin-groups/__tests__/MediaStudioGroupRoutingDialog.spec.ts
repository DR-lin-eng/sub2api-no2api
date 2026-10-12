import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MediaStudioGroupRoutingDialog from '@/features/admin-groups/presentation/widgets/MediaStudioGroupRoutingDialog.vue'
import type { AdminGroup } from '@/features/admin-groups/data/dtos/adminGroupDtos'
import type { MediaStudioGroupRoutes } from '@/features/admin-groups/data/datasources/mediaStudioGroupRouteDatasource'

const { getGroups, getModels, getRoutes, saveRoutes } = vi.hoisted(() => ({
  getGroups: vi.fn(),
  getModels: vi.fn(),
  getRoutes: vi.fn(),
  saveRoutes: vi.fn(),
}))

vi.mock('@/features/admin-groups/data/datasources/adminGroupQueries', () => ({
  getAllIncludingInactive: getGroups,
  getMediaStudioModels: getModels,
}))

vi.mock('@/features/admin-groups/data/datasources/mediaStudioGroupRouteDatasource', () => ({
  getMediaStudioGroupRoutes: getRoutes,
  saveMediaStudioGroupRoutes: saveRoutes,
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

function group(id: number, status: 'active' | 'inactive' = 'active'):
  Pick<AdminGroup, 'id' | 'name' | 'platform' | 'status'> {
  return { id, name: `Group ${id}`, platform: 'openai', status }
}

function route(group_id: number, models = ['gpt-image-2']): MediaStudioGroupRoutes[number] {
  return { group_id, priority: group_id, enabled: true, models }
}

async function openDialog(groups: ReturnType<typeof group>[], routes: MediaStudioGroupRoutes) {
  getGroups.mockResolvedValue(groups)
  getRoutes.mockResolvedValue(routes)
  const wrapper = mount(MediaStudioGroupRoutingDialog, {
    props: { visible: false },
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        Icon: true,
      },
    },
  })
  await wrapper.setProps({ visible: true })
  await flushPromises()
  return wrapper
}

describe('MediaStudioGroupRoutingDialog historical routes', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getModels.mockResolvedValue(['gpt-image-2'])
    saveRoutes.mockResolvedValue([])
  })

  it('drops deleted and inactive groups while retaining active route order and configured models', async () => {
    const wrapper = await openDialog(
      [group(1), group(2), group(3, 'inactive')],
      [route(99), route(2, ['custom-image-model']), route(3), route(1)],
    )

    expect(getModels.mock.calls).toEqual([[1, 'openai'], [2, 'openai']])
    expect(wrapper.text()).toContain('custom-image-model')
    await wrapper.get('.btn-primary').trigger('click')
    await flushPromises()

    expect(saveRoutes).toHaveBeenCalledWith([
      { group_id: 2, priority: 0, enabled: true, models: ['custom-image-model'] },
      { group_id: 1, priority: 1, enabled: true, models: ['gpt-image-2'] },
    ])
    expect(wrapper.emitted('saved')).toHaveLength(1)
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('allows removing a configuration whose routes are all unavailable', async () => {
    const wrapper = await openDialog([group(1), group(3, 'inactive')], [route(99), route(3)])

    expect(getModels).not.toHaveBeenCalled()
    expect(wrapper.get('input[type="checkbox"]').element).toHaveProperty('checked', false)
    await wrapper.get('.btn-primary').trigger('click')
    await flushPromises()

    expect(saveRoutes).toHaveBeenCalledWith([])
  })

  it('keeps model selection validation for active groups after removing stale routes', async () => {
    const wrapper = await openDialog([group(1)], [route(99), route(1, [])])

    await wrapper.get('.btn-primary').trigger('click')
    await flushPromises()

    expect(saveRoutes).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('admin.groups.mediaStudioRouting.modelRequired')
    expect(wrapper.emitted('saved')).toBeUndefined()
  })
})
