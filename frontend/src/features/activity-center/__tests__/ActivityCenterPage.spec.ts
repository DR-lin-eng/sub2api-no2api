import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import ActivityCenterPage from '../presentation/pages/ActivityCenterPage.vue'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

describe('ActivityCenterPage', () => {
  const mountPage = () => mount(ActivityCenterPage, {
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        Icon: true,
      },
    },
  })

  it('renders the activity center shell with filters and rule panels', async () => {
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.text()).toContain('activityCenter.title')
    expect(wrapper.text()).toContain('activityCenter.rules.title')
    expect(wrapper.findAll('article')).toHaveLength(4)
    expect(wrapper.findAll('button').some((button) => button.text() === 'activityCenter.actions.details')).toBe(true)
  })

  it('filters activities and updates the selected detail panel', async () => {
    const wrapper = mountPage()
    await flushPromises()

    await wrapper.get('input[type="search"]').setValue('referral')
    await flushPromises()

    expect(wrapper.findAll('article')).toHaveLength(1)
    expect(wrapper.text()).toContain('activityCenter.activities.referralSprint.title')

    await wrapper.get('button.btn-secondary.btn-sm').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('activityCenter.selected.title')
    expect(wrapper.text()).toContain('activityCenter.activities.referralSprint.summary')
  })
})
