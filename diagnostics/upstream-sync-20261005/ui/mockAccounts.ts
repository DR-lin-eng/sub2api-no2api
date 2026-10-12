export async function updateAccount(id: number, changes: { priority: number }) {
  return { id, name: '隔离测试账号', platform: 'anthropic', type: 'oauth', ...changes }
}
