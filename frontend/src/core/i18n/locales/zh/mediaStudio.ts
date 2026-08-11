export default {
  mediaStudio: {
    title: '媒体工坊',
    description: '选择创作类型，输入描述，再按需要调整比例、清晰度和生成数量。',
    modeItems: {
      image: {
        title: '图片生成',
        short: '提示词、参考图、比例和风格'
      },
      video: {
        title: '视频生成',
        short: '首帧、运动描述、时长和比例'
      },
      batch: {
        title: '批量创作',
        short: '批量 Prompt、变量和队列'
      },
      disabled: '稍后'
    },
    composer: {
      greeting: '你好，想创作什么？',
      placeholder: '描述你想生成的图片，按 Ctrl/⌘ + Enter 开始生成。',
      model: '模型',
      selectKey: '选择 Key',
      loadingKeys: '加载 Key…',
      loadingModels: '加载模型…',
      noKeys: '没有可用的启用状态 API Key。',
      manualModelHint: '未从当前 Key 读取到图片模型，可以手动输入模型名。',
      reload: '重试',
      shortHint: '当前版本只提交图片任务；视频和批量入口先保留，后续接入。',
      unit: '张',
      countValue: '{count} 张',
      send: '开始生成'
    },
    session: {
      localHint: '会话保存在本机浏览器',
      clear: '清空',
      you: '你',
      studio: '媒体工坊',
      failed: '生成失败',
      retry: '重试',
      noImageResult: '任务已完成，但没有返回可预览图片。'
    }
  }
}
