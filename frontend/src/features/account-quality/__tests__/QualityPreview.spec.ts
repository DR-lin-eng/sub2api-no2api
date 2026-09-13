import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
import QualityPreview from '../presentation/widgets/QualityPreview.vue'

describe('QualityPreview', () => {
  it('renders generated HTML in a scripts-enabled opaque sandbox', () => {
    const wrapper = mount(QualityPreview, {
      props: { point: { id: 'run-1', status: 'ready', started_at: '', details: { stage2: { status: 'passed', answer: '<svg/>', preview_html: '<meta http-equiv="Content-Security-Policy" content="default-src none"><svg><script>requestAnimationFrame(()=>{})</script></svg>' } } } },
    })
    const frame = wrapper.find('iframe')
    expect(frame.exists()).toBe(true)
    expect(frame.attributes('sandbox')).toBe('allow-scripts')
    expect(frame.attributes('srcdoc')).toContain('<svg>')
    expect(frame.attributes('srcdoc')).toContain('requestAnimationFrame')
    expect(frame.attributes('srcdoc')).not.toContain('allow-same-origin')
  })

  it('keeps image fallback for historical records', () => {
    const wrapper = mount(QualityPreview, { props: { point: { id: 'run-2', status: 'ready', started_at: '', has_preview: true } } })
    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(wrapper.find('img').attributes('src')).toContain('/api/v1/account-quality-share/image/run-2')
  })
})
