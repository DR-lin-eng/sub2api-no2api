import { afterEach, describe, expect, it } from 'vitest'
import {
  clearModelCapabilities,
  initializeModelCapabilities,
  isMediaStudioImageModel,
  isMediaStudioVideoModel,
} from '../domain/services/modelCapabilityService'
import type { CustomModelConfig } from '../domain/entities/customModelConfig'

function config(
  model_name: string,
  capabilities: CustomModelConfig['capabilities'],
  prefix_match = false,
): CustomModelConfig {
  return {
    id: model_name.length,
    model_name,
    prefix_match,
    capabilities,
    created_at: '',
    updated_at: '',
  }
}

describe('model capability prefix matching', () => {
  afterEach(() => clearModelCapabilities())

  it('matches all models that start with a configured prefix', () => {
    initializeModelCapabilities([
      config('agnes-', ['image'], true),
    ])

    expect(isMediaStudioImageModel('agnes-image-2.0-flash')).toBe(true)
    expect(isMediaStudioImageModel('agnes-video-v1')).toBe(true)
    expect(isMediaStudioImageModel('other-agnes-image')).toBe(false)
  })

  it('prefers an exact configuration over a matching prefix', () => {
    initializeModelCapabilities([
      config('agnes-', ['image'], true),
      config('agnes-image-2.0-flash', ['video']),
    ])

    expect(isMediaStudioImageModel('agnes-image-2.0-flash')).toBe(false)
    expect(isMediaStudioVideoModel('agnes-image-2.0-flash')).toBe(true)
  })

  it('prefers the longest matching prefix', () => {
    initializeModelCapabilities([
      config('agnes-', ['image'], true),
      config('agnes-image-', ['video'], true),
    ])

    expect(isMediaStudioImageModel('agnes-image-2.0-flash')).toBe(false)
    expect(isMediaStudioVideoModel('agnes-image-2.0-flash')).toBe(true)
  })
})
