import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import QuotaUsageDistributionChart from '../presentation/widgets/QuotaUsageDistributionChart.vue'

vi.mock('chart.js', () => ({
  Chart: { register: vi.fn() },
  BarElement: {},
  CategoryScale: {},
  LinearScale: {},
  Tooltip: {},
}))

vi.mock('vue-chartjs', () => ({
  Bar: defineComponent({
    name: 'Bar',
    props: {
      data: { type: Object, required: true },
      options: { type: Object, required: true },
    },
    template: '<div class="bar-chart" />',
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, number>) => params?.count == null ? key : `${key}:${params.count}`,
    }),
  }
})

const distribution = {
  average_used_percent: 68.25,
  measured_accounts: 12,
  unknown_accounts: 3,
  buckets: [
    { key: '0_20', min_percent: 0, max_percent: 20, count: 1 },
    { key: '20_40', min_percent: 20, max_percent: 40, count: 2 },
    { key: '40_70', min_percent: 40, max_percent: 70, count: 3 },
    { key: '70_90', min_percent: 70, max_percent: 90, count: 2 },
    { key: '90_100', min_percent: 90, max_percent: 100, count: 3 },
    { key: 'over_100', min_percent: 100, count: 1 },
  ],
}

describe('QuotaUsageDistributionChart', () => {
  it('renders all quota buckets and the uncapped average', () => {
    const wrapper = mount(QuotaUsageDistributionChart, {
      props: { distribution, loading: false },
    })

    const bar = wrapper.getComponent({ name: 'Bar' })
    expect(bar.props('data')).toMatchObject({
      labels: [
        'admin.accountInspection.quotaUsage.buckets.0_20',
        'admin.accountInspection.quotaUsage.buckets.20_40',
        'admin.accountInspection.quotaUsage.buckets.40_70',
        'admin.accountInspection.quotaUsage.buckets.70_90',
        'admin.accountInspection.quotaUsage.buckets.90_100',
        'admin.accountInspection.quotaUsage.buckets.over_100',
      ],
      datasets: [{ data: [1, 2, 3, 2, 3, 1] }],
    })
    expect(wrapper.text()).toContain('68.3%')
    expect(wrapper.text()).toContain('admin.accountInspection.quotaUsage.calculation')
  })

  it('shows the empty state when no account has a measurable quota', () => {
    const wrapper = mount(QuotaUsageDistributionChart, {
      props: {
        distribution: { average_used_percent: null, measured_accounts: 0, unknown_accounts: 4, buckets: [] },
        loading: false,
      },
    })

    expect(wrapper.findComponent({ name: 'Bar' }).exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.accountInspection.quotaUsage.empty')
    expect(wrapper.text()).toContain('4')
  })
})
