export default {
  title: 'Support Chat',
  description: 'Control the user Support Chat entry and the admin Support Inbox entry. Disabled by default; enable it explicitly after reviewing the rollout.',
  enabled: 'Enable Support Chat',
  enabledHint: 'When off, both support sidebar entries are hidden and unread badge polling stops.',
  retentionEnabled: 'Enable automatic message cleanup',
  retentionEnabledHint: 'Off by default. Messages are permanently removed only when this switch is on and the retention period is greater than zero.',
  retentionDays: 'Message retention (days)',
  retentionDaysHint: 'Range: 0–3650 days. Use 0 to retain indefinitely. When enabled, ordinary messages and unreferenced images are removed after the period regardless of read state.',
  retentionFinancialHint: 'Balance-transfer receipts are financial records and are not removed by this retention setting.',
}
