export default {
  title: 'Support Chat',
  description: 'Control the user Support Chat entry and the admin Support Inbox entry. Disabled by default; enable it explicitly after reviewing the rollout.',
  enabled: 'Enable Support Chat',
  enabledHint: 'When off, both support sidebar entries are hidden and unread badge polling stops.',
  retentionDays: 'Message retention (days)',
  retentionDaysHint: 'Range: 0–3650 days. Use 0 to retain indefinitely. After saving, expired messages and unreferenced message images are permanently removed in batches within about 10 minutes.',
  retentionFinancialHint: 'Balance-transfer receipts are financial records and are not removed by this retention setting.',
}
