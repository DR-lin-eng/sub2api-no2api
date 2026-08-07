export default {
  mediaStudio: {
    title: '媒体工坊',
    description: '面向图片、视频和批量创意生成的统一工作台预览。先搭好创作入口、能力卡片和任务流骨架，后续可直接接入真实生成 API。',
    hero: {
      eyebrow: 'Media Studio Preview',
      previewLabel: '当前预览能力',
      tags: {
        images: '图片生成',
        videos: '视频生成',
        preview: '任务预览'
      },
      stats: {
        modes: {
          value: '3',
          label: '媒体入口'
        },
        flow: {
          value: '4',
          label: '标准步骤'
        },
        api: {
          value: '0',
          label: '待接入 API'
        }
      }
    },
    status: {
      preview: '预览壳子',
      comingSoon: '待接入'
    },
    modes: {
      eyebrow: '生成类型',
      title: '为后续多媒体生成预留入口',
      description: '每个入口都保持独立的 UI 和状态 owner，后续接入图片、视频或批量任务时不需要重排页面结构。',
      selectPreview: '查看预览'
    },
    modeItems: {
      image: {
        title: '图片生成',
        description: '预留提示词、尺寸、风格、模型选择和结果预览区域。',
        hint: '图片生成会在这里展示网格预览、下载入口和失败重试状态。'
      },
      video: {
        title: '视频生成',
        description: '预留脚本、时长、比例、参考图和分镜预览区域。',
        hint: '视频生成会在这里展示封面、转码状态、播放预览和素材引用。'
      },
      batch: {
        title: '批量创作',
        description: '预留批量提示词、队列进度、结果归档和统一导出区域。',
        hint: '批量创作会在这里展示任务队列、成功率、成本占用和批量下载。'
      }
    },
    workspace: {
      eyebrow: '工作流骨架',
      title: '从提示词到交付的统一链路',
      description: '壳层先固定输入、配置、生成和交付四段结构。真实接口上线后，只需替换数据源和任务状态。',
      canvasLabel: 'Preview Canvas',
      generatePlaceholder: '生成按钮占位',
      previewState: '等待接入生成服务'
    },
    stages: {
      prompt: {
        title: '输入创意提示词',
        description: '承载文本、参考素材和批量条目。'
      },
      configure: {
        title: '选择模型与参数',
        description: '承载尺寸、比例、时长、风格和质量配置。'
      },
      generate: {
        title: '提交生成任务',
        description: '承载队列、进度、失败重试和取消状态。'
      },
      deliver: {
        title: '预览与交付结果',
        description: '承载预览、下载、归档和用量回显。'
      }
    }
  }
}
