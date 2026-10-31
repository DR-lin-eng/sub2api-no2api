<template>
  <div class="session-list">
    <p class="session-list__title">{{ t('mediaStudio.session.sessions') }}</p>
    <p v-if="sessions.length === 0" class="session-list__empty">
      {{ t('mediaStudio.session.sessionsEmpty') }}
    </p>
    <ul v-else class="session-list__items">
      <li v-for="session in sessions" :key="session.id" class="session-list__row">
        <button
          type="button"
          class="session-list__item"
          :class="{ 'is-active': session.id === activeId }"
          :title="session.title || t('mediaStudio.session.untitled')"
          @click="emit('select', session.id)"
        >
          <span class="session-list__name">{{ session.title || t('mediaStudio.session.untitled') }}</span>
          <span class="session-list__count">{{ t('mediaStudio.session.messageCount', { count: session.messageCount }) }}</span>
        </button>
        <button
          v-if="session.id !== activeId"
          type="button"
          class="session-list__remove"
          :aria-label="t('mediaStudio.session.deleteConversation')"
          :title="t('mediaStudio.session.deleteConversation')"
          @click="remove(session.id)"
        >
          <Icon name="trash" size="xs" />
        </button>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/common/widgets/icons/Icon.vue'
import type { MediaStudioSessionSummary } from '@/features/media-studio/presentation/composables/useMediaStudioController'

defineProps<{
  sessions: MediaStudioSessionSummary[]
  activeId: string
}>()

const emit = defineEmits<{
  select: [id: string]
  remove: [id: string]
}>()

const { t } = useI18n()

// 删除历史对话不可撤销，先做一次确认再交给上层回收素材缓存。
function remove(id: string) {
  if (!window.confirm(t('mediaStudio.session.deleteConfirm'))) return
  emit('remove', id)
}
</script>

<style scoped>
.session-list {
  @apply flex w-full flex-col;
}

.session-list__title {
  @apply px-2 pb-1.5 text-xs font-medium text-gray-500 dark:text-gray-400;
}

.session-list__empty {
  @apply px-2 pb-2 text-sm text-gray-500 dark:text-gray-400;
}

.session-list__items {
  @apply max-h-72 space-y-1 overflow-y-auto;
}

.session-list__row {
  @apply flex items-center gap-1;
}

.session-list__item {
  @apply flex min-w-0 flex-1 items-center justify-between gap-2 rounded-xl border border-transparent px-2.5 py-2 text-sm text-gray-700 transition-colors;
  @apply hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-800;
}

.session-list__item.is-active {
  @apply border-primary-500 bg-primary-50 text-primary-600;
  @apply dark:border-primary-400 dark:bg-primary-500/15 dark:text-primary-300;
}

.session-list__name {
  @apply min-w-0 flex-1 truncate text-left;
}

.session-list__count {
  @apply shrink-0 text-[11px] text-gray-400 dark:text-gray-500;
}

.session-list__item.is-active .session-list__count {
  @apply text-primary-500 dark:text-primary-300;
}

.session-list__remove {
  @apply inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-gray-400 transition-colors;
  @apply hover:bg-red-50 hover:text-red-600 dark:text-gray-500 dark:hover:bg-red-500/10 dark:hover:text-red-300;
}
</style>
