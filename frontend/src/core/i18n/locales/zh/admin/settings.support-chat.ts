export default {
  title: '在线客服',
  description: '控制用户端在线客服入口和管理员客服收件箱入口。默认关闭，确认发布后再显式开启。',
  enabled: '启用在线客服',
  enabledHint: '关闭后隐藏用户端在线客服和管理员客服收件箱，并停止侧边栏未读红点更新。',
  retentionEnabled: '启用消息自动清理',
  retentionEnabledHint: '默认关闭。只有开启此开关且保留天数大于 0 时，后台才会永久删除过期消息。',
  retentionDays: '消息记录保留天数',
  retentionDaysHint: '范围 0–3650 天；0 表示永久保留。开启自动清理后，后台会分批永久删除过期消息和无引用的消息图片。',
  retentionFinancialHint: '余额转账回执属于财务凭证，不受此保留时长影响。',
}
