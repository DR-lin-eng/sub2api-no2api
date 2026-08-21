import { ref, onMounted } from 'vue'
import {
  listAdminChatQuickReplies,
  createAdminChatQuickReply,
  updateAdminChatQuickReply,
  deleteAdminChatQuickReply,
  reorderAdminChatQuickReplies,
  importAdminChatQuickReplies,
} from '@/features/support-chat/data/datasources/supportChatDatasource'

export interface QuickReply {
  id: string | number
  title: string
  content: string
  custom?: boolean
}

const LOCAL_STORAGE_KEY = 'support_chat_custom_replies_v1'
const MIGRATED_FLAG_KEY = 'support_chat_quick_replies_migrated'

export function useQuickReplies() {
  const quickReplies = ref<QuickReply[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  // 从 LocalStorage 加载旧数据（用于迁移）
  function loadFromLocalStorage(): QuickReply[] {
    try {
      const raw = localStorage.getItem(LOCAL_STORAGE_KEY)
      if (!raw) return []
      const parsed = JSON.parse(raw) as QuickReply[]
      if (!Array.isArray(parsed)) return []
      return parsed.filter((item) => item && typeof item.title === 'string' && typeof item.content === 'string')
    } catch {
      return []
    }
  }

  // 检查是否已迁移
  function isMigrated(): boolean {
    return localStorage.getItem(MIGRATED_FLAG_KEY) === 'true'
  }

  // 标记已迁移
  function markAsMigrated() {
    localStorage.setItem(MIGRATED_FLAG_KEY, 'true')
  }

  // 一次性迁移：将 LocalStorage 数据上传到后端
  async function migrateFromLocalStorage(): Promise<void> {
    if (isMigrated()) return

    const localData = loadFromLocalStorage()
    if (localData.length === 0) {
      markAsMigrated()
      return
    }

    try {
      // 批量创建
      for (const item of localData) {
        await createAdminChatQuickReply({ title: item.title, content: item.content })
      }
      markAsMigrated()
      // 清理旧数据
      localStorage.removeItem(LOCAL_STORAGE_KEY)
    } catch (err) {
      console.error('Failed to migrate quick replies:', err)
      // 迁移失败不阻塞，下次重试
    }
  }

  // 从后端加载快捷回复
  async function loadQuickReplies(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const data = await listAdminChatQuickReplies()
      quickReplies.value = data.map((item) => ({
        id: item.id,
        title: item.title,
        content: item.content,
        custom: true,
      }))
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load quick replies'
      console.error('Failed to load quick replies:', err)
    } finally {
      loading.value = false
    }
  }

  // 创建快捷回复
  async function addQuickReply(title: string, content: string): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const created = await createAdminChatQuickReply({ title, content })
      quickReplies.value.push({
        id: created.id,
        title: created.title,
        content: created.content,
        custom: true,
      })
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to create quick reply'
      throw err
    } finally {
      loading.value = false
    }
  }

  // 更新快捷回复
  async function updateQuickReply(id: number, title: string, content: string): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const updated = await updateAdminChatQuickReply(id, { title, content })
      const index = quickReplies.value.findIndex((item) => item.id === id)
      if (index >= 0) {
        quickReplies.value.splice(index, 1, {
          id: updated.id,
          title: updated.title,
          content: updated.content,
          custom: true,
        })
      }
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to update quick reply'
      throw err
    } finally {
      loading.value = false
    }
  }

  // 删除快捷回复
  async function removeQuickReply(id: number): Promise<void> {
    loading.value = true
    error.value = null
    try {
      await deleteAdminChatQuickReply(id)
      quickReplies.value = quickReplies.value.filter((item) => item.id !== id)
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to delete quick reply'
      throw err
    } finally {
      loading.value = false
    }
  }

  // 重新排序快捷回复
  async function reorderQuickReplies(ids: number[]): Promise<void> {
    loading.value = true
    error.value = null
    try {
      await reorderAdminChatQuickReplies(ids)
      // 更新本地顺序
      const orderMap = new Map(ids.map((id, index) => [id, index]))
      quickReplies.value.sort((a, b) => {
        const aOrder = typeof a.id === 'number' ? orderMap.get(a.id) ?? 999 : 999
        const bOrder = typeof b.id === 'number' ? orderMap.get(b.id) ?? 999 : 999
        return aOrder - bOrder
      })
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to reorder quick replies'
      throw err
    } finally {
      loading.value = false
    }
  }

  async function importQuickReplies(items: Array<{ title: string; content: string }>): Promise<void> {
    loading.value = true
    error.value = null
    try {
      const imported = await importAdminChatQuickReplies(items)
      quickReplies.value = [
        ...imported.map((item) => ({ id: item.id, title: item.title, content: item.content, custom: true })),
        ...quickReplies.value,
      ]
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to import quick replies'
      throw err
    } finally {
      loading.value = false
    }
  }

  // 初始化：迁移旧数据 + 加载后端数据
  async function initialize(): Promise<void> {
    await migrateFromLocalStorage()
    await loadQuickReplies()
  }

  onMounted(() => {
    initialize()
  })

  return {
    quickReplies,
    loading,
    error,
    loadQuickReplies,
    addQuickReply,
    updateQuickReply,
    removeQuickReply,
    reorderQuickReplies,
    importQuickReplies,
    initialize,
  }
}
