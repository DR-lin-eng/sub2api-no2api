import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const currentDir = dirname(fileURLToPath(import.meta.url))
const source = readFileSync(resolve(currentDir, '../AppHeader.vue'), 'utf8')

describe('AppHeader responsive width constraints', () => {
  it('lets the title shrink before header actions overflow the viewport', () => {
    expect(source).toContain('class="flex min-w-0 flex-1 items-center gap-2 sm:gap-4"')
    expect(source).toContain('class="hidden min-w-0 lg:block"')
    expect(source).toContain('class="truncate text-lg font-semibold text-gray-900 dark:text-white"')
    expect(source).toContain('class="truncate text-xs text-gray-500 dark:text-dark-400"')
    expect(source).toContain('class="flex shrink-0 items-center gap-1 sm:gap-3"')
  })

  it('bounds long account names inside the user menu trigger', () => {
    expect(source).toContain('class="hidden min-w-0 max-w-32 text-left md:block"')
    expect(source).toContain('class="truncate text-sm font-medium text-gray-900 dark:text-white"')
  })
})
