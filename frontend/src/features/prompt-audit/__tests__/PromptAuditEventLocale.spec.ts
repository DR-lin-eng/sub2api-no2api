import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'
import type { PromptAuditEvent } from '../domain/models/promptAuditTypes'
import { emptyEventFilters } from '../domain/promptAuditViewModel'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      locale: { value: 'en' },
      t: (key: string) => key === 'admin.promptAudit.common.unknown' ? 'Unknown' : key,
    }),
  }
})

import EventDetailDialog from '../presentation/widgets/EventDetailDialog.vue'
import EventWorkspace from '../presentation/widgets/EventWorkspace.vue'

const DialogStub = defineComponent({
  template: '<div><slot /></div>',
})

const event = {
  id: 1,
  job_id: 1,
  decision: 'future_decision',
  risk_level: 'future_risk',
  action: 'future_action',
  categories: ['future_category'],
  matched_scanners: ['future_category'],
  scanner_scores: { future_category: 1 },
  scanner_evidence: { future_category: 'review evidence' },
  scanner_backend: 'guard',
  scanner_version: 'v1',
  guard_endpoint_id: 'guard-1',
  policy_id: 'priority',
  policy_version: 1,
  config_version: 1,
  chunk_total: 1,
  latency_ms: 5,
  issue_summaries: [{
    category: 'future_category',
    scanner_id: 'future_category',
    title: 'future title',
    description: 'future description',
    severity: 'future_severity',
    severity_label: 'future severity label',
    action: 'future_action',
    action_label: 'future action label',
    code: 'future_code',
    score: 1,
    evidence: 'review evidence',
    evidence_hash: 'hash',
  }],
  created_at: '2026-08-18T00:00:00Z',
  snapshot: {
    request_id: 'req-1',
    user_id: 1,
    username: 'alice',
    user_email: 'alice@example.test',
    api_key_id: 1,
    api_key_name: 'key',
    group_name: 'group',
    provider: 'openai',
    endpoint: '/v1/chat/completions',
    protocol: 'openai_chat',
    model: 'gpt-test',
    prompt_hash: 'hash',
    redacted_preview: 'preview',
    full_prompt: 'prompt',
    prompt_length: 6,
    message_count: 1,
    stage: 'http',
  },
} as unknown as PromptAuditEvent

describe('Prompt Audit event locale mapping', () => {
  it('uses a neutral localized fallback in the event table', () => {
    const wrapper = mount(EventWorkspace, {
      props: {
        events: [event],
        total: 1,
        page: 1,
        pageSize: 20,
        filters: emptyEventFilters(),
        selectedIds: [],
        loading: false,
        error: '',
      },
      global: { stubs: { Pagination: true } },
    })

    expect(wrapper.text()).toContain('Unknown · Unknown')
    expect(wrapper.get('[data-test="event-1"] span.rounded-full').classes()).toContain('bg-gray-100')
    expect(wrapper.text()).not.toContain('future_decision')
    expect(wrapper.text()).not.toContain('future_risk')
    expect(wrapper.text()).not.toContain('future_category')
  })

  it('does not expose future enum values in summary, issue, or guard-return views', () => {
    const wrapper = mount(EventDetailDialog, {
      props: { show: true, event, loading: false },
      global: { stubs: { BaseDialog: DialogStub } },
    })

    expect(wrapper.text()).toContain('Unknown · Unknown')
    expect(wrapper.text()).not.toContain('future_decision')
    expect(wrapper.text()).not.toContain('future_risk')
    expect(wrapper.text()).not.toContain('future_action')
    expect(wrapper.text()).not.toContain('future_category')
    expect(wrapper.text()).not.toContain('admin.promptAudit.decisions.future')
  })
})
