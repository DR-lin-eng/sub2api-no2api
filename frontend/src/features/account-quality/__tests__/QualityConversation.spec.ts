import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
import QualityConversation from '../presentation/widgets/QualityConversation.vue'

describe('QualityConversation', () => {
  it('shows interrupted output warning while retaining code analysis', () => {
    const wrapper = mount(QualityConversation, {
      props: {
        title: 'stage2',
        detail: {
          status: 'interrupted',
          answer: '<svg>',
          output_interrupted: true,
          code_match: {
            version: 'code-fingerprint-v1', score: 55, raw_points: 55, max_points: 100, threshold: 55,
            source_complete: false, is_model_a: true, normal_class: 'model_a', matched_signals: ['js_trig_leg_animation'], missing_signals: [],
          },
        },
      },
    })
    expect(wrapper.text()).toContain('common.accountQuality.outputInterrupted')
    expect(wrapper.text()).toContain('common.accountQuality.codeMatchScore')
    expect(wrapper.text()).toContain('common.accountQuality.incompleteDrawing')
  })
})
