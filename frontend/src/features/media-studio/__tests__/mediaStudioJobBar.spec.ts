import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import MediaStudioJobBar from '../presentation/widgets/MediaStudioJobBar.vue'
import type { MediaStudioJobView } from '../presentation/composables/useMediaStudioJobs'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params && 'count' in params ? `${key}:${String(params.count)}` : key,
  }),
}))
enableAutoUnmount(afterEach)

function job(overrides: Partial<MediaStudioJobView> = {}): MediaStudioJobView {
  return {
    messageId: 'message-1',
    mode: 'image',
    prompt: 'a quiet cat',
    status: 'processing',
    taskIds: ['task-1'],
    createdAt: Date.now() - 2_000,
    ...overrides,
  }
}

function clickButton(wrapper: ReturnType<typeof mount>, label: string) {
  const button = wrapper.findAll('button').find(item => item.text() === label)
  expect(button).toBeTruthy()
  return button!.trigger('click')
}

describe('media studio job bar', () => {
  it('renders one row per job and only shows the five most recent', () => {
    const jobs = Array.from({ length: 7 }, (_, index) => job({
      messageId: `message-${index}`,
      status: index === 0 ? 'processing' : 'failed',
    }))
    const wrapper = mount(MediaStudioJobBar, { props: { jobs } })

    expect(wrapper.findAll('.job-row')).toHaveLength(5)
    expect(wrapper.text()).toContain('mediaStudio.jobs.activeCount:1')
  })

  it('offers retry only for failed jobs and never renders a cancel action', async () => {
    const wrapper = mount(MediaStudioJobBar, {
      props: { jobs: [job({ status: 'processing' }), job({ messageId: 'message-2', status: 'failed' })] },
    })

    const labels = wrapper.findAll('button').map(button => button.text())
    expect(labels).not.toContain('mediaStudio.jobs.cancel')
    expect(labels.filter(label => label === 'mediaStudio.jobs.retry')).toHaveLength(1)

    await clickButton(wrapper, 'mediaStudio.jobs.retry')
    expect(wrapper.emitted('retry')).toEqual([['message-2']])
  })

  it('renders the classified error text for failed jobs', () => {
    const wrapper = mount(MediaStudioJobBar, {
      props: { jobs: [job({ status: 'failed', error: 'upstream boom', errorCode: 'timeout' })] },
    })

    expect(wrapper.text()).toContain('mediaStudio.jobs.errorCodes.timeout')
  })

  it('renders the localized network message instead of the browser error text', () => {
    const wrapper = mount(MediaStudioJobBar, {
      props: { jobs: [job({ status: 'failed', error: 'Failed to fetch', errorCode: 'network' })] },
    })

    expect(wrapper.text()).toContain('mediaStudio.jobs.errorCodes.network')
    expect(wrapper.text()).not.toContain('Failed to fetch')
  })

  it('forwards focus requests with the message id', async () => {
    const wrapper = mount(MediaStudioJobBar, { props: { jobs: [job({ status: 'completed' })] } })

    await clickButton(wrapper, 'mediaStudio.jobs.focus')
    expect(wrapper.emitted('focus')).toEqual([['message-1']])
  })
})
