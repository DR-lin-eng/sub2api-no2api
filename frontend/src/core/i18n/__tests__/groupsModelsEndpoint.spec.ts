import { describe, expect, it } from 'vitest'
import { baseCompile } from '@intlify/message-compiler'
import en from '../locales/en'
import zh from '../locales/zh'

describe('group model-list endpoint labels', () => {
  for (const [locale, messages] of [['en', en], ['zh', zh]] as const) {
    for (const key of ['title', 'hint'] as const) {
      it(`${locale} ${key} compiles a named endpoint parameter`, () => {
        const text = messages.admin.groups.modelsList[key]
        const errors: string[] = []
        const result = baseCompile(text, { onError: error => errors.push(error.message) })
        expect(errors).toEqual([])
        expect(text).toContain('{endpoint}')
        expect(text).not.toContain('/v1/models')
        expect(result.code).toContain('_named("endpoint")')
      })
    }
  }
})
