import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsImageGenerationStatsCard from '../OpsImageGenerationStatsCard.vue'

const mockGetImageGenerationStats = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getImageGenerationStats: (...args: any[]) => mockGetImageGenerationStats(...args)
  }
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const EmptyStateStub = defineComponent({
  name: 'EmptyState',
  props: {
    title: { type: String, default: '' },
    description: { type: String, default: '' }
  },
  template: '<div class="empty-state">{{ title }}|{{ description }}</div>'
})

const sampleResponse = {
  start_time: '2026-07-17T00:00:00Z',
  end_time: '2026-07-17T01:00:00Z',
  platform: 'openai',
  group_id: 7,
  request_count: 12,
  image_count: 15,
  requests_per_minute: 0.2,
  avg_duration_ms: 18000,
  p95_duration_ms: 24000,
  max_duration_ms: 32000,
  average_concurrent: 3.3,
  peak_concurrent: 6,
  realtime: {
    available: true,
    scope: 'instance' as const,
    enabled: true,
    current_concurrent: 5,
    waiting: 2,
    limit: 8,
    max_waiting: 20
  },
  by_resolution: [
    {
      resolution: '1024x1024',
      billing_tier: '1K',
      request_count: 8,
      image_count: 8,
      avg_duration_ms: 12000,
      p95_duration_ms: 15000,
      max_duration_ms: 18000
    }
  ]
}

describe('OpsImageGenerationStatsCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGetImageGenerationStats.mockResolvedValue(sampleResponse)
  })

  it('uses dashboard filters and renders resolution and concurrency data', async () => {
    const wrapper = mount(OpsImageGenerationStatsCard, {
      props: {
        timeRange: '1h',
        platformFilter: 'openai',
        groupIdFilter: 7,
        refreshToken: 1
      },
      global: { stubs: { EmptyState: EmptyStateStub } }
    })
    await flushPromises()

    expect(mockGetImageGenerationStats).toHaveBeenCalledWith(
      {
        time_range: '1h',
        platform: 'openai',
        group_id: 7
      },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
    expect(wrapper.text()).toContain('1024 × 1024')
    expect(wrapper.text()).toContain('1K')
    expect(wrapper.text()).toContain('5 / 8')
    expect(wrapper.text()).toContain('3.3 / 6')
  })

  it('uses explicit timestamps for a custom time range', async () => {
    mount(OpsImageGenerationStatsCard, {
      props: {
        timeRange: 'custom',
        customStartTime: '2026-07-17T00:00:00Z',
        customEndTime: '2026-07-17T02:00:00Z',
        refreshToken: 1
      },
      global: { stubs: { EmptyState: EmptyStateStub } }
    })
    await flushPromises()

    expect(mockGetImageGenerationStats).toHaveBeenCalledWith(
      expect.objectContaining({
        start_time: '2026-07-17T00:00:00Z',
        end_time: '2026-07-17T02:00:00Z'
      }),
      expect.any(Object)
    )
  })
})
