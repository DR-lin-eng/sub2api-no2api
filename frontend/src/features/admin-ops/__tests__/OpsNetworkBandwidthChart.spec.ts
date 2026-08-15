import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import OpsNetworkBandwidthChart from '../presentation/widgets/OpsNetworkBandwidthChart.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) => params?.names ? `${key}:${params.names}` : key
    })
  }
})

vi.mock('vue-chartjs', () => ({
  Line: {
    name: 'Line',
    props: ['data', 'options'],
    template: '<div class="line-chart" />'
  }
}))

describe('OpsNetworkBandwidthChart', () => {
  it('renders only the supplied default-route samples and interface names', () => {
    const wrapper = mount(OpsNetworkBandwidthChart, {
      props: {
        points: [
          {
            bucket_start: '2026-08-15T00:00:00Z',
            receive_bytes_per_second: 1024,
            transmit_bytes_per_second: 512
          },
          {
            bucket_start: '2026-08-15T00:01:00Z',
            receive_bytes_per_second: 2048,
            transmit_bytes_per_second: 256
          }
        ],
        interfaces: ['eth0'],
        loading: false,
        timeRange: '1h'
      },
      global: {
        stubs: {
          EmptyState: true,
          HelpTooltip: true
        }
      }
    })

    expect(wrapper.text()).toContain('admin.ops.networkBandwidth.interfaces:eth0')
    expect(wrapper.find('.line-chart').exists()).toBe(true)
    const line = wrapper.findComponent({ name: 'Line' })
    expect(line.props('data').datasets[0].data).toEqual([1024, 2048])
    expect(line.props('data').datasets[1].data).toEqual([512, 256])
  })

  it('keeps missing samples as an empty state instead of zero bandwidth', () => {
    const wrapper = mount(OpsNetworkBandwidthChart, {
      props: {
        points: [{ bucket_start: '2026-08-15T00:00:00Z' }],
        loading: false,
        timeRange: '1h'
      },
      global: {
        stubs: {
          EmptyState: { template: '<div class="empty-state" />' },
          HelpTooltip: true
        }
      }
    })

    expect(wrapper.find('.line-chart').exists()).toBe(false)
    expect(wrapper.find('.empty-state').exists()).toBe(true)
  })
})
