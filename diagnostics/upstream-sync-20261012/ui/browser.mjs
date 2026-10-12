import { createRequire } from 'node:module'
import { mkdirSync, writeFileSync } from 'node:fs'
const { chromium } = createRequire('/opt/qa/package.json')('playwright-core')
const phase = process.argv[2]
const modified = phase === 'MODIFIED'
const browser = await chromium.launch({ executablePath: '/usr/bin/chromium-browser', headless: true, args: ['--no-sandbox', '--disable-dev-shm-usage'] })
const page = await browser.newPage()
const errors = []
page.on('pageerror', e => errors.push(String(e)))
const records = []
for (let attempt = 0; attempt < 100; attempt++) {
  try { if ((await fetch('http://127.0.0.1:4173')).ok) break } catch {}
  if (attempt === 99) throw new Error('harness never ready')
  await new Promise(resolve => setTimeout(resolve, 300))
}
try {
  for (const width of [320, 375, 1440]) for (const locale of ['en', 'zh']) {
    await page.setViewportSize({ width, height: 800 })
    await page.goto(`http://127.0.0.1:4173/?locale=${locale}`)
    await page.locator('[data-testid="groups-cell"] button').waitFor()
    await page.evaluate(() => document.fonts.ready)
    const observation = await page.locator('[data-testid="groups-cell"]').evaluate(cell => {
      const texts = Array.from(cell.querySelectorAll('span')).filter(el => el.textContent.includes('must wrap') && el.children.length === 0)
      const button = cell.querySelector('button')
      const rect = button.getBoundingClientRect()
      const hit = document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2)
      return { textClipped: texts.some(el => el.scrollWidth > el.clientWidth + 1), horizontalOverflow: document.documentElement.scrollWidth > innerWidth + 1, moreButton: button.textContent.trim(), moreVisible: hit === button || button.contains(hit), textCount: texts.length }
    })
    if (observation.textClipped === modified || observation.horizontalOverflow || observation.textCount !== 4 || observation.moreButton !== '+2' || (modified && !observation.moreVisible)) throw new Error(JSON.stringify(observation))
    records.push({ phase, width, locale, ...observation })
    mkdirSync(`/evidence/ui/${phase}`, { recursive: true })
    if (locale === 'zh') await page.screenshot({ path: `/evidence/ui/${phase}/${width}.png`, fullPage: true })
  }
  if (modified) {
    await page.goto('http://127.0.0.1:4173/?translation=1&locale=en')
    await page.locator('[data-testid="group-i18n-menu"]').click()
    await page.locator('[data-locale="en"]').click()
    await page.locator('input[data-tour="group-form-name"]').fill('Edited English name')
    if (await page.locator('[data-testid="default-name"]').textContent() !== '默认名称') throw new Error('translation changed default')
    if (await page.locator('[data-testid="translated-name"]').textContent() !== 'Edited English name') throw new Error('translation did not bind')
    records.push({ phase, translation_edit: 'PASS', default_name_preserved: true, data_tour_on_input: true })
  }
  if (errors.length) throw new Error(errors.join('\n'))
  writeFileSync(`/evidence/ui/${phase}.json`, JSON.stringify(records, null, 2) + '\n')
  console.log(`${phase}: browser cases=${records.length}; clipping=${!modified}; mobile overflow=false; +2 visibility=${records.filter(r => 'moreVisible' in r).every(r => r.moreVisible)}; translation edit=${modified ? 'PASS' : 'not-present'}`)
} finally { await browser.close() }
