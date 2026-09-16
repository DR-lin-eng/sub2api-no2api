import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const source = [
  'src/features/admin-accounts/presentation/widgets/CreateAccountDialog.vue',
  'src/features/admin-accounts/presentation/widgets/create/CreateAccountPlatformFields.vue',
  'src/features/admin-accounts/presentation/widgets/create/CreateAccountCredentialFields.vue',
  'src/features/admin-accounts/presentation/composables/useCreateAccountEditorPolicy.ts',
  'src/features/admin-accounts/presentation/composables/useCreateAccountOAuthActions.ts',
  'src/core/constants/account.ts'
].map((path) => readFileSync(resolve(process.cwd(), path), 'utf8')).join('\n')

describe('CreateAccountModal Grok account types', () => {
  it('offers API-key setup alongside OAuth with the official xAI default', () => {
    expect(source).toContain('data-testid="grok-account-type-api-key"')
    expect(source).toContain("@click=\"accountCategory = 'apikey'\"")
    expect(source).toContain("newPlatform === 'grok'")
    expect(source).toContain("case 'grok': return 'https://api.x.ai/v1'")
    expect(source).toContain("form.platform === 'grok'")
    expect(source).toContain("case 'grok': return 'xai-...'")
  })

  it('exposes custom upstream URL and header override for the OAuth create flow', () => {
    expect(source).toContain('data-testid="grok-custom-base-url-toggle"')
    expect(source).toContain('data-testid="grok-custom-base-url-input"')
    expect(source).toContain('form.platform === \'grok\' && isOAuthFlow')
  })

  it('validates and applies upstream config on all three Grok OAuth create paths', () => {
    // 授权码兑换 / RT 批量 / SSO 批量 3 处调用（定义为箭头函数，不计入）
    expect(source.match(/validateGrokOAuthUpstreamConfig\(\)/g)?.length).toBe(3)
    expect(source.match(/applyGrokOAuthUpstreamConfig\(credentials\)/g)?.length).toBe(3)
  })
})
