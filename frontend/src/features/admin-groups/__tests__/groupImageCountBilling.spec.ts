import { describe, expect, it } from 'vitest'
import {
  createCreateGroupFormState,
  createEditGroupFormState,
  resetDisabledBatchImagePricing,
} from '../presentation/composables/groupEditorFormSupport'

describe('per-image billing form compatibility', () => {
  it('defaults both editors to the existing billing behavior', () => {
    expect(createCreateGroupFormState().image_count_billing).toBe(false)
    expect(createEditGroupFormState().image_count_billing).toBe(false)
  })

  it('clears opt-in when image generation is disabled or the platform changes', () => {
    const form = createCreateGroupFormState()
    form.platform = 'openai'
    form.allow_image_generation = true
    form.image_count_billing = true
    resetDisabledBatchImagePricing(form)
    expect(form.image_count_billing).toBe(true)
    form.platform = 'gemini'
    resetDisabledBatchImagePricing(form)
    expect(form.image_count_billing).toBe(false)
    form.platform = 'openai'
    form.image_count_billing = true
    form.allow_image_generation = false
    resetDisabledBatchImagePricing(form)
    expect(form.image_count_billing).toBe(false)
  })
})
