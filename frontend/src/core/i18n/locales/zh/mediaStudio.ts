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
      disabled: '不可用'
    },
    composer: {
      greeting: '你好，想创作什么？',
      placeholder: '描述你想生成的图片，按 Ctrl/⌘ + Enter 开始生成。',
      videoPlaceholder: '描述画面、镜头运动和氛围，按 Ctrl/⌘ + Enter 开始生成视频。',
      model: '模型',
      selectKey: '选择 Key',
      loadingKeys: '加载 Key…',
      loadingModels: '加载模型…',
      noKeys: '没有可用的启用状态 API Key。',
      manualModelHint: '未从当前 Key 读取到当前类型的模型，可以手动输入模型名。',
      reload: '重试',
      shortHint: '图片和视频都通过所选 API Key 提交；生成结果只保留在当前页面内存中。',
      batchHint: '批量创作复用项目现有的完整任务队列、费用预估和下载流程。',
      unit: '张',
      countValue: '{count} 张',
      durationValue: '{count} 秒',
      send: '开始生成'
    },
    session: {
      localHint: '会话仅保留在当前页面内存中',
      clear: '清空',
      you: '你',
      studio: '媒体工坊',
      failed: '生成失败',
      retry: '重试',
      noImageResult: '任务已完成，但没有返回可预览图片。',
      noVideoResult: '任务已完成，但没有返回可预览视频。'
    },
    batch: {
      title: '进入批量图片工作区',
      description: '使用已经过验证的 Gemini 批量任务流程，可提交多条 Prompt、跟踪任务、预览结果并下载压缩包。可用 Key 和分组权限会在工作区内再次校验。',
      open: '打开批量工作区'
    }
  }
}
