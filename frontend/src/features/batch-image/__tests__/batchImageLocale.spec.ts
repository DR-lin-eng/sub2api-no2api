import { describe, expect, it } from 'vitest'
import {
  batchImageItemStatusLabel,
  batchImageJobStatusLabel,
} from '@/features/batch-image/presentation/batchImageLocale'

describe.each([
  {
    name: 'English',
    messages: {
      'batchImage.status.processingResults': 'Processing results',
      'batchImage.itemStatus.succeeded': 'Succeeded',
      'common.unknown': 'Unknown',
    },
  },
  {
    name: 'Chinese',
    messages: {
      'batchImage.status.processingResults': '处理结果中',
      'batchImage.itemStatus.succeeded': '成功',
      'common.unknown': '未知',
    },
  },
])('batch image locale mapping - $name', ({ messages }) => {
  const t = (key: string) => messages[key as keyof typeof messages] ?? key

  it('normalizes aliases and localizes future values', () => {
    expect(batchImageJobStatusLabel(t, 'indexing')).toBe(messages['batchImage.status.processingResults'])
    expect(batchImageJobStatusLabel(t, 'processing_results')).toBe(messages['batchImage.status.processingResults'])
    expect(batchImageItemStatusLabel(t, 'success')).toBe(messages['batchImage.itemStatus.succeeded'])
    expect(batchImageJobStatusLabel(t, 'future_job')).toBe(messages['common.unknown'])
    expect(batchImageItemStatusLabel(t, 'future_item')).toBe(messages['common.unknown'])
  })
})
