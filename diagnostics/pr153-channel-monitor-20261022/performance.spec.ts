import { createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { expect, it, vi } from 'vitest'
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key }),
}))
import Grid from '@/features/channel-monitor-user/presentation/widgets/MonitorCardGrid.vue'
import type { UserMonitorView } from '@/features/channel-monitor-user/data/datasources/channelMonitorUserDatasource'

it('measures the same 360-row, 60-history-point grouping and render workload', () => {
  const items = Array.from({ length: 360 }, (_, index) => ({
    id: index + 1, name: `Fixture ${index}`, group_name: `Group ${index}`,
    provider: ['openai', 'anthropic', 'gemini'][index % 3], monitor_mode: index % 2 ? 'active' : 'passive',
    primary_model: 'fixture', primary_status: index % 5 ? 'operational' : 'degraded',
    primary_latency_ms: index, availability_7d: 99, extra_models: [],
    timeline: Array.from({ length: 60 }, (_, point) => ({
      status: (point + index) % 7 ? 'operational' : 'failed', latency_ms: 50,
      checked_at: '2026-10-22T00:00:00Z',
    })),
  })) as UserMonitorView[]
  const samples: number[] = []
  for (let iteration = 0; iteration < 16; iteration++) {
    const started = performance.now()
    const wrapper = mount(Grid, { props: { items, window: '7d', countdownSeconds: 10, timelinePoints: 20, loading: false, detailCache: {} },
      global: { plugins: [createPinia()], stubs: { MonitorTimeline: true, ProviderIcon: true, Icon: true } } })
    const rows = wrapper.findAll('button[class~="group/item"]').length
    wrapper.unmount()
    if (iteration >= 4) samples.push(performance.now() - started)
    expect(rows).toBe(360)
  }
  const sorted = [...samples].sort((a, b) => a - b)
  console.log('PR153_PERFORMANCE ' + JSON.stringify({ rows: 360, history_points: 60, timeline_stubbed: true, samples_ms: samples,
    median_ms: (sorted[5] + sorted[6]) / 2, p90_ms: sorted[10] }))
}, 60000)
