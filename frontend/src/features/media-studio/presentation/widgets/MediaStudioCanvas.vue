<template>
  <section class="flex min-h-[calc(100vh-7rem)] items-center justify-center px-5 py-16">
    <div class="w-full max-w-5xl">
      <h1 class="mb-8 text-center text-3xl font-semibold tracking-tight text-gray-950 dark:text-white">
        {{ t('mediaStudio.composer.greeting') }}
      </h1>

      <div class="relative mx-auto max-w-4xl">
        <div
          v-if="typeMenuOpen"
          class="absolute bottom-16 left-4 z-20 w-56 rounded-2xl border border-gray-200 bg-white p-2 shadow-xl shadow-gray-900/10 dark:border-dark-700 dark:bg-dark-900"
        >
          <p class="px-2 py-1.5 text-xs font-medium text-gray-400 dark:text-gray-500">{{ t('mediaStudio.composer.creationType') }}</p>
          <button
            v-for="mode in modes"
            :key="mode.id"
            type="button"
            class="flex w-full items-center justify-between rounded-xl px-3 py-2 text-sm transition-colors"
            :class="mode.id === selectedModeId ? 'bg-gray-100 text-gray-950 dark:bg-dark-800 dark:text-white' : 'text-gray-700 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-dark-800'"
            @click="selectMode(mode.id)"
          >
            <span class="flex items-center gap-2.5">
              <Icon :name="mode.iconName" size="sm" />
              <span>{{ t(`mediaStudio.modeItems.${mode.id}.title`) }}</span>
            </span>
            <Icon v-if="mode.id === selectedModeId" name="check" size="sm" />
          </button>
        </div>

        <div class="rounded-[2rem] border border-gray-200 bg-white p-4 shadow-[0_18px_60px_rgba(15,23,42,0.08)] dark:border-dark-700 dark:bg-dark-900 dark:shadow-black/20">
          <div class="min-h-36 px-1 py-1 text-lg leading-8 text-gray-400 dark:text-gray-500">
            {{ t('mediaStudio.composer.placeholderPrefix') }}
            <span class="inline-flex h-8 w-8 items-center justify-center rounded-full border border-gray-200 bg-gray-50 text-gray-600 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300">
              <Icon name="user" size="sm" />
            </span>
            {{ t('mediaStudio.composer.placeholderSuffix') }}
          </div>

          <div class="flex flex-wrap items-center gap-2 pt-3">
            <button
              type="button"
              class="composer-chip bg-gray-100 text-gray-900 dark:bg-dark-800 dark:text-white"
              :aria-expanded="typeMenuOpen"
              @click="typeMenuOpen = !typeMenuOpen"
            >
              <Icon :name="selectedMode.iconName" size="sm" />
              <span>{{ t(`mediaStudio.modeItems.${selectedMode.id}.title`) }}</span>
              <Icon :name="typeMenuOpen ? 'chevronUp' : 'chevronDown'" size="xs" class="text-gray-400" />
            </button>
            <button type="button" class="composer-chip">
              <Icon name="cube" size="sm" />
              <span>{{ t('mediaStudio.composer.model') }}</span>
            </button>
            <button type="button" class="composer-chip">
              <Icon name="grid" size="sm" />
              <span>1:1</span>
              <span>2K</span>
              <span>4</span>
            </button>
            <button type="button" class="composer-icon" :aria-label="t('mediaStudio.composer.textTool')">
              <Icon name="document" size="sm" />
            </button>
            <button type="button" class="composer-icon" :aria-label="t('mediaStudio.composer.mention')">
              <Icon name="user" size="sm" />
            </button>

            <div class="ml-auto flex items-center gap-3">
              <span class="inline-flex items-center gap-1.5 text-sm font-medium text-gray-500 dark:text-gray-400">
                <Icon name="sparkles" size="sm" />
                <span>3/{{ t('mediaStudio.composer.unit') }}</span>
              </span>
              <button type="button" class="flex h-10 w-10 items-center justify-center rounded-full bg-gray-200 text-white dark:bg-dark-700" :aria-label="t('mediaStudio.composer.send')" disabled>
                <Icon name="arrowUp" size="sm" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/common/widgets/icons/Icon.vue'
import type { MediaStudioMode, MediaStudioModeId } from '@/features/media-studio/presentation/composables/useMediaStudioPreview'

defineProps<{
  modes: MediaStudioMode[]
  selectedMode: MediaStudioMode
  selectedModeId: MediaStudioModeId
}>()

const emit = defineEmits<{
  select: [id: MediaStudioModeId]
}>()

const { t } = useI18n()
const typeMenuOpen = ref(false)

function selectMode(id: MediaStudioModeId) {
  emit('select', id)
  typeMenuOpen.value = false
}
</script>

<style scoped>
.composer-chip {
  @apply inline-flex h-10 items-center gap-2 rounded-xl border border-gray-200 bg-white px-3 text-sm font-medium text-gray-700 shadow-sm transition-colors hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-200 dark:hover:bg-dark-800;
}

.composer-icon {
  @apply inline-flex h-10 w-10 items-center justify-center rounded-xl border border-gray-200 bg-white text-gray-700 shadow-sm transition-colors hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-200 dark:hover:bg-dark-800;
}
</style>
