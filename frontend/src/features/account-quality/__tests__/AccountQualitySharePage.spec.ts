import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getPublicSnapshot } = vi.hoisted(() => ({ getPublicSnapshot: vi.fn() }))
vi.mock('../data/datasources/accountQualityPublicDatasource', () => ({ getPublicSnapshot }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key }) }))

import AccountQualitySharePage from '../presentation/pages/AccountQualitySharePage.vue'
import { qualityVerdict } from '../presentation/qualityDisplay'

describe('AccountQualitySharePage', () => {
  beforeEach(() => {
    getPublicSnapshot.mockReset().mockResolvedValue({
      model: 'gpt-quality', effort: 'medium', interval_minutes: 10, now: '2026-09-12T12:00:00Z',
      last_run_at: '2026-09-12T11:50:00Z', next_run_at: '2026-09-12T12:00:00Z',
      total: 1, passed: 1, degraded: 0, uncertain: 0, error: 0, model_version: '8.4.14',
      points: [{ id: 'run-1', status: 'ready', label: 'normal', model: 'gpt-quality', effort: 'medium', latency_ms: 1200, started_at: '2026-09-12T11:50:00Z', has_preview: true, details: { stage1: { status: 'passed', conversation_id: 'conv-123', response_id: 'resp-123', answer: '21', reasoning_tokens: 0 }, stage2: { status: 'passed', response_id: 'resp-456', answer: '<svg>pelican</svg>' } } }],
    })
  })

  it('shows conversation id, answer text and image preview', async () => {
    const wrapper = mount(AccountQualitySharePage, { global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, QualityPreview: { template: '<img src="/preview.webp" />' }, QualityConversation: { template: '<div>{{ title }} {{ detail?.conversation_id }} {{ detail?.answer }}</div>' } } } })
    await flushPromises()
    expect(getPublicSnapshot).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('conv-123')
    expect(wrapper.text()).toContain('21')
    expect(wrapper.find('img[src="/preview.webp"]').exists()).toBe(true)
  })

  it('hides empty failed conversations instead of showing a blank detail card', async () => {
    getPublicSnapshot.mockResolvedValueOnce({ model: '', effort: 'medium', interval_minutes: 10, now: '2026-09-12T12:00:00Z', total: 1, passed: 0, degraded: 0, uncertain: 0, error: 1, points: [{ id: 'run-2', status: 'error', started_at: '2026-09-12T11:50:00Z', details: { stage1: { status: 'error', answer: '' } } }] })
    const wrapper = mount(AccountQualitySharePage, { global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, QualityPreview: { template: '<div />' }, QualityConversation: { template: '<div />' } } } })
    await flushPromises()
    expect(wrapper.text()).not.toContain('common.accountQuality.idUnavailable')
    expect(wrapper.text()).not.toContain('account-')
  })

  it('uses the text-stage verdict when drawing output is interrupted', async () => {
    getPublicSnapshot.mockResolvedValueOnce({ model: 'gpt-quality', effort: 'medium', interval_minutes: 10, now: '2026-09-12T12:00:00Z', total: 1, passed: 1, degraded: 0, uncertain: 0, error: 0, points: [{ id: 'run-partial', status: 'ready', started_at: '2026-09-12T11:50:00Z', details: { stage1: { status: 'passed', answer: '21' }, stage2: { status: 'interrupted', answer: '<svg>', output_interrupted: true, code_match: { version: 'code-fingerprint-v1', score: 55, raw_points: 55, max_points: 100, threshold: 55, source_complete: false, is_model_a: true, normal_class: 'model_a', matched_signals: [], missing_signals: [] } } } }] })
    const wrapper = mount(AccountQualitySharePage, { global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, QualityPreview: { template: '<div />' }, QualityConversation: { template: '<div>{{ title }} {{ detail?.status }} {{ detail?.output_interrupted ? \'common.accountQuality.outputInterrupted\' : \'\' }}</div>' } } } })
    await flushPromises()
    expect(qualityVerdict({ id: 'run-partial', status: 'ready', started_at: '', details: { stage1: { status: 'passed', answer: '21' }, stage2: { status: 'interrupted', answer: '<svg>', output_interrupted: true, code_match: { version: 'code-fingerprint-v1', score: 55, raw_points: 55, max_points: 100, threshold: 55, source_complete: false, is_model_a: true, normal_class: 'model_a', matched_signals: [], missing_signals: [] } } } })).toBe('passed')
    expect(wrapper.text()).toContain('common.accountQuality.normal')
  })
})
