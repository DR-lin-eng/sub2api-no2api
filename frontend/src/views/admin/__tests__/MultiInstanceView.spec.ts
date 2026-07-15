import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MultiInstanceView from '../MultiInstanceView.vue'

const { getStatus } = vi.hoisted(() => ({ getStatus: vi.fn() }))

vi.mock('@/api/admin', () => ({
  adminAPI: { cluster: { getStatus } },
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key }),
  }
})

vi.mock('@/utils/format', () => ({
  formatDateTime: (value: string) => value,
  formatRelativeTime: () => 'now',
}))

vi.mock('@/utils/apiError', () => ({
  extractApiErrorMessage: (_error: unknown, fallback: string) => fallback,
}))

function statusFixture() {
  return {
    deployment: {
      mode: 'multi_instance',
      node_name: 'api-a',
      runner_id: 'api-a-runner',
      worker_mode: 'auto',
      worker_enabled: true,
      frontend_enabled: true,
      heartbeat_interval_seconds: 30,
      stale_after_seconds: 90,
      task_lease_seconds: 60,
    },
    summary: { online_nodes: 2, stale_nodes: 0, stopped_nodes: 0, worker_nodes: 2, active_tasks: 1, unhealthy_nodes: 0 },
    instances: [{
      runner_id: 'api-a-runner',
      node_name: 'api-a',
      deployment_mode: 'multi_instance',
      worker_mode: 'auto',
      worker_enabled: true,
      version: '1.2.3',
      hostname: 'host-a',
      process_id: 10,
      database_ok: true,
      redis_ok: true,
      started_at: '2026-07-15T00:00:00Z',
      last_seen_at: '2026-07-15T00:01:00Z',
      status: 'online',
      current: true,
    }],
    tasks: [{
      id: 1,
      run_id: 'run-1',
      task_key: 'backup:scheduled',
      status: 'running',
      node_name: 'api-a',
      runner_id: 'api-a-runner',
      metadata: {},
      result: {},
      error_message: '',
      started_at: '2026-07-15T00:00:00Z',
      heartbeat_at: '2026-07-15T00:01:00Z',
      lease_until: '2026-07-15T00:02:00Z',
    }],
    observed_at: '2026-07-15T00:01:00Z',
  }
}

describe('MultiInstanceView', () => {
  beforeEach(() => {
    getStatus.mockReset()
    getStatus.mockResolvedValue(statusFixture())
  })

  it('renders node health, resolved worker mode, and active task lease', async () => {
    const wrapper = mount(MultiInstanceView, {
      global: {
        mocks: { $t: (key: string) => key },
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: { template: '<span />' },
          Toggle: { props: ['modelValue'], template: '<button type="button" />' },
        },
      },
    })

    await flushPromises()

    expect(getStatus).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('api-a')
    expect(wrapper.text()).toContain('backup:scheduled')
    expect(wrapper.text()).toContain('1.2.3')
  })
})
