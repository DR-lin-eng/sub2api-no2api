import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsDashboardHeader from '../OpsDashboardHeader.vue'

const mockGetGroups = vi.fn()

vi.mock('@/api', () => ({
  adminAPI: {
    groups: {
      getAll: (...args: any[]) => mockGetGroups(...args)
    }
  }
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getRealtimeTrafficSummary: vi.fn()
  }
}))

vi.mock('@/stores', () => ({
  useAdminSettingsStore: () => ({
    opsRealtimeMonitoringEnabled: false,
    setOpsRealtimeMonitoringEnabledLocal: vi.fn()
  })
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const EmptyStub = defineComponent({ template: '<div><slot /></div>' })

describe('OpsDashboardHeader', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGetGroups.mockResolvedValue([])
  })

  it('TTFT 明细按钮使用 TTFT 查询预设', async () => {
    const wrapper = mount(OpsDashboardHeader, {
      props: {
        overview: {} as any,
        platform: '',
        groupId: null,
        timeRange: '1h',
        queryMode: 'auto',
        loading: false,
        lastUpdated: null
      },
      global: {
        stubs: {
          Select: EmptyStub,
          HelpTooltip: EmptyStub,
          BaseDialog: EmptyStub,
          Icon: EmptyStub
        }
      }
    })
    await flushPromises()

    const detailsButtons = wrapper
      .findAll('button')
      .filter((button) => button.text() === 'admin.ops.requestDetails.details')
    for (const button of detailsButtons) await button.trigger('click')

    const presets = (wrapper.emitted('openRequestDetails') ?? []).map(([preset]) => preset)
    expect(presets).toContainEqual({
      title: 'admin.ops.ttftLabel',
      kind: 'success',
      sort: 'ttft_desc',
      ttft_only: true
    })
  })

  it('Redis 使用率按活跃连接计算而不是空闲连接总数', async () => {
    const wrapper = mount(OpsDashboardHeader, {
      props: {
        overview: {
          system_metrics: {
            redis_ok: true,
            redis_pool_size: 4096,
            redis_conn_total: 4096,
            redis_conn_idle: 4096
          }
        } as any,
        platform: '',
        groupId: null,
        timeRange: '1h',
        queryMode: 'auto',
        loading: false,
        lastUpdated: null
      },
      global: {
        stubs: {
          Select: EmptyStub,
          HelpTooltip: EmptyStub,
          BaseDialog: EmptyStub,
          Icon: EmptyStub
        }
      }
    })
    await flushPromises()

    const redisCard = wrapper.findAll('div').find((element) => element.text().startsWith('Redis'))
    expect(redisCard?.text()).toContain('0%')
  })

  it.each([
    { count: 22_200, status: 'admin.ops.ok' },
    { count: 30_000, status: 'common.warning' },
    { count: 50_000, status: 'common.critical' }
  ])('协程数 $count 显示 $status', async ({ count, status }) => {
    const wrapper = mount(OpsDashboardHeader, {
      props: {
        overview: {
          system_metrics: {
            goroutine_count: count
          }
        } as any,
        platform: '',
        groupId: null,
        timeRange: '1h',
        queryMode: 'auto',
        loading: false,
        lastUpdated: null
      },
      global: {
        stubs: {
          Select: EmptyStub,
          HelpTooltip: EmptyStub,
          BaseDialog: EmptyStub,
          Icon: EmptyStub
        }
      }
    })
    await flushPromises()

    const goroutineCard = wrapper.get('[data-test="goroutine-card"]')
    expect(goroutineCard.get('[data-test="goroutine-status"]').text()).toBe(status)
    expect(goroutineCard.text()).toContain('30K')
    expect(goroutineCard.text()).toContain('50K')
  })
})
