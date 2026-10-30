export default {
  mediaStudio: {
    title: '媒体工坊',
    description: '选择创作类型，输入描述，再按需要调整比例、清晰度和生成数量。',
    tabs: {
      studio: '创作',
      gallery: '作品库'
    },
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
      group: '分组',
      model: '模型',
      resolution: '分辨率',
      quality: '质量',
      count: '数量',
      duration: '时长',
      durationIncrease: '增加时长',
      durationDecrease: '减少时长',
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
      manualModelHint: '当前分组未返回当前类型的模型。',
      reload: '重试',
      shortHint: '选择管理员开放的媒体分组后即可生成；生成结果会保存在当前浏览器中。',
      advancedSettings: '更多',
      resourceSettings: '基础设置',
      imageSettings: '图片参数',
      videoSettings: '视频参数',
      unit: '张',
      countValue: '{count} 张',
      durationValue: '{count} 秒',
      send: '开始生成',
      imageEdit: {
        attachHint: '拖拽或粘贴参考图片，最多 9 张',
        attachCount: '已添加 {count}/{limit} 张',
        remove: '移除图片'
      },
      attachmentType: {
        image: '图片',
        video: '视频',
        audio: '音频'
      },
      upload: {
        addHint: '添加参考素材（图片 / 视频 / 音频）',
        dropHint: '拖拽文件到这里上传',
        disabled: '当前模型不允许上传文件',
        noTypes: '当前模型未开放任何可上传的文件类型',
        storageUnavailable: '管理员尚未配置对象存储，暂时无法上传',
        retry: '重试上传',
        rejections: {
          type: '「{name}」的文件类型不被当前模型支持',
          size: '「{name}」超过单文件大小上限（{limit} MB）',
          count: '「{name}」超出数量上限（最多 {limit} 个）',
          total: '「{name}」导致总大小超过上限（{limit} MB）'
        }
      },
      estimate: {
        loading: '正在预估消耗…',
        value: '预估消耗 {amount}',
        failed: '预估消耗不可用：{message}'
      }
    },
    session: {
      localHint: '会话和图片结果保存在当前浏览器中',
      new: '新建对话',
      sessions: '历史对话',
      sessionsEmpty: '还没有历史对话',
      untitled: '新对话',
      deleteConversation: '删除对话',
      deleteConfirm: '确定要删除这个对话吗？',
      messageCount: '{count} 条消息',
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
      elapsed: '耗时 {time}',
      cost: '扣费 {amount}',
      costHeld: '预冻结 {amount}',
      editImage: '基于此图继续修改',
      noImageResult: '任务已完成，但没有返回可预览图片。',
      noVideoResult: '任务已完成，但没有返回可预览视频。'
    },
    jobs: {
      title: '生成任务',
      activeCount: '{count} 个进行中',
      toggle: '展开或收起任务列表',
      status: {
        queued: '排队中',
        processing: '生成中',
        completed: '已完成',
        failed: '已失败'
      },
      elapsed: '已耗时 {time}',
      cost: '扣费 {amount}',
      costHeld: '预冻结 {amount}',
      retry: '重试',
      focus: '定位',
      errorCodes: {
        insufficient_quota: '额度不足，请充值后再试',
        unauthorized: '当前密钥无权访问该模型',
        rate_limited: '请求过于频繁，请稍后重试',
        content_policy: '提示词或素材未通过内容审核',
        timeout: '上游生成超时，请重试',
        upstream: '上游服务异常，请稍后重试',
        network: '网络连接中断，请检查网络后重试',
        unknown: '生成失败，请重试'
      }
    },
    gallery: {
      title: '作品库',
      subtitle: '生成完成的作品会自动归档到这里，仅保存在当前浏览器',
      loading: '正在读取作品…',
      empty: '还没有作品，生成完成后会自动出现在这里。',
      emptyFiltered: '没有符合当前筛选条件的作品。',
      untitled: '未命名作品',
      mediaMissing: '素材已不可用',
      removeConfirm: '确定要删除这件作品吗？',
      clearConfirm: '确定要清空作品库吗？此操作不可撤销。',
      storageUnavailable: '当前浏览器不支持本地存储，作品库暂不可用。',
      clear: '清空作品库',
      filters: {
        all: '全部',
        image: '图片',
        video: '视频',
        favoriteOnly: '只看收藏',
        search: '搜索提示词或模型',
        count: '{count} 件作品'
      },
      actions: {
        reuse: '复用参数',
        edit: '继续编辑',
        download: '下载',
        favorite: '收藏',
        unfavorite: '取消收藏',
        remove: '删除'
      }
    },
  }
}
