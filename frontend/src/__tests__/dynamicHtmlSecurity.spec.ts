import { readFileSync, readdirSync } from 'node:fs'
import { dirname, extname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const currentDir = dirname(fileURLToPath(import.meta.url))
const srcDir = resolve(currentDir, '..')

function collectRuntimeSources(directory: string): Array<{ path: string; source: string }> {
  const sources: Array<{ path: string; source: string }> = []
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    if (entry.name === '__tests__') continue
    const absolutePath = join(directory, entry.name)
    if (entry.isDirectory()) {
      sources.push(...collectRuntimeSources(absolutePath))
      continue
    }
    if (!new Set(['.ts', '.vue']).has(extname(entry.name))) continue
    sources.push({
      path: relative(srcDir, absolutePath),
      source: readFileSync(absolutePath, 'utf8'),
    })
  }
  return sources
}

const runtimeSources = collectRuntimeSources(srcDir)
const sourceByPath = new Map(runtimeSources.map(({ path, source }) => [path, source]))

const expectedVHtmlSinks = [
  'common/pages/HomePage.vue:sanitizedHomeContent',
  'common/pages/LegalDocumentPage.vue:renderedHtml',
  'common/widgets/data/AnnouncementBell.vue:renderMarkdown(selectedAnnouncement.content)',
  'common/widgets/data/AnnouncementPopup.vue:renderedContent',
  'common/widgets/data/ImageUpload.vue:sanitizedValue',
  'common/widgets/layout/AppSidebar.vue:sanitizeSvg(item.iconSvg)',
  'common/widgets/layout/AppSidebar.vue:sanitizeSvg(item.iconSvg)',
  'common/widgets/layout/AppSidebar.vue:sanitizeSvg(item.iconSvg)',
  'features/admin-settings/presentation/widgets/AdminComplianceDialog.vue:renderedDocument',
  'features/activity-center/presentation/pages/ActivityCenterPage.vue:safeBanners[campaign.id]',
  'features/channels-user/presentation/pages/CustomLandingPage.vue:renderedHtml',
  'features/keys/presentation/pages/KeyUsagePage.vue:row.iconSvg',
  'features/keys/presentation/widgets/UseKeyDialog.vue:file.highlighted',
  'features/model-plaza/presentation/widgets/ModelPlazaContent.vue:descriptionHtml',
].sort()

const requiredSafetySignals: Record<string, string> = {
  'common/pages/HomePage.vue': 'sanitizeHomeContentHtml',
  'common/pages/LegalDocumentPage.vue': 'DOMPurify.sanitize',
  'common/widgets/data/AnnouncementBell.vue': 'DOMPurify.sanitize',
  'common/widgets/data/AnnouncementPopup.vue': 'DOMPurify.sanitize',
  'common/widgets/data/ImageUpload.vue': 'sanitizeSvg',
  'common/widgets/layout/AppSidebar.vue': 'sanitizeSvg',
  'features/admin-settings/presentation/widgets/AdminComplianceDialog.vue': 'DOMPurify.sanitize',
  'features/activity-center/presentation/pages/ActivityCenterPage.vue': 'sanitizeActivityBannerHtml',
  'features/activity-center/presentation/activityHtml.ts': 'DOMPurify.sanitize',
  'features/channels-user/presentation/pages/CustomLandingPage.vue': 'sanitizeCustomPageHtml',
  'features/keys/presentation/pages/KeyUsagePage.vue': 'iconSvg: ICON_',
  'features/keys/presentation/widgets/UseKeyDialog.vue': 'const escapeHtml',
  'features/model-plaza/presentation/widgets/ModelPlazaContent.vue': 'DOMPurify.sanitize',
}

describe('dynamic HTML security boundary', () => {
  it('allows only the reviewed v-html sinks', () => {
    const actual = runtimeSources.flatMap(({ path, source }) =>
      Array.from(source.matchAll(/v-html\s*=\s*"([^"]+)"/g), (match) => `${path}:${match[1]}`),
    ).sort()

    expect(actual).toEqual(expectedVHtmlSinks)
  })

  it('keeps every approved sink connected to its sanitizer or constant-only owner', () => {
    const findings = Object.entries(requiredSafetySignals).flatMap(([path, signal]) => {
      const source = sourceByPath.get(path) || ''
      return source.includes(signal) ? [] : [`${path}: missing ${signal}`]
    })

    expect(findings).toEqual([])
  })

  it('permits direct innerHTML assignment only for clearing an element', () => {
    const findings = runtimeSources.flatMap(({ path, source }) =>
      Array.from(source.matchAll(/\.innerHTML\s*=\s*([^;\n]+)/g), (match) => ({
        path,
        value: match[1].trim(),
      })),
    ).filter(({ value }) => value !== "''" && value !== '""')

    expect(findings).toEqual([])
  })

  it('keeps legacy HTML-writing APIs out of runtime source', () => {
    const findings = runtimeSources.flatMap(({ path, source }) => [
      ...(source.includes('insertAdjacentHTML(') ? [`${path}: insertAdjacentHTML`] : []),
      ...(source.includes('document.write(') ? [`${path}: document.write`] : []),
      ...(source.includes('document.writeln(') ? [`${path}: document.writeln`] : []),
    ])

    expect(findings).toEqual([])
  })
})
