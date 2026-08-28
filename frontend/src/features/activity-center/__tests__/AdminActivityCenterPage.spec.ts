import { createPinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'

import AdminActivityCenterPage from '../presentation/pages/AdminActivityCenterPage.vue'

const { listMock, listRecordsMock, createMock, getAllGroupsMock } = vi.hoisted(() => ({
  listMock: vi.fn(),
  listRecordsMock: vi.fn(),
  createMock: vi.fn(),
  getAllGroupsMock: vi.fn(),
}))

vi.mock('@/features/activity-center/data/datasources/adminActivityCenterDatasource', () => ({
  default: {
    list: listMock,
    create: createMock,
    update: vi.fn(),
    delete: vi.fn(),
    listRecords: listRecordsMock,
  },
}))

vi.mock('@/features/admin-groups/data/datasources/adminGroupQueries', () => ({
  getAll: getAllGroupsMock,
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params && Object.keys(params).length > 0
          ? `${key}${JSON.stringify(params)}`
          : key,
    }),
  }
})

const SelectStub = {
  props: ['modelValue', 'options'],
  emits: ['update:modelValue', 'change'],
  template: `
    <select
      :value="modelValue"
      @change="$emit('update:modelValue', $event.target.value); $emit('change')"
    >
      <option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option>
    </select>
  `,
}

const GroupSelectorStub = {
  props: ['modelValue', 'groups'],
  emits: ['update:modelValue'],
  template: `
    <div data-test="group-selector">
      <button
        v-for="group in groups"
        :key="group.id"
        type="button"
        :data-test="'group-' + group.id"
        @click="$emit('update:modelValue', modelValue.includes(group.id) ? modelValue.filter((id) => id !== group.id) : [...modelValue, group.id])"
      >
        {{ group.name }}
      </button>
    </div>
  `,
}

describe('AdminActivityCenterPage', () => {
  beforeEach(() => {
    vi.useRealTimers()
    listMock.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    listRecordsMock.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
    getAllGroupsMock.mockResolvedValue([
      { id: 11, name: 'VIP', platform: 'openai', subscription_type: 'standard', rate_multiplier: 1, account_count: 0 },
      { id: 12, name: 'Pro', platform: 'anthropic', subscription_type: 'standard', rate_multiplier: 1, account_count: 0 },
    ])
    createMock.mockClear()
    createMock.mockResolvedValue({
      id: 1,
      title: 'Lucky Draw',
      subtitle: '',
      banner_url: '',
      banner_html: '',
      type: 'lottery',
      ref_id: '',
      config_json: '{}',
      status: 'draft',
      sort_order: 0,
      content: '',
      created_at: '2026-08-28T00:00:00Z',
      updated_at: '2026-08-28T00:00:00Z',
    })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  const mountPage = () => mount(AdminActivityCenterPage, {
    global: {
      plugins: [createPinia()],
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        TablePageLayout: { template: '<section><slot name="filters" /><slot name="table" /><slot name="pagination" /></section>' },
        DataTable: { template: '<div><slot name="empty" /></div>' },
        Pagination: true,
        BaseDialog: {
          props: ['show'],
          template: '<section v-if="show"><slot /><slot name="footer" /></section>',
        },
        ConfirmDialog: true,
        EmptyState: true,
        Select: SelectStub,
        GroupSelector: GroupSelectorStub,
        Icon: { template: '<i />' },
      },
    },
  })

  it('saves concrete lottery pool and prize config', async () => {
    const wrapper = mountPage()
    await flushPromises()

    const createButton = wrapper.findAll('button').find((button) => button.text().includes('createCampaign'))
    expect(createButton).toBeTruthy()
    await createButton!.trigger('click')

    await wrapper.find('form input[required]').setValue('Lucky Draw')
    await wrapper.findAll('select')[2].setValue('lottery')
    await flushPromises()
    await wrapper.get('[data-test="group-11"]').trigger('click')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(createMock).toHaveBeenCalledTimes(1)
    const payload = createMock.mock.calls[0][0]
    expect(payload.type).toBe('lottery')
    expect(payload.ref_id).toBe('')

    const config = JSON.parse(payload.config_json)
    expect(config.lottery.pools).toHaveLength(1)
    expect(config.lottery.pools[0].name).toBe('admin.activityCenter.config.defaultPool')
    expect(config.lottery.pools[0].required_group_ids).toEqual([11])
    expect(config.lottery.pools[0].prizes[0]).toMatchObject({
      label: 'admin.activityCenter.config.defaultPrize',
      prize_type: 'none',
      weight: 100,
      is_fallback: true,
    })
  })

  it('defaults end time to selected start day 23:59 only after choosing start time', async () => {
    const wrapper = mountPage()
    await flushPromises()

    const createButton = wrapper.findAll('button').find((button) => button.text().includes('createCampaign'))
    expect(createButton).toBeTruthy()
    await createButton!.trigger('click')

    const dateInputs = wrapper.findAll('input[type="datetime-local"]')
    expect(dateInputs).toHaveLength(2)
    expect((dateInputs[1].element as HTMLInputElement).value).toBe('')

    await dateInputs[0].setValue('2026-08-28T09:30')
    await dateInputs[0].trigger('change')

    expect((dateInputs[1].element as HTMLInputElement).value).toBe('2026-08-28T23:59')
  })

  it('allows removing the default no-prize option and saves an empty prize list', async () => {
    const wrapper = mountPage()
    await flushPromises()

    const createButton = wrapper.findAll('button').find((button) => button.text().includes('createCampaign'))
    expect(createButton).toBeTruthy()
    await createButton!.trigger('click')

    await wrapper.find('form input[required]').setValue('Lucky Draw')
    await wrapper.findAll('select')[2].setValue('lottery')
    await wrapper.get('[data-test="remove-prize"]').trigger('click')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    const payload = createMock.mock.calls[0][0]
    const config = JSON.parse(payload.config_json)
    expect(config.lottery.pools[0].prizes).toEqual([])
  })

  it('collapses and expands lottery pool details', async () => {
    const wrapper = mountPage()
    await flushPromises()

    const createButton = wrapper.findAll('button').find((button) => button.text().includes('createCampaign'))
    expect(createButton).toBeTruthy()
    await createButton!.trigger('click')

    await wrapper.findAll('select')[2].setValue('lottery')
    expect(wrapper.text()).toContain('admin.activityCenter.config.poolTier')

    await wrapper.get('[data-test="pool-toggle-0"]').trigger('click')
    expect(wrapper.text()).not.toContain('admin.activityCenter.config.poolTier')

    await wrapper.get('[data-test="pool-toggle-0"]').trigger('click')
    expect(wrapper.text()).toContain('admin.activityCenter.config.poolTier')
  })

  it('does not save when end time is earlier than current time', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-28T13:30:00+08:00'))

    const wrapper = mountPage()
    await flushPromises()

    const createButton = wrapper.findAll('button').find((button) => button.text().includes('createCampaign'))
    expect(createButton).toBeTruthy()
    await createButton!.trigger('click')

    await wrapper.find('form input[required]').setValue('Lucky Draw')
    const dateInputs = wrapper.findAll('input[type="datetime-local"]')
    await dateInputs[1].setValue('2026-08-28T13:29')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(createMock).not.toHaveBeenCalled()
  })
})
