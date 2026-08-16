export default {
  title: '在线客服',
  description: '控制用户端在线客服入口和管理员客服收件箱入口。默认关闭，确认发布后再显式开启。',
  enabled: '启用在线客服',
  enabledHint: '关闭后隐藏用户端在线客服和管理员客服收件箱，并停止侧边栏未读红点轮询。',
  retentionDays: '消息记录保留天数',
  retentionDaysHint: '范围 0–3650 天；0 表示永久保留。保存后后台会在约 10 分钟内分批永久删除过期消息和无引用的消息图片。',
  retentionFinancialHint: '余额转账回执属于财务凭证，不受此保留时长影响。',
}
