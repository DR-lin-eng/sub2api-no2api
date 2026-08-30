import { afterEach, describe, expect, it, vi } from 'vitest'
import apiClient from '@/core/networks/client'
import { issueEmbeddedCapability } from '@/features/channels-user/data/datasources/embeddedCapabilityDatasource'

describe('embedded capability datasource', () => {
  afterEach(() => vi.restoreAllMocks())

  it('requests a menu-scoped capability without placing a login token in the payload', async () => {
    const post = vi.spyOn(apiClient, 'post').mockResolvedValue({
      data: {
        token: 'permission-proof',
        token_type: 'embedded_capability',
        expires_at: '2026-08-30T12:01:30Z',
        menu_id: 'help',
        target_origin: 'https://help.example.test',
      },
    })

    const result = await issueEmbeddedCapability('help', 'https://help.example.test')

    expect(post).toHaveBeenCalledWith('/auth/embedded-capability', {
      menu_id: 'help',
      target_origin: 'https://help.example.test',
    })
    expect(post.mock.calls[0]?.[1]).not.toHaveProperty('token')
    expect(result.token_type).toBe('embedded_capability')
  })
})
