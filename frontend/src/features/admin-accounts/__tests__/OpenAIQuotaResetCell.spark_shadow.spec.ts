import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OpenAIQuotaResetCell from '@/features/admin-accounts/presentation/widgets/OpenAIQuotaResetCell.vue'
import type { Account } from '@/types'
import {
  refreshOpenAIQuota,
  resetOpenAIQuota
} from '@/features/admin-accounts/data/datasources/adminAccountsDatasource'

vi.mock('@/features/admin-accounts/data/datasources/adminAccountsDatasource', () => ({
  refreshOpenAIQuota: vi.fn(),
  resetOpenAIQuota: vi.fn(),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params?.time ? `${key}:${params.time}` : params?.count ? `${key}:${params.count}` : key,
    }),
  }
})

function makeAccount(overrides: Partial<Account>): Account {
  return {
    id: 1,
    name: 'acc',
    platform: 'openai',
    type: 'oauth',
    proxy_id: null,
    concurrency: 3,
    priority: 50,
    status: 'active',
    error_message: null,
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    schedulable: true,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null,
    ...overrides,
  }
}

// 第二个按钮(橙色)是 reset 按钮::disabled="resetting||loading||!canReset" :title="resetButtonTitle"
const resetButton = (wrapper: ReturnType<typeof mount>) =>
  wrapper.get('[data-testid="openai-reset-button"]')

beforeEach(() => {
  vi.mocked(refreshOpenAIQuota).mockReset()
  vi.mocked(resetOpenAIQuota).mockReset()
})

