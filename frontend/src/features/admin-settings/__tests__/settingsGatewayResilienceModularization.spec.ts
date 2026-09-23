import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const currentDir = dirname(fileURLToPath(import.meta.url))
const tabsDir = resolve(currentDir, '../presentation/widgets/settings-tabs')
const readTabSource = (relativePath: string) =>
  readFileSync(resolve(tabsDir, relativePath), 'utf8')
const normalizedTemplateHash = (source: string) =>
  createHash('sha256')
    .update(
      source
        .split('\n')
        .map((line) => line.trim())
        .filter(Boolean)
        .join('\n'),
    )
    .digest('hex')

const cards = [
  {
    marker: 'Codex OAuth A/B Simulation Settings',
    component: 'SettingsCodexSimulationCard',
    file: 'SettingsCodexSimulationCard.vue',
		templateHash: 'c78190ebe0430d38d939afcd187759beb1e88dd0fda341eb303e65facf4fdb3e',
  },
  {
    marker: 'Global Temporary Unschedulable Settings',
    component: 'SettingsGlobalTempUnschedulableCard',
    file: 'SettingsGlobalTempUnschedulableCard.vue',
    templateHash: 'b6b31579717d9e5f8909f9d6b090856c6a8d80f4bd630827e8801467348f6c6d',
  },
  {
    marker: 'Overload Cooldown (529) Settings',
    component: 'SettingsOverloadCooldownCard',
    file: 'SettingsOverloadCooldownCard.vue',
    templateHash: 'aa0eb82d2b4a3e6e4237391f75783ac1fa04f7718ceadd8fd2e5c5264c203b1c',
  },
  {
    marker: 'Rate Limit Cooldown (429) Settings',
    component: 'SettingsRateLimit429CooldownCard',
    file: 'SettingsRateLimit429CooldownCard.vue',
    templateHash: '2faf2ab279ed890750f3a32480586ac73ab9799aa8e61168485721d5402d9381',
  },
  {
    marker: 'OAuth 401 Cleanup Settings',
    component: 'SettingsOAuth401CleanupCard',
    file: 'SettingsOAuth401CleanupCard.vue',
    templateHash: '5ffe4ea1d4cb5e8fb4425c107f93ddbd840db04fee6981df45867e7f2a36cdad',
  },
  {
    marker: 'Stream Timeout Settings',
    component: 'SettingsStreamTimeoutCard',
    file: 'SettingsStreamTimeoutCard.vue',
    templateHash: 'e4de6fc997711a2170ea9b8a14fb1e4f69cb5118abddb6ecdd6dab1f7717441e',
  },
  {
    marker: 'Request Rectifier Settings',
    component: 'SettingsRequestRectifierCard',
    file: 'SettingsRequestRectifierCard.vue',
    templateHash: '4c3e1cd8c109f3c85ab56a49112977f663ef4a93c5b4b2f55153166f2841046a',
  },
  {
    marker: 'Beta Policy Settings',
    component: 'SettingsBetaPolicyCard',
    file: 'SettingsBetaPolicyCard.vue',
    templateHash: '6d68ac1342ece2af6c3c1df266006dd558d09e5d41f455ab7b4199aba132a13d',
  },
  {
    marker: 'OpenAI Fast/Flex Policy Settings',
    component: 'SettingsOpenAIFastPolicyCard',
    file: 'SettingsOpenAIFastPolicyCard.vue',
    templateHash: '4b3de515335da6164d6ebd6859604ea3d70d715cfc5e6811ed53183c07af6544',
  },
] as const

const panelSource = readTabSource('SettingsGatewayResiliencePanel.vue')

function templateBody(source: string) {
  return source.slice('<template>\n'.length, source.indexOf('\n</template>'))
}

function effectiveLineCount(source: string) {
  return source
    .split('\n')
    .filter((line) => line.trim() && !line.trim().startsWith('//')).length
}

describe('settings gateway resilience modularization', () => {
  it('keeps the parent as a static gateway resilience card composition', () => {
    let previousPosition = -1
    for (const card of cards) {
      expect(panelSource).toContain(
        `import ${card.component} from './gateway-resilience/${card.file}'`,
      )
      expect(panelSource).toContain(`<!-- ${card.marker} -->`)
      const position = panelSource.indexOf(`<${card.component} />`)
      expect(position).toBeGreaterThan(previousPosition)
      previousPosition = position
    }

    expect(panelSource).not.toContain('useSettingsPageContext')
    expect(panelSource).not.toContain('v-model')
    expect(panelSource).not.toContain('import(')
  })

  it('keeps each original card template on the shared page context', () => {
    for (const card of cards) {
      const source = readTabSource(`gateway-resilience/${card.file}`)
      const template = templateBody(source)

      expect(source.match(/useSettingsPageContext\(\)/g)).toHaveLength(1)
      expect(template.trim()).toMatch(/^<div class="card">/)
      expect(normalizedTemplateHash(template)).toBe(card.templateHash)
      expect(source).not.toMatch(/\b(?:ref|reactive|computed|watch)\s*\(/)
      expect(source).not.toMatch(
        /\b(?:onMounted|onBeforeMount|onUnmounted)\s*\(|defineProps|defineEmits|adminSettingsAPI|data\/datasources|@\/api|\bfetch\s*\(|\baxios\b/,
      )
      expect(source).not.toContain('import(')
      expect(effectiveLineCount(source)).toBeLessThan(1500)
    }
  })
})
