import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import type { AdminUser } from '@/types'
import UserEditModal from '../UserEditModal.vue'

const { update, showError, showSuccess } = vi.hoisted(() => ({
  update: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: { update },
    userAttributes: { updateUserAttributeValues: vi.fn() }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() })
}))

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key })
}))

const user: AdminUser = {
  id: 42,
  email: 'tier-user@example.com',
  username: 'tier-user',
  notes: '',
  role: 'user',
  balance: 0,
  concurrency: 2,
  rpm_limit: 0,
  scheduling_tier: 2,
  status: 'active',
  allowed_groups: [],
  balance_notify_enabled: false,
  balance_notify_threshold: null,
  balance_notify_extra_emails: [],
  created_at: '2026-07-24T00:00:00Z',
  updated_at: '2026-07-24T00:00:00Z'
}

const mountModal = () => mount(UserEditModal, {
  props: { show: true, user },
  global: {
    stubs: {
      BaseDialog: {
        props: ['show', 'title'],
        template: '<div v-if="show"><slot /><slot name="footer" /></div>'
      },
      UserAttributeForm: true,
      Icon: true,
      TotpStepUpDialog: true
    }
  }
})

describe('UserEditModal', () => {
  beforeEach(() => {
    update.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    update.mockResolvedValue(user)
  })

  it('preserves priority tier zero in the update payload', async () => {
    const wrapper = mountModal()

    expect(wrapper.get('[data-test="scheduling-tier-select"]').element).toHaveProperty('value', '2')
    await wrapper.get('[data-test="scheduling-tier-select"]').setValue('0')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(update).toHaveBeenCalledWith(42, expect.objectContaining({ scheduling_tier: 0 }))
    expect(wrapper.emitted('success')).toHaveLength(1)
  })
})
