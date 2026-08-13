export default {
  flattenNamespaces: '摊平 Codex namespace 工具',
  flattenNamespacesDesc:
    '仅用于不接受 namespace 工具的 OAuth 中继兼容。默认关闭以保留原生 Codex namespace 声明；compact 请求始终使用摊平工具。',
  codexFingerprintMode: 'Codex 指纹收敛',
  codexFingerprintModeDesc:
    '仅对 OpenAI OAuth 账号按需启用。旧账号升级和新建账号均保持关闭，只有管理员显式选择后才会收敛标识。',
  codexFingerprintModeOff: '关闭（保留客户端标识）',
  codexFingerprintModeDevice: '仅设备',
  codexFingerprintModeSession: '设备与会话',
  codexFingerprintModeFull: '设备、会话与线程',
  codexPrewarmContinuation: 'Codex 账号预热',
  codexThinkingTagNormalization: '规范化 Codex 思考标签',
  codexThinkingTagNormalizationDesc:
    '仅用于 OpenAI API Key 的 Responses 降级：将返回正文开头的 <thinking>...</thinking> 转为 Codex 思考项。默认关闭。',
}