describe('OpenAIQuotaResetCell — 外审 F6:影子禁用重置', () => {
  it('影子账号(parent_account_id 非空)的 reset 按钮被禁用且提示在母账号重置', () => {
    const account = makeAccount({ parent_account_id: 100 })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    const btn = resetButton(wrapper)
    expect(btn.attributes('disabled')).toBeDefined()
    expect(btn.attributes('title')).toBe('admin.accounts.openaiQuotaReset.resetTooltipShadow')
    wrapper.unmount()
  })

  it('普通账号(无 parent_account_id)未查询时禁用原因是「需先查询」而非影子提示', () => {
    const account = makeAccount({ parent_account_id: null })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    const btn = resetButton(wrapper)
    // 未加载数据时本就 disabled(无次数),但提示语必须是 needQuery,不得是 shadow 提示。
    expect(btn.attributes('title')).toBe('admin.accounts.openaiQuotaReset.resetTooltipNeedQuery')
    wrapper.unmount()
  })

  it('查询后默认折叠为最早到期时间,点击 +N 展开完整列表', async () => {
    vi.mocked(refreshOpenAIQuota).mockResolvedValue({
      rate_limit_reset_credits: {
        available_count: 3,
        credits: [
          { expires_at: '2026-07-05T04:05:06Z' },
          { expires_at: '2026-07-03T04:05:06Z' },
          { expires_at: 'not-a-date' },
        ],
      },
      fetched_at: 1770000000,
      cache_persisted: true,
    })

    const account = makeAccount({ parent_account_id: null })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    await wrapper.findAll('button')[0].trigger('click')
    await flushPromises()

    expect(refreshOpenAIQuota).toHaveBeenCalledWith(1)
    expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.expiresAt:')
    expect(wrapper.text()).toContain('+2')
    expect(wrapper.text()).not.toContain('not-a-date')

    const toggle = wrapper.find('[data-testid="reset-credit-expiry-toggle"]')
    expect(toggle.exists()).toBe(true)
    expect(toggle.attributes('aria-expanded')).toBe('false')
    await toggle.trigger('click')

    expect(toggle.attributes('aria-expanded')).toBe('true')
    expect(wrapper.find('[data-testid="reset-credit-expiry-details"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('not-a-date')
    expect(wrapper.text()).not.toContain('undefined')
    wrapper.unmount()
  })

  it('只有一张重置卡时不显示展开按钮', async () => {
    vi.mocked(refreshOpenAIQuota).mockResolvedValue({
      rate_limit_reset_credits: {
        available_count: 1,
        credits: [
          { expires_at: '2026-07-03T04:05:06Z' },
        ],
      },
      fetched_at: 1770000000,
      cache_persisted: true,
    })

    const account = makeAccount({ parent_account_id: null })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    await wrapper.findAll('button')[0].trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="reset-credit-expiry-toggle"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="reset-credit-expiry-details"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.expiresAt:')
    wrapper.unmount()
  })

  it('从账号缓存恢复次数时会丢弃过期卡并收紧可用次数', () => {
    const dateNowSpy = vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2026-08-05T00:00:00Z'))
    const account = makeAccount({
      extra: {
        codex_reset_credit_snapshot: {
          available_count: 2,
          credits: [
            { expires_at: '2026-08-01T00:00:00Z' },
            { expires_at: '2026-08-10T00:00:00Z' },
          ],
        },
      },
    })

    const wrapper = mount(OpenAIQuotaResetCell, { props: { account } })

    try {
      expect(wrapper.get('[data-testid="openai-reset-credit-count"]').text()).toContain('1')
      expect(resetButton(wrapper).attributes('disabled')).toBeUndefined()
      expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.expiresAt:')
    } finally {
      wrapper.unmount()
      dateNowSpy.mockRestore()
    }
  })

  it('实时查询成功但缓存写入失败时仍显示次数并给出局部失败提示', async () => {
    vi.mocked(refreshOpenAIQuota).mockResolvedValue({
      rate_limit_reset_credits: {
        available_count: 1,
        credits: [],
      },
      fetched_at: 1770000000,
      cache_persisted: false,
    })
    const wrapper = mount(OpenAIQuotaResetCell, {
      props: { account: makeAccount({}) },
    })

    await wrapper.findAll('button')[0].trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="openai-reset-credit-count"]').text()).toContain('1')
    expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.refreshCachePersistFailed')
    wrapper.unmount()
  })

  it('查询后展示 App Server 返回的多个 rate-limit 桶', async () => {
    vi.mocked(refreshOpenAIQuota).mockResolvedValue({
      rate_limits_by_limit_id: {
        codex: {
          limit_id: 'codex',
          limit_name: null,
          primary: {
            used_percent: 25,
            window_duration_mins: 15,
            resets_at: 1730947200,
          },
        },
        codex_other: {
          limit_id: 'codex_other',
          limit_name: 'codex_other',
          primary: {
            used_percent: 42,
            window_duration_mins: 60,
            resets_at: 1730950800,
          },
        },
      },
      rate_limit_reset_credits: { available_count: 0, credits: [] },
      fetched_at: 1770000000,
      cache_persisted: true,
    })

    const wrapper = mount(OpenAIQuotaResetCell, {
      props: { account: makeAccount({}) },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt', 'color'],
            template: '<div class="rate-limit-bar">{{ label }}|{{ utilization }}|{{ resetsAt }}</div>',
          },
        },
      },
    })

    await wrapper.findAll('button')[0].trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="openai-rate-limit-buckets"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('15m|25|2024-11-07T02:40:00.000Z')
    expect(wrapper.text()).toContain('codex_other 1h|42|2024-11-07T03:40:00.000Z')
    expect(wrapper.text()).toContain('15m')
    expect(wrapper.text()).toContain('1h')
    wrapper.unmount()
  })

  it('账号行自动刷新替换 extra 后仍保留已查询的桶百分比', async () => {
    vi.mocked(refreshOpenAIQuota).mockResolvedValue({
      rate_limits_by_limit_id: {
        codex: {
          limit_id: 'codex',
          primary: {
            used_percent: 100,
            window_duration_mins: 10080,
            resets_at: 1730947200,
          },
        },
      },
      fetched_at: 1770000000,
      cache_persisted: true,
    })

    const wrapper = mount(OpenAIQuotaResetCell, {
      props: { account: makeAccount({}) },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization'],
            template: '<div class="rate-limit-bar">{{ label }}|{{ utilization }}</div>',
          },
        },
      },
    })

    await wrapper.find('[data-testid="openai-primary-query"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('7d|100')

    await wrapper.setProps({
      account: makeAccount({
        extra: {
          codex_reset_credit_snapshot: {
            available_count: 0,
            credits: [],
          },
        },
      }),
    })
    await flushPromises()

    expect(wrapper.text()).toContain('7d|100')
    wrapper.unmount()
  })

  it('账号列表携带持久化桶快照时首屏直接显示百分比', () => {
    const wrapper = mount(OpenAIQuotaResetCell, {
      props: {
        account: makeAccount({
          id: 1900,
          extra: {
            codex_rate_limit_snapshot: {
              fetched_at: 1770000000,
              rate_limits_by_limit_id: {
                codex: {
                  limit_id: 'codex',
                  primary: {
                    used_percent: 88,
                    window_duration_mins: 10080,
                    resets_at: 1730947200,
                  },
                },
              },
            },
          },
        }),
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization'],
            template: '<div class="rate-limit-bar">{{ label }}|{{ utilization }}</div>',
          },
        },
      },
    })

    expect(wrapper.text()).toContain('7d|88')
    wrapper.unmount()
  })

  it('没有主动快照时按规范化窗口显示被动额度并标明来源', () => {
    const wrapper = mount(OpenAIQuotaResetCell, {
      props: {
        account: makeAccount({
          id: 1901,
          extra: {
            codex_usage_updated_at: '2026-08-27T00:00:00Z',
            codex_5h_used_percent: 21,
            codex_5h_window_minutes: 300,
            codex_5h_reset_at: '2026-08-27T05:00:00Z',
            codex_7d_used_percent: 54,
            codex_7d_window_minutes: 10080,
            codex_7d_reset_at: '2026-09-03T00:00:00Z'
          }
        })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization'],
            template: '<div class="rate-limit-bar">{{ label }}|{{ utilization }}</div>'
          }
        }
      }
    })

    expect(wrapper.text()).toContain('5h|21')
    expect(wrapper.text()).toContain('7d|54')
    expect(wrapper.text()).toContain('admin.accounts.usageWindow.passiveSampled')
    expect(refreshOpenAIQuota).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('主动持久化桶优先于可能不同的被动响应头快照', () => {
    const wrapper = mount(OpenAIQuotaResetCell, {
      props: {
        account: makeAccount({
          id: 1902,
          extra: {
            codex_5h_used_percent: 91,
            codex_5h_window_minutes: 300,
            codex_rate_limit_snapshot: {
              fetched_at: 1770000000,
              rate_limits_by_limit_id: {
                codex: {
                  limit_id: 'codex',
                  primary: {
                    used_percent: 24,
                    window_duration_mins: 300
                  }
                }
              }
            }
          }
        })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization'],
            template: '<div class="rate-limit-bar">{{ label }}|{{ utilization }}</div>'
          }
        }
      }
    })

    expect(wrapper.text()).toContain('5h|24')
    expect(wrapper.text()).not.toContain('5h|91')
    expect(wrapper.text()).not.toContain('admin.accounts.usageWindow.passiveSampled')
    wrapper.unmount()
  })

  it('被动窗口已经重置时不继续显示旧占用比例', () => {
    const wrapper = mount(OpenAIQuotaResetCell, {
      props: {
        account: makeAccount({
          id: 1903,
          extra: {
            codex_5h_used_percent: 87,
            codex_5h_reset_at: '2020-01-01T00:00:00Z'
          }
        })
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization'],
            template: '<div class="rate-limit-bar">{{ label }}|{{ utilization }}</div>'
          }
        }
      }
    })

    expect(wrapper.text()).toContain('5h|0')
    expect(wrapper.text()).not.toContain('5h|87')
    wrapper.unmount()
  })

  it('主查询也会刷新父组件提供的本地计数', async () => {
    const queryLocalUsage = vi.fn().mockResolvedValue(undefined)
    vi.mocked(refreshOpenAIQuota).mockResolvedValue({
      rate_limits_by_limit_id: {
        codex: {
          limit_id: 'codex',
          primary: {
            used_percent: 25,
            window_duration_mins: 10080,
            resets_at: 1730947200,
          },
        },
      },
      fetched_at: 1770000000,
      cache_persisted: true,
    })

    const wrapper = mount(OpenAIQuotaResetCell, {
      props: { account: makeAccount({}), queryLocalUsage },
    })

    await wrapper.find('[data-testid="openai-primary-query"]').trigger('click')
    await flushPromises()

    expect(queryLocalUsage).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('父组件提供查询按钮时，次数只作为只读徽章显示', () => {
    const wrapper = mount(OpenAIQuotaResetCell, {
      props: { account: makeAccount({}) },
      slots: {
        'pre-actions': '<button data-testid="parent-query">查询</button>',
      },
    })

    expect(wrapper.find('[data-testid="parent-query"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="openai-reset-credit-count"]').element.tagName).toBe('SPAN')
    expect(wrapper.find('[data-testid="openai-secondary-quota-query"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('保留窗口本地计数并展示查询得到的服务端 Token 计数', async () => {
    vi.mocked(refreshOpenAIQuota).mockResolvedValue({
      rate_limits_by_limit_id: {
        codex: {
          limit_id: 'codex',
          primary: {
            used_percent: 25,
            window_duration_mins: 300,
            resets_at: 1730947200,
          },
        },
      },
      server_token_usage: {
        summary: {
          lifetime_tokens: 1234,
          peak_daily_tokens: 456,
          longest_running_turn_seconds: 12,
          current_streak_days: 3,
          longest_streak_days: 7,
        },
        current_reset_cycle_tokens: 777,
        current_reset_cycle_window_minutes: 10080,
        current_reset_cycle_limit_id: 'codex',
        current_reset_cycle_approximate: false,
        daily_usage_buckets: [{ start_date: '2026-08-23', tokens: 321 }],
      },
      fetched_at: 1770000000,
      cache_persisted: true,
    })

    const wrapper = mount(OpenAIQuotaResetCell, {
      props: {
        account: makeAccount({}),
        localWindowStats: {
          five_hour: {
            requests: 9,
            tokens: 900,
            cost: 0.09,
            standard_cost: 0.09,
            user_cost: 0.04,
          },
        },
      },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt', 'windowStats', 'color'],
            template: '<div class="rate-limit-bar">{{ label }}|{{ utilization }}|{{ windowStats?.requests }}|{{ windowStats?.tokens }}|{{ windowStats?.cost }}|{{ windowStats?.user_cost }}</div>',
          },
        },
      },
    })

    await wrapper.findAll('button')[0].trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('5h|25|9|900|0.09|0.04')
    expect(wrapper.find('[data-testid="openai-server-token-usage"]').text()).toContain('admin.accounts.openaiQuotaReset.serverUsageFields.lifetimeTokens: 1.2K')
    expect(wrapper.find('[data-testid="openai-server-token-usage"]').text()).toContain('admin.accounts.openaiQuotaReset.serverUsageFields.currentResetCycleTokens (7d): 777')
    expect(wrapper.text()).toContain('admin.accounts.openaiQuotaReset.serverUsageFields.dailyBucket 2026-08-23: 321')
    wrapper.unmount()
  })

  it('未知桶保留英文原字段和值而不静默丢弃', async () => {
    vi.mocked(refreshOpenAIQuota).mockResolvedValue({
      rate_limits_by_limit_id: {
        'gpt-aaa': {
          limit_id: 'gpt-aaa',
          raw_value: 100,
        },
        future: {
          limitId: 'future',
          raw_fields: {
            windowDurationMins: 100,
            newField: 'keep-me',
          },
        },
      },
      fetched_at: 1770000000,
      cache_persisted: true,
    })

    const wrapper = mount(OpenAIQuotaResetCell, { props: { account: makeAccount({}) } })
    await wrapper.findAll('button')[0].trigger('click')
    await flushPromises()

    const buckets = wrapper.find('[data-testid="openai-rate-limit-buckets"]')
    expect(buckets.text()).toContain('gpt-aaa')
    expect(buckets.text()).toContain('100')
    expect(buckets.text()).toContain('windowDurationMins: 100')
    expect(buckets.text()).toContain('newField: keep-me')
    wrapper.unmount()
  })

  it('直接收到标量未知桶时也按原始英文键值显示', async () => {
    vi.mocked(refreshOpenAIQuota).mockResolvedValue({
      rateLimitsByLimitId: { 'gpt-aaa': 100 },
      fetched_at: 1770000000,
      cache_persisted: true,
    })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account: makeAccount({}) } })
    await wrapper.findAll('button')[0].trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="openai-rate-limit-buckets"]').text()).toContain('gpt-aaa:100')
    wrapper.unmount()
  })

  it('兼容直接传入的 camelCase App Server 字段', async () => {
    vi.mocked(refreshOpenAIQuota).mockResolvedValue({
      rateLimitsByLimitId: {
        codex_other: {
          limitId: 'codex_other',
          limitName: 'Other',
          primary: {
            usedPercent: 11,
            windowDurationMins: 30,
            resetsAt: 1730950800,
          },
        },
      },
      fetched_at: 1770000000,
      cache_persisted: true,
    })

    const wrapper = mount(OpenAIQuotaResetCell, {
      props: { account: makeAccount({}) },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt', 'color'],
            template: '<div class="rate-limit-bar">{{ label }}|{{ utilization }}</div>',
          },
        },
      },
    })

    await wrapper.findAll('button')[0].trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Other 30m|11')
    expect(wrapper.text()).toContain('30m')
    wrapper.unmount()
  })

  it('没有多桶时回退显示旧的单桶窗口', async () => {
    vi.mocked(refreshOpenAIQuota).mockResolvedValue({
      rate_limit: {
        allowed: true,
        limit_reached: false,
        primary_window: {
          used_percent: 63,
          limit_window_seconds: 18000,
          reset_after_seconds: 1200,
          reset_at: 1730947200,
        },
      },
      fetched_at: 1770000000,
      cache_persisted: true,
    })

    const wrapper = mount(OpenAIQuotaResetCell, {
      props: { account: makeAccount({}) },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt', 'color'],
            template: '<div class="rate-limit-bar">{{ label }}|{{ utilization }}</div>',
          },
        },
      },
    })

    await wrapper.findAll('button')[0].trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('5h|63')
    wrapper.unmount()
  })

  it('按 windowDurationMins 标注仅返回的周窗口', async () => {
    vi.mocked(refreshOpenAIQuota).mockResolvedValue({
      rate_limits_by_limit_id: {
        codex: {
          limit_id: 'codex',
          primary: {
            used_percent: 33,
            window_duration_mins: 10080,
            resets_at: 1730950800,
          },
        },
      },
      fetched_at: 1770000000,
      cache_persisted: true,
    })

    const wrapper = mount(OpenAIQuotaResetCell, {
      props: { account: makeAccount({}) },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization', 'resetsAt', 'color'],
            template: '<div class="rate-limit-bar">{{ label }}|{{ utilization }}</div>',
          },
        },
      },
    })

    await wrapper.findAll('button')[0].trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('7d|33')
    expect(wrapper.text()).not.toContain('10080m')
    wrapper.unmount()
  })
})
