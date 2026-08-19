export default {
  title: '自定义模型配置',
  description: '配置非主流模型的多模态能力',

  table: {
    modelName: '模型名称',
    prefixMatch: '前缀匹配',
    capabilities: '能力',
    actions: '操作',
    empty: '暂无配置',
    emptyHint: '点击"新增配置"按钮添加自定义模型配置'
  },

  capabilities: {
    image: '图像生成',
    video: '视频生成',
    audio: '音频生成'
  },

  actions: {
    backToSettings: '返回功能开关',
    create: '新增配置',
    edit: '编辑',
    delete: '删除',
    deleteConfirm: '确定要删除这个配置吗？',
    deleteFailed: '删除模型配置失败'
  },

  modal: {
    createTitle: '新增模型配置',
    editTitle: '编辑模型配置',
    modelName: '模型名称',
    modelNamePlaceholder: '例如：flux-pro, midjourney-v6',
    modelNameHint: '输入完整的模型名称，需与API返回的模型名称完全匹配',
    modelNamePrefixHint: '输入模型名前缀，例如 agnes-，将匹配所有以该前缀开头的模型',
    prefixMatch: '按模型名前缀匹配',
    prefixMatchHint: '启用后，输入内容会作为前缀匹配多个模型；关闭时仅匹配完整模型名称',
    capabilities: '模型能力',
    capabilitiesHint: '勾选该模型支持的全部能力',
    saveFailed: '保存失败，请重试'
  },

  tips: {
    title: '使用说明',
    content: '此功能允许您为非主流模型手动配置多模态能力。配置后，系统将根据这些配置判断模型是否支持图像、视频或音频生成功能。'
  }
}
