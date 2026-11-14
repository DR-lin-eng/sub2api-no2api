import { beforeEach, describe, expect, it, vi } from 'vitest'

const actions = vi.hoisted(() => ({
  updateAdminApiKey: vi.fn(), createAdminApiKey: vi.fn(), rotateAdminApiKey: vi.fn(),
  revokeAdminApiKey: vi.fn(), deleteAdminApiKey: vi.fn(), regenerateAdminApiKey: vi.fn(),
}))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/core/stores/appStore', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }) }))
vi.mock('@/features/admin-settings/data/datasources/adminSettingsActions', () => actions)
vi.mock('@/features/admin-settings/data/datasources/adminSettingsQueries', () => ({
  getAdminApiKey: vi.fn().mockResolvedValue({ exists: true, masked_key: 'fixture' }),
  listAdminApiKeys: vi.fn().mockResolvedValue({ items: [] }),
}))

import { useSettingsAdminApiKeys } from '../presentation/composables/useSettingsAdminApiKeys'

describe('voucher permission on existing admin keys', () => {
  beforeEach(() => vi.clearAllMocks())

  it('adds the independent permission to the original legacy key without rotating its value', async () => {
    actions.updateAdminApiKey.mockResolvedValue({})
    const panel = useSettingsAdminApiKeys(vi.fn())
    panel.editScopedAdminApiKey({
      id: 'legacy', name: 'external software', key_prefix: 'fixture', last_four: '1234',
      scopes: ['admin.read'], status: 'active', created_by: 1,
      created_at: '2026-11-14T00:00:00Z', updated_at: '2026-11-14T00:00:00Z',
    })
    expect(panel.adminApiKeyScopeOptions.some(option => option.value === 'admin.redeem.secrets')).toBe(true)
    panel.adminApiKeyForm.scopes.push('admin.redeem.secrets')
    await panel.createScopedAdminApiKey()
    expect(actions.updateAdminApiKey).toHaveBeenCalledWith('legacy', {
      name: 'external software', scopes: ['admin.read', 'admin.redeem.secrets'], expires_at: null,
    })
    expect(actions.createAdminApiKey).not.toHaveBeenCalled()
    expect(actions.rotateAdminApiKey).not.toHaveBeenCalled()
    expect(actions.regenerateAdminApiKey).not.toHaveBeenCalled()
    expect(panel.adminApiKeyPanelSecret.value).toBe('')
  })
})
