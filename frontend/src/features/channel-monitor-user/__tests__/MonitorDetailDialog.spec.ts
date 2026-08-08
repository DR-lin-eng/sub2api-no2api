import { createPinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { defaultFetchDetail } = vi.hoisted(() => ({
  defaultFetchDetail: vi.fn(),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('@/features/channel-monitor-user/data/datasources/channelMonitorUserDatasource', async () => {
  const actual = await vi.importActual<
    typeof import('@/features/channel-monitor-user/data/datasources/channelMonitorUserDatasource')
  >('@/features/channel-monitor-user/data/datasources/channelMonitorUserDatasource')
  return {
    ...actual,
    status: defaultFetchDetail,
  }
})

import MonitorDetailDialog from '@/features/channel-monitor-user/presentation/widgets/MonitorDetailDialog.vue'
import type { UserMonitorDetail } from '@/features/channel-monitor-user/data/datasources/channelMonitorUserDatasource'

const detail: UserMonitorDetail = {
  id: 7,
  name: 'Public monitor',
  provider: 'openai',
  group_name: 'Public',
  models: [
    {
      model: 'gpt-test',
      latest_status: 'operational',
      latest_latency_ms: 12,
      availability_7d: 100,
      availability_15d: 100,
      availability_30d: 100,
      avg_latency_7d_ms: 10,
    },
  ],
}

function mountDialog(fetchDetail?: (id: number) => Promise<UserMonitorDetail>) {
  return mount(MonitorDetailDialog, {
    props: {
      show: true,
      monitorId: detail.id,
      title: detail.name,
      fetchDetail,
    },
    global: {
      plugins: [createPinia()],
      stubs: {
        BaseDialog: {
          props: ['show', 'title'],
          template: '<section v-if="show"><slot /><slot name="footer" /></section>',
        },
      },
    },
  })
}

describe('MonitorDetailDialog detail loader', () => {
  beforeEach(() => {
    defaultFetchDetail.mockReset()
    defaultFetchDetail.mockResolvedValue(detail)
  })

  it('uses an injected loader for the anonymous share surface', async () => {
    const sharedFetchDetail = vi.fn().mockResolvedValue(detail)

    const wrapper = mountDialog(sharedFetchDetail)
    await flushPromises()

    expect(sharedFetchDetail).toHaveBeenCalledWith(detail.id)
    expect(defaultFetchDetail).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('gpt-test')
  })

  it('keeps the authenticated loader as the default', async () => {
    const wrapper = mountDialog()
    await flushPromises()

    expect(defaultFetchDetail).toHaveBeenCalledWith(detail.id)
    expect(wrapper.text()).toContain('gpt-test')
  })
})
