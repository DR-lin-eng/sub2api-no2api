import { createRequire } from 'node:module'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { Linter } from 'eslint'

const require = createRequire(import.meta.url)
const architecturePlugin = require('../../eslint/architecture-boundaries.cjs')
const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), '../..')

const verify = (code: string, relativeFile: string) => {
  const linter = new Linter({ configType: 'flat' })
  return linter.verify(
    code,
    {
      files: ['**/*.{ts,vue}'],
      languageOptions: {
        ecmaVersion: 'latest',
        sourceType: 'module',
      },
      plugins: {
        architecture: architecturePlugin,
      },
      rules: {
        'architecture/no-new-debt': 'error',
      },
    },
    resolve(frontendRoot, relativeFile),
  )
}

describe('frontend architecture boundaries', () => {
  it('blocks new transitional barrel imports', () => {
    const messages = verify(
      "import { userAPI } from '@/api'",
      'src/features/keys/presentation/newLegacyImport.ts',
    )

    expect(messages).toEqual([
      expect.objectContaining({
        messageId: 'legacy',
        ruleId: 'architecture/no-new-debt',
      }),
    ])
  })

  it('blocks new imports from another feature private presentation layer', () => {
    const messages = verify(
      "import TotpLoginDialog from '@/features/auth/presentation/widgets/TotpLoginDialog.vue'",
      'src/features/keys/presentation/newCrossFeatureImport.ts',
    )

    expect(messages).toEqual([
      expect.objectContaining({
        messageId: 'crossFeature',
        ruleId: 'architecture/no-new-debt',
      }),
    ])
  })

  it('allows imports from another feature stable owner contract', () => {
    const messages = verify(
      "import { useAuthStore } from '@/features/auth'",
      'src/features/channels-user/presentation/newOwnerContractImport.ts',
    )

    expect(messages).toEqual([])
  })

  it('blocks relative imports that reverse data and presentation dependencies', () => {
    const messages = verify(
      "import KeyDialog from '../presentation/widgets/KeyDialog.vue'",
      'src/features/keys/data/newReverseLayerImport.ts',
    )

    expect(messages).toEqual([
      expect.objectContaining({
        messageId: 'reverseLayer',
        ruleId: 'architecture/no-new-debt',
      }),
    ])
  })

  it('allows presentation to import its owning feature datasource', () => {
    const messages = verify(
      "import { keysAPI } from '@/features/keys/data/datasources/keysDatasource'",
      'src/features/keys/presentation/newOwnedImport.ts',
    )

    expect(messages).toEqual([])
  })

  it('requires a migrated import to be removed from the exact baseline', () => {
    const messages = verify('export {}', 'src/App.vue')

    expect(messages).toEqual([
      expect.objectContaining({
        messageId: 'staleBaseline',
        ruleId: 'architecture/no-new-debt',
      }),
    ])
  })
})
