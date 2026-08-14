import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MultiInstanceView from '@/features/admin-cluster/presentation/pages/MultiInstancePage.vue'

const clusterAPI = vi.hoisted(() => ({
  getStatus: vi.fn(),
  renameNode: vi.fn(),
  createRollout: vi.fn(),
  pauseRollout: vi.fn(),
  resumeRollout: vi.fn(),
  cancelRollout: vi.fn(),
  retryRolloutTarget: vi.fn(),
}))

vi.mock('@/features/admin-cluster/data/datasources/adminClusterDatasource', () => ({
  default: clusterAPI,
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key }),
  }
})

vi.mock('@/core/utils/format', () => ({
  formatDateTime: (value: string) => value,
  formatRelativeTime: () => 'now',
}))

vi.mock('@/core/utils/apiError', () => ({
  extractApiErrorMessage: (_error: unknown, fallback: string) => fallback,
}))

function statusFixture() {
  return {
    deployment: {
      mode: 'multi_instance',
      node_id: 'node-a',
      node_name: 'api-a',
      runner_id: 'api-a-runner',
      worker_mode: 'auto',
      worker_enabled: true,
      frontend_enabled: true,
      heartbeat_interval_seconds: 30,
      stale_after_seconds: 90,
      task_lease_seconds: 60,
      update_driver: 'binary',
      rollout_poll_seconds: 5,
      rollout_drain_grace_seconds: 10,
      rollout_drain_timeout_seconds: 900,
      rollout_verify_heartbeats: 2,
    },
    summary: { online_nodes: 2, stale_nodes: 0, stopped_nodes: 0, worker_nodes: 2, active_tasks: 1, unhealthy_nodes: 0 },
    instances: [{
      node_id: 'node-a',
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
    release: {
      state: {
        desired_version: '1.2.3',
        generation: 1,
        updated_at: '2026-07-15T00:01:00Z',
      },
      recent_rollouts: [],
      version_counts: [{ version: '1.2.3', nodes: 1 }],
      consistent: true,
    },
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
    Object.values(clusterAPI).forEach((mock) => mock.mockReset())
    clusterAPI.getStatus.mockResolvedValue(statusFixture())
    clusterAPI.renameNode.mockResolvedValue(undefined)
    clusterAPI.createRollout.mockResolvedValue({})
    vi.spyOn(window, 'confirm').mockReturnValue(true)
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

    expect(clusterAPI.getStatus).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('api-a')
    expect(wrapper.text()).toContain('backup:scheduled')
    expect(wrapper.text()).toContain('1.2.3')
  })

  it('shows node-managed updates when the binary rollout driver is active', async () => {
    clusterAPI.getStatus.mockResolvedValue(statusFixture())
    const wrapper = mount(MultiInstanceView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: { template: '<span />' },
          Toggle: { props: ['modelValue'], template: '<button type="button" />' },
        },
      },
    })

    await flushPromises()

    expect(wrapper.text()).toContain('admin.cluster.release.binaryDriver')
    expect(wrapper.text()).not.toContain('admin.cluster.release.externalDriver')
  })

  it('renames a logical node without changing its stable identity', async () => {
    const wrapper = mount(MultiInstanceView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: { template: '<span />' },
          Toggle: { props: ['modelValue'], template: '<button type="button" />' },
        },
      },
    })
    await flushPromises()

    await wrapper.get('button[title="admin.cluster.nodes.rename"]').trigger('click')
    await wrapper.get('input[maxlength="128"]').setValue('edge-primary')
    await wrapper.get('button[title="admin.cluster.nodes.saveName"]').trigger('click')
    await flushPromises()

    expect(clusterAPI.renameNode).toHaveBeenCalledWith('node-a', 'edge-primary')
    expect(clusterAPI.getStatus).toHaveBeenCalledTimes(2)
  })

  it('creates a rollout task from any cluster page replica', async () => {
    const wrapper = mount(MultiInstanceView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: { template: '<span />' },
          Toggle: { props: ['modelValue'], template: '<button type="button" />' },
        },
      },
    })
    await flushPromises()

    await wrapper.get('input[placeholder="admin.cluster.release.latestPlaceholder"]').setValue('1.2.4')
    const startButton = wrapper.findAll('button').find((button) => button.text().includes('admin.cluster.release.start'))
    expect(startButton).toBeDefined()
    await startButton!.trigger('click')
    await flushPromises()

    expect(window.confirm).toHaveBeenCalled()
    expect(clusterAPI.createRollout).toHaveBeenCalledWith('1.2.4')
  })
})
