# Agnes Video API 集成说明

## 透传架构

Agnes Video API 采用**透传方式**集成，与 Grok Video 使用相同的归一化逻辑。

```
Agnes 响应 → normalizeMediaStudioVideoTask() → MediaStudioVideoTask
Grok 响应 → normalizeMediaStudioVideoTask() → MediaStudioVideoTask
```

## 类型定义

### 请求类型
```typescript
interface AgnesVideoGenerationRequest {
  model: string
  prompt: string
  image_url?: string              // 图生视频
  duration?: number               // 默认 5 秒
  aspect_ratio?: '16:9' | '9:16' | '1:1'
  motion_level?: 1 | 2 | 3 | 4 | 5
  negative_prompt?: string
}
```

### 响应类型（透传）
```typescript
interface MediaStudioVideoTask {
  id: string                      // 提取自 task_id
  status: 'processing' | 'completed' | 'failed'
  error?: string                  // 提取自 error_message
  raw: Record<string, unknown>    // 完整 Agnes 响应
}
```

## 字段兼容

### 任务 ID 提取优先级
```typescript
// Grok: 8 个可能位置
request_id → requestId → id → data.request_id → data.id → 
video.request_id → video.id → result.request_id → result.id → 
task_id → data.task_id → video.task_id

// Agnes: 固定位置
task_id
```

### 状态映射
```typescript
// Agnes 原始状态
pending      → processing
processing   → processing
completed    → completed
failed       → failed

// Grok 兼容状态（不变）
complete/succeeded/success/done → completed
error/cancelled/canceled/expired → failed
```

### 错误信息提取
```typescript
// Grok: error.message → raw.message → data.error → result.error
// Agnes: error_message
```

## API 函数

```typescript
// 提交生成（返回归一化的 MediaStudioVideoTask）
await submitAgnesVideoGeneration(apiKey, {
  model: 'agnes-video-v2.0',
  prompt: '一只橘猫在阳光下睡觉',
  duration: 5,
  aspect_ratio: '16:9',
  motion_level: 3,
}, 'idempotency-key-123')

// 查询状态（返回归一化的 MediaStudioVideoTask）
await getAgnesVideoTask(apiKey, 'task-id-xxx')

// 访问原始响应
const task = await getAgnesVideoTask(apiKey, taskId)
const agnesRaw = task.raw as AgnesVideoQueryResponse
console.log('进度:', agnesRaw.progress)           // Agnes 特有
console.log('视频URL:', agnesRaw.video_url)       // Agnes 特有
console.log('封面URL:', agnesRaw.cover_url)       // Agnes 特有
```

## 轮询实现

复用现有的视频轮询逻辑（前端 composable）：

```typescript
// 前端已有通用轮询
function pollVideoTask(apiKey: string, taskId: string) {
  const interval = setInterval(async () => {
    const task = await getAgnesVideoTask(apiKey, taskId)
    
    if (task.status === 'completed') {
      clearInterval(interval)
      const agnesRaw = task.raw as AgnesVideoQueryResponse
      showVideo(agnesRaw.video_url)
    }
    
    if (task.status === 'failed') {
      clearInterval(interval)
      showError(task.error)
    }
  }, 3000)
}
```

## 获取视频内容

**注意：Agnes 可能直接返回视频 URL，不需要走 `/content` 代理**

```typescript
const task = await getAgnesVideoTask(apiKey, taskId)
const agnesRaw = task.raw as AgnesVideoQueryResponse

if (task.status === 'completed') {
  // 优先使用 Agnes 原始 URL
  if (agnesRaw.video_url) {
    videoElement.src = agnesRaw.video_url
  } else {
    // 回退到 Grok 模式的代理下载
    const blob = await getVideoGenerationContent(apiKey, task.id)
    videoElement.src = URL.createObjectURL(blob)
  }
}
```

## Agnes 特有字段

通过 `task.raw` 访问：

```typescript
const agnesRaw = task.raw as AgnesVideoQueryResponse

agnesRaw.progress        // 进度 0-100（processing 时）
agnesRaw.video_url       // 视频 URL（completed 时）
agnesRaw.cover_url       // 封面图 URL（completed 时）
agnesRaw.prompt          // 提示词（可能被优化过）
agnesRaw.duration        // 时长
agnesRaw.aspect_ratio    // 比例
agnesRaw.motion_level    // 运动幅度
agnesRaw.created_at      // 创建时间（ISO 8601）
agnesRaw.completed_at    // 完成时间（ISO 8601）
```

## 后端路由要求

Agnes Video 需要后端添加这些路由（参考 Grok）：

```
POST /v1/video/generations        → 转发到 Agnes API
GET  /v1/video/generations/:id    → 转发查询请求
```

如果 Agnes 返回的视频 URL 需要代理：
```
GET  /v1/video/generations/:id/content  → 下载并转发视频流
```

## 与 Grok 的对比

| 特性 | Grok | Agnes |
|------|------|-------|
| 任务 ID 字段 | 8 个位置 | `task_id` 固定 |
| 状态枚举 | 不统一 | 4 个标准状态 |
| 错误字段 | `error.message` | `error_message` |
| 进度反馈 | ❌ 无 | ✅ `progress` |
| 视频 URL | `/content` 代理 | 可能直接返回 |
| 封面图 | ❌ 无 | ✅ `cover_url` |
| 运动控制 | ❌ 无 | ✅ `motion_level` |

## 使用建议

1. **提交时指定幂等键**，避免重复生成
2. **轮询间隔 3-5 秒**，Agnes 有 `progress` 可以频繁查询
3. **优先使用 Agnes 原始 URL**，避免不必要的代理转发
4. **错误处理**：`task.error` 已归一化，直接使用
5. **进度显示**：检查 `task.raw.progress` 是否存在
