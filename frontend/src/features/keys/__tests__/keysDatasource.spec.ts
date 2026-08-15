import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))

vi.mock('@/core/networks/client', () => ({
  apiClient: {
    post,
  },
}))

import { create } from '../data/datasources/keysDatasource'

describe('keys datasource', () => {
  beforeEach(() => {
    post.mockReset()
    post.mockResolvedValue({ data: {} })
  })

  it('sends ordered group bindings together with the legacy primary group', async () => {
    const groupBindings = [
      { group_id: 7, max_rate_multiplier: 1.2 },
      { group_id: 8, max_rate_multiplier: null },
    ]

    await create('routing key', 7, undefined, [], [], 0, undefined, undefined, groupBindings)

    expect(post).toHaveBeenCalledWith('/keys', {
      name: 'routing key',
      group_id: 7,
      group_bindings: groupBindings,
    })
  })
})
