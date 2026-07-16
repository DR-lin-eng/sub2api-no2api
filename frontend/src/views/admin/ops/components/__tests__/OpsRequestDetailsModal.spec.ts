import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsRequestDetailsModal from '../OpsRequestDetailsModal.vue'

const mockListRequestDetails = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    listRequestDetails: (...args: any[]) => mockListRequestDetails(...args)
  }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showWarning: vi.fn()
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() })
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const BaseDialogStub = defineComponent({
  props: {
    show: { type: Boolean, default: false },
    title: { type: String, default: '' }
  },
  template: '<div v-if="show"><slot /></div>'
})

const PaginationStub = defineComponent({
  template: '<div class="pagination-stub" />'
})

describe('OpsRequestDetailsModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockListRequestDetails.mockResolvedValue({
      items: [
        {
          kind: 'success',
          created_at: '2026-07-17T00:30:00Z',
          request_id: 'req-ttft',
          platform: 'openai',
          model: 'gpt-5',
          duration_ms: 1200,
          first_token_ms: 450,
          stream: true
        }
      ],
      total: 1,
      page: 1,
      page_size: 10
    })
  })

  it('TTFT 明细仅查询有首 Token 的成功请求并显示 TTFT 列', async () => {
    const wrapper = mount(OpsRequestDetailsModal, {
      props: {
        modelValue: false,
        timeRange: '1h',
        preset: {
          title: 'TTFT',
          kind: 'success',
          sort: 'ttft_desc',
          ttft_only: true
        }
      },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Pagination: PaginationStub
        }
      }
    })

    await wrapper.setProps({ modelValue: true })
    await flushPromises()

    expect(mockListRequestDetails).toHaveBeenCalledWith(
      expect.objectContaining({
        kind: 'success',
        sort: 'ttft_desc',
        ttft_only: true,
        page: 1,
        page_size: 10
      })
    )
    expect(wrapper.text()).toContain('admin.ops.requestDetails.table.ttft')
    expect(wrapper.text()).not.toContain('admin.ops.requestDetails.table.duration')
    expect(wrapper.text()).toContain('450 ms')
  })
})
