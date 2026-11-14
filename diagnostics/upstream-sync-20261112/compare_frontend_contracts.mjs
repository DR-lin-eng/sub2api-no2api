import fs from 'node:fs'
import crypto from 'node:crypto'
import { createRequire } from 'node:module'

const require = createRequire('/work/frontend/package.json')
const ts = require('typescript')
const hash = (text) => crypto.createHash('sha256').update(text).digest('hex')
const transpile = (text) => ts.transpileModule(text, {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext }
}).outputText
const dataUrl = (text) => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`
const platforms = ['anthropic', 'openai', 'gemini', 'antigravity-claude', 'antigravity-gemini', 'grok']
const result = {}

for (const [stage, root] of [['baseline', '/baseline'], ['candidate', '/candidate']]) {
  const dir = `${root}/frontend/src/features/keys/presentation/resolvers`
  const catalog = dataUrl(transpile(fs.readFileSync(`${dir}/openCodeModelCatalogs.ts`, 'utf8')))
  const resolverSource = fs.readFileSync(`${dir}/openCodeConfigResolver.ts`, 'utf8')
    .replace("'@/features/keys/presentation/resolvers/openCodeModelCatalogs'", JSON.stringify(catalog))
  const { generateOpenCodeConfig } = await import(dataUrl(transpile(resolverSource)))
  result[stage] = {}
  for (const platform of platforms) {
    const { content } = generateOpenCodeConfig({ platform,
      baseUrl: 'https://example.com/base', apiKey: 'sk-byte-test',
      pathLabel: `${platform}.json`, hint: 'keys.useKeyModal.opencode.hint' })
    const provider = JSON.parse(content).provider[platform]
    result[stage][platform] = { bytes: Buffer.byteLength(content), sha256: hash(content),
      providerKeys: Object.keys(provider), modelIds: Object.keys(provider.models ?? {}) }
  }
  const card = fs.readFileSync(`${root}/frontend/src/features/admin-settings/presentation/widgets/settings-tabs/gateway-resilience/SettingsCodexSimulationCard.vue`, 'utf8')
  const template = card.slice('<template>\n'.length, card.indexOf('\n</template>'))
  result[stage].codexCard = { source_sha256: hash(card), template_sha256:
    hash(template.split('\n').map(line => line.trim()).filter(Boolean).join('\n')) }
}
console.log(JSON.stringify(result, null, 2))
