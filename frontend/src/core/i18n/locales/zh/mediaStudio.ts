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
      disabled: '不可用'
    },
    composer: {
      greeting: '你好，想创作什么？',
      placeholder: '描述你想生成的图片，按 Ctrl/⌘ + Enter 开始生成。',
      videoPlaceholder: '描述画面、镜头运动和氛围，按 Ctrl/⌘ + Enter 开始生成视频。',
      model: '模型',
      customWidth: '自定义宽度',
      customHeight: '自定义高度',
      aspectRatio: '比例',
      customAspectRatio: {
        option: '自定义比例',
        title: '添加自定义比例',
        width: '横向比例',
        height: '纵向比例',
        hint: '比例值需为正整数，长边与短边的比例不能超过 3:1。',
        invalid: '请输入符合比例约束的数值。',
        add: '添加'
      },
      selectGroup: '选择媒体分组',
      loadingGroups: '加载媒体分组…',
      loadingModels: '加载模型…',
      noGroups: '管理员暂未配置可用的媒体分组。',
      manualModelHint: '未从当前分组读取到当前类型的模型，可以手动输入模型名。',
      reload: '重试',
      shortHint: '选择管理员开放的媒体分组后即可生成；生成结果会保存在当前浏览器中。',
      unit: '张',
      countValue: '{count} 张',
      durationValue: '{count} 秒',
      send: '开始生成',
      imageEdit: {
        attachHint: '拖拽或粘贴参考图片，最多 9 张',
        attachCount: '已添加 {count}/{limit} 张',
        remove: '移除图片'
      }
    },
    session: {
      localHint: '会话和图片结果保存在当前浏览器中',
      clear: '清空',
      select: '选择',
      selectAll: '全选',
      deselectAll: '取消全选',
      deleteSelected: '删除所选',
      cancelSelect: '取消选择',
      selectMessage: '选择消息',
      selectedCount: '已选择 {count} 条',
      you: '你',
      studio: '媒体工坊',
      failed: '生成失败',
      retry: '重试',
      enlargeImage: '放大图片',
      editImage: '基于此图继续修改',
      noImageResult: '任务已完成，但没有返回可预览图片。',
      noVideoResult: '任务已完成，但没有返回可预览视频。'
    },
  }
}
