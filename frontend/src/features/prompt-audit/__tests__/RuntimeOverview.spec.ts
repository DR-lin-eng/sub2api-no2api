import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import type { PromptAuditMode, PromptAuditRuntime } from '../domain/models/promptAuditTypes'

const messages: Record<string, string> = {
  'admin.promptAudit.status.unknown': 'Unknown',
  'admin.promptAudit.mode.unknown': 'Unknown',
  'admin.promptAudit.runtime.dependencyStatus.unknown': 'Unknown',
  'admin.promptAudit.runtime.dependenciesValue': 'DB {database} · Redis {redis}',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      locale: { value: 'en' },
      t: (key: string, params?: Record<string, unknown>) =>
        (messages[key] || key).replace(/\{(\w+)\}/g, (_, token) => String(params?.[token] ?? `{${token}}`)),
    }),
  }
})

import RuntimeOverview from '../presentation/widgets/RuntimeOverview.vue'

const runtime = {
  process_status: 'future_process',
  effective_mode: 'future_mode' as PromptAuditMode,
  expected_config_version: 2,
  active_config_version: 1,
  worker_total: 1,
  worker_active: 1,
  queue_capacity: 10,
  queue: { staging: 0, queued: 0, processing: 0, retry: 0, done: 0, failed: 0, active: 0 },
  processed_total: 0,
  failed_total: 0,
  enqueued_total: 0,
  dropped_total: 0,
  database_status: 'future_database',
  redis_status: 'future_redis',
  endpoints: {},
  guard_metrics: {
    total: 0,
    allowed: 0,
    flagged: 0,
    blocked: 0,
    unavailable: 0,
    invalid: 0,
    timeouts: 0,
    failovers: 0,
    bulkhead_full: 0,
    record_failed: 0,
  },
} satisfies PromptAuditRuntime

describe('RuntimeOverview locale mapping', () => {
  it('renders localized unknown labels instead of raw dynamic keys or backend values', () => {
    const wrapper = mount(RuntimeOverview, {
      props: { runtime, loading: false, error: '' },
    })

    expect(wrapper.text()).toContain('Unknown')
    expect(wrapper.text()).toContain('DB Unknown · Redis Unknown')
    expect(wrapper.text()).not.toContain('future_process')
    expect(wrapper.text()).not.toContain('future_mode')
    expect(wrapper.text()).not.toContain('future_database')
    expect(wrapper.text()).not.toContain('admin.promptAudit.status.future_process')
  })
})
