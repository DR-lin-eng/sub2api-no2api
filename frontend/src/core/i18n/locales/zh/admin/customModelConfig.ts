export default {
  title: '自定义模型配置',
  description: '配置非主流模型的多模态能力',

  table: {
    modelName: '模型名称',
    prefixMatch: '前缀匹配',
    capabilities: '能力',
    requestTemplate: '请求模板',
    noTemplate: '未使用模板',
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

  template: {
    manage: '模板管理',
    import: '导入模板',
    export: '复制模板 JSON',
    copied: '模板 JSON 已复制',
    copyFailed: '复制模板 JSON 失败',
    managerTitle: '请求模板管理',
    managerHint: '模板集中保存请求头、请求体和端点适配规则，模型配置只引用模板。',
    create: '新增模板',
    createTitle: '新增请求模板',
    editTitle: '编辑请求模板',
    importTitle: '导入请求模板',
    importInput: '粘贴模板 JSON',
    importPreview: '格式化预览',
    importAction: '导入并保存',
    importHint: '可粘贴完整模板 JSON，也可以只粘贴 request_adapter。点击格式化后会自动补齐缺失结构。',
    importedName: '导入的请求模板',
    invalidTemplate: '模板必须是合法的 JSON 对象',
    importFailed: '导入模板失败',
    name: '模板名称',
    namePlaceholder: '例如：图片编辑转 JSON',
    description: '说明',
    descriptionPlaceholder: '例如：将 multipart 图片编辑请求转换为 JSON 请求',
    sourceEndpoint: '匹配客户端端点',
    targetEndpoint: '上游端点',
    contentType: '上游请求格式',
    preserve: '保持原请求格式',
    headers: '自定义请求头',
    headersHint: '填写后会覆盖同名的上游请求头，认证和 Content-Type 由系统管理。',
    addHeader: '添加请求头',
    headerName: '请求头名称',
    headerValue: '请求头值',
    noHeaders: '未配置额外请求头',
    bodyMode: '请求体处理',
    bodyModes: {
      off: '保持原请求体',
      merge: '局部合并请求体',
      replace: '完整替换请求体'
    },
    requestBody: '请求体 JSON',
    format: '格式化',
    loadSample: '加载样例',
    sampleName: '图片编辑转 JSON 样例',
    sampleDescription: '将图片编辑请求转换为 JSON，并把参考图片转换到 image 数组。',
    variablesTitle: '可用变量',
    mergeHint: '配置内容会递归合并到原始 JSON 请求体中。',
    replaceHint: '上游请求将完整使用这里配置的 JSON 请求体。',
    noDescription: '暂无说明',
    empty: '暂无请求模板',
    invalidJson: '请求体必须是合法的 JSON 对象',
    saveFailed: '保存请求模板失败',
    deleteConfirm: '确定删除这个请求模板吗？引用它的模型会自动解除关联。',
    deleteFailed: '删除请求模板失败'
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
    requestTemplate: '请求模板',
    noTemplate: '不使用请求模板',
    requestTemplateHint: '多个模型可以引用同一个模板，模板修改后统一生效。',
    videoApiType: {
      label: '视频 API 类型',
      autoDetect: '自动检测',
      grok: 'Grok Video',
      agnes: 'Agnes Video'
    },
    videoApiTypeHint: '选择该视频模型使用的 API 接口类型',
    saveFailed: '保存失败，请重试'
  },

  tips: {
    title: '使用说明',
    content: '此功能允许您为非主流模型手动配置多模态能力。配置后，系统将根据这些配置判断模型是否支持图像、视频或音频生成功能。'
  }
}
