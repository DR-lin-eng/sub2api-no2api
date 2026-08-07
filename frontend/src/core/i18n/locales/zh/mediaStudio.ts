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
      }
    },
    composer: {
      assets: '资产库',
      greeting: '你好，想创作什么？',
      creationType: '创作类型',
      placeholderPrefix: '输入文字或',
      placeholderSuffix: '主体，描述你想生成的内容。',
      model: '图片 5.0 Lite',
      textTool: '文字工具',
      mention: '引用主体',
      unit: '张',
      send: '开始生成'
    }
  }
}
