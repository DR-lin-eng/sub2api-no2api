import { describe, expect, it } from 'vitest'

import {
  REQUEST_TEMPLATE_IMPORT_PLACEHOLDER,
  REQUEST_TEMPLATE_VARIABLES,
} from '../presentation/requestTemplateLiterals'

describe('request template literals', () => {
  it('provides a valid JSON import sample without passing it through vue-i18n', () => {
    const sample = JSON.parse(REQUEST_TEMPLATE_IMPORT_PLACEHOLDER) as {
      request_adapter?: { body?: { value?: { image?: string } } }
    }

    expect(sample.request_adapter?.body?.value?.image).toBe(
      '{{request.input_images}}',
    )
  })

  it('keeps request adapter variables as literal braces', () => {
    expect(REQUEST_TEMPLATE_VARIABLES).toContain('{{request.model}}')
    expect(REQUEST_TEMPLATE_VARIABLES).toContain('{{request.body.some_field}}')
  })
})
