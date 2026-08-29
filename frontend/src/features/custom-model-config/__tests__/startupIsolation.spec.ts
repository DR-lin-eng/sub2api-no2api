import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

describe('custom model capability startup isolation', () => {
  it('does not call the admin configuration API from the application bootstrap', () => {
    const mainSource = readFileSync(resolve(process.cwd(), 'src/main.ts'), 'utf8')
    expect(mainSource).not.toContain('initializeModelCapabilitiesOnStartup')
    expect(mainSource).not.toContain('custom-model-config')
  })
})
