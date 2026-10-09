import { createRequire } from 'node:module'
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs'
const require = createRequire('/opt/qa/package.json')
const { chromium } = require('playwright-core')
const phase = process.argv[2]
const input = JSON.parse(readFileSync('/evidence/INPUT.json', 'utf8'))
const fixed = phase === 'MODIFIED'
const output = `/evidence/${phase.toLowerCase()}`
mkdirSync(output, { recursive: true })
const browser = await chromium.launch({ executablePath: '/usr/bin/chromium-browser', headless: true, args: ['--no-sandbox', '--disable-dev-shm-usage'] })
const context = await browser.newContext({ viewport: { width: 1440, height: 800 }, deviceScaleFactor: 1 })
const page = await context.newPage()
const errors = []
page.on('pageerror', (error) => errors.push(String(error)))
const readyDeadline = Date.now() + 45000
while (true) {
  try {
    const response = await fetch('http://127.0.0.1:4173/layout-fixture.html')
    if (response.ok) break
  } catch {}
  if (Date.now() > readyDeadline) throw new Error('Vite fixture did not become ready within 45 seconds')
  await new Promise((resolve) => setTimeout(resolve, 300))
}
const records = []
const labels = {
  zh: ['首字', '总耗时', '速度', '本地首字', 'OpenAI 引擎首字'],
  en: ['First', 'Total', 'Speed', 'Local TTFT', 'OpenAI engine TTFT'],
}
async function check({ width, locale, audience, theme, extra = false, missing = '' }) {
  await page.setViewportSize({ width, height: 800 })
  const query = new URLSearchParams({ locale, audience, theme, extra: extra ? '1' : '0', missing })
  await page.goto(`http://127.0.0.1:4173/layout-fixture.html?${query}`)
  await page.waitForSelector('body[data-ready="true"]')
  await page.evaluate(() => document.fonts.ready)
  const observation = await page.evaluate(({ locale, labels }) => {
    const metrics = [...document.querySelectorAll('[data-testid="output-token-speed"]')].map((speed) => {
      const root = speed.closest('.grid-cols-\\[max-content_max-content\\]')
      const pairs = labels[locale].map((text) => {
        const label = [...root.querySelectorAll('span,dt')].find((element) => element.textContent === text)
        if (!label) return null
        const value = label.nextElementSibling
        const left = label.getBoundingClientRect()
        const right = value.getBoundingClientRect()
        return { label: text, value: value.textContent.trim(), sameLine: Math.abs(left.top - right.top) < 2, separateColumns: left.right <= right.left, top: left.top, valueLeft: right.left }
      }).filter(Boolean)
      return { pairs, width: root.getBoundingClientRect().width, allPairsAligned: pairs.every((pair) => pair.sameLine && pair.separateColumns) }
    })
    return { metrics, horizontalOverflow: innerWidth < document.documentElement.scrollWidth }
  }, { locale, labels })
  const first = observation.metrics[0].pairs
  const total = first.find((pair) => pair.label === labels[locale][1])
  const expectedTotal = missing === 'zero' ? '0ms' : '52.07s'
  const totalMatches = total?.value === expectedTotal
  const diagnosticsVisible = first.some((pair) => pair.label === labels[locale][3])
  const expectedDiagnostics = audience === 'admin' && missing !== 'timing'
  const diagnosticsCorrect = diagnosticsVisible === expectedDiagnostics
  const expectedFailure = extra && !fixed
  const success = expectedFailure ? !totalMatches : totalMatches && observation.metrics.every((metric) => metric.allPairsAligned)
  const okay = success && diagnosticsCorrect && !observation.horizontalOverflow
  records.push({ width, locale, audience, theme, extra, missing, expectedFailure, okay, ...observation })
  if (!okay) throw new Error(JSON.stringify(records.at(-1)))
  if (locale === 'zh' && audience === 'admin' && theme === 'dark' && [375,1440].includes(width) && !missing) {
    await page.screenshot({ path: `${output}/${width}${extra ? '-extra' : ''}.png`, fullPage: true })
  }
}
try {
  for (const width of input.viewports) for (const locale of input.locales) for (const audience of input.audiences) for (const theme of input.themes) await check({ width, locale, audience, theme })
  for (const width of [375,1440]) await check({ width, locale: 'zh', audience: 'admin', theme: 'dark', extra: true })
  for (const missing of ['first','timing','zero']) for (const width of [320,1440]) for (const audience of input.audiences) await check({ width, locale: 'zh', audience, theme: 'dark', missing })
  if (errors.length) throw new Error(errors.join('\n'))
  writeFileSync(`${output}/results.json`, JSON.stringify({ phase, browser: browser.version(), records, errors }, null, 2) + '\n')
  console.log(`${phase}: cases=${records.length}; normal_pairs=aligned; user_diagnostics=hidden; mobile_overflow=none; extra_nodes=${fixed ? 'pairs stay aligned' : 'total label displays 输出 TPS (screenshot mismatch reproduced)'}`)
  const sample = records.find((record) => record.extra && record.width === 1440)
  console.log(`${phase}: observed_first_row=${JSON.stringify(sample.metrics[0].pairs.map(({ label, value, sameLine }) => ({ label, value, sameLine })))}`)
} finally {
  await browser.close()
}
