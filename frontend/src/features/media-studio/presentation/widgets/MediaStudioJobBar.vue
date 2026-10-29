<template>
  <section class="job-bar">
    <header class="job-bar-header">
      <div class="flex min-w-0 items-center gap-2 text-xs font-semibold text-gray-700 dark:text-gray-200">
        <Icon
          name="refresh"
          size="xs"
          :class="hasActive ? 'animate-spin text-primary-500' : 'text-gray-400'"
        />
        <span>{{ t('mediaStudio.jobs.title') }}</span>
        <span v-if="activeCount > 0" class="job-count-pill">
          {{ t('mediaStudio.jobs.activeCount', { count: activeCount }) }}
        </span>
      </div>
      <button
        type="button"
        class="text-gray-400 transition-colors hover:text-gray-600 dark:hover:text-gray-200"
        :aria-label="t('mediaStudio.jobs.toggle')"
        :aria-expanded="!collapsed"
        @click="collapsed = !collapsed"
      >
        <Icon :name="collapsed ? 'chevronDown' : 'chevronUp'" size="xs" />
      </button>
    </header>

    <div v-if="!collapsed" class="job-bar-list">
      <article v-for="job in visibleJobs" :key="job.messageId" class="job-row">
        <Icon :name="statusIcon(job.status)" size="sm" :class="statusIconClass(job.status)" />

        <div class="min-w-0 flex-1">
          <div class="flex items-baseline gap-2">
            <span class="truncate text-xs font-medium text-gray-800 dark:text-gray-100">{{ job.prompt }}</span>
            <span class="flex-shrink-0 text-[11px] text-gray-500 dark:text-gray-400">{{ statusLabel(job.status) }}</span>
          </div>

          <div v-if="isActive(job.status)" class="mt-1 h-1 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-dark-800">
            <div
              class="job-progress-fill"
              :class="job.progress === undefined ? 'job-progress-indeterminate' : ''"
              :style="{ width: `${progressOf(job)}%` }"
            ></div>
          </div>
          <p
            v-else-if="job.status === 'failed' && job.error"
            class="mt-0.5 truncate text-[11px] text-red-600 dark:text-red-300"
          >
            {{ errorLabel(job) }}
          </p>
        </div>

        <div class="flex flex-shrink-0 items-center gap-2 text-[11px] text-gray-500 dark:text-gray-400">
          <span>{{ t('mediaStudio.jobs.elapsed', { time: elapsedOf(job) }) }}</span>
          <span v-if="job.cost !== undefined">
            {{
              t(job.status === 'completed' ? 'mediaStudio.jobs.cost' : 'mediaStudio.jobs.costHeld', {
                amount: formatCost(job.cost, job.currency),
              })
            }}
          </span>
          <button
            v-if="job.status === 'failed'"
            type="button"
            class="job-action"
            @click="emit('retry', job.messageId)"
          >
            {{ t('mediaStudio.jobs.retry') }}
          </button>
          <button type="button" class="job-action" @click="emit('focus', job.messageId)">
            {{ t('mediaStudio.jobs.focus') }}
          </button>
        </div>
      </article>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/common/widgets/icons/Icon.vue'
import type { MediaStudioJobView } from '@/features/media-studio/presentation/composables/useMediaStudioJobs'

const props = defineProps<{
  jobs: MediaStudioJobView[]
}>()

const emit = defineEmits<{
  retry: [messageId: string]
  focus: [messageId: string]
}>()

const { t } = useI18n()
const collapsed = ref(false)
const nowTick = ref(Date.now())
let timer: number | undefined

const activeCount = computed(() => props.jobs.filter(job => isActive(job.status)).length)
const hasActive = computed(() => activeCount.value > 0)
// 只渲染最近 5 条，任务条是辅助视图，完整历史仍在消息流里。
const visibleJobs = computed(() => props.jobs.slice(0, 5))

function isActive(status: MediaStudioJobView['status']): boolean {
  return status === 'queued' || status === 'processing'
}

function statusLabel(status: MediaStudioJobView['status']): string {
  switch (status) {
    case 'queued':
      return t('mediaStudio.jobs.status.queued')
    case 'processing':
      return t('mediaStudio.jobs.status.processing')
    case 'failed':
      return t('mediaStudio.jobs.status.failed')
    default:
      return t('mediaStudio.jobs.status.completed')
  }
}

function statusIcon(status: MediaStudioJobView['status']): 'clock' | 'refresh' | 'checkCircle' | 'exclamationCircle' {
  switch (status) {
    case 'queued':
      return 'clock'
    case 'processing':
      return 'refresh'
    case 'failed':
      return 'exclamationCircle'
    default:
      return 'checkCircle'
  }
}

function statusIconClass(status: MediaStudioJobView['status']): string {
  if (status === 'completed') return 'flex-shrink-0 text-emerald-500'
  if (status === 'failed') return 'flex-shrink-0 text-red-500'
  if (status === 'processing') return 'flex-shrink-0 animate-spin text-primary-500'
  return 'flex-shrink-0 text-gray-400'
}

function errorLabel(job: MediaStudioJobView): string {
  const code = job.errorCode
  if (!code) return job.error || t('mediaStudio.jobs.errorCodes.unknown')
  switch (code) {
    case 'insufficient_quota':
      return t('mediaStudio.jobs.errorCodes.insufficient_quota')
    case 'unauthorized':
      return t('mediaStudio.jobs.errorCodes.unauthorized')
    case 'rate_limited':
      return t('mediaStudio.jobs.errorCodes.rate_limited')
    case 'content_policy':
      return t('mediaStudio.jobs.errorCodes.content_policy')
    case 'timeout':
      return t('mediaStudio.jobs.errorCodes.timeout')
    case 'upstream':
      return t('mediaStudio.jobs.errorCodes.upstream')
    case 'network':
      return t('mediaStudio.jobs.errorCodes.network')
    default:
      return job.error || t('mediaStudio.jobs.errorCodes.unknown')
  }
}

function progressOf(job: MediaStudioJobView): number {
  const value = job.progress
  if (typeof value !== 'number' || !Number.isFinite(value)) return 100
  return Math.min(100, Math.max(4, Math.round(value)))
}

function elapsedOf(job: MediaStudioJobView): string {
  const end = typeof job.completedAt === 'number' ? job.completedAt : nowTick.value
  const totalSeconds = Math.max(0, Math.round((end - job.createdAt) / 1000))
  if (totalSeconds < 60) return `${totalSeconds}s`
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  return `${minutes}m ${String(seconds).padStart(2, '0')}s`
}

function formatCost(amount: number, currency?: string): string {
  const value = Number.isFinite(amount) ? amount : 0
  const code = (currency || 'USD').toUpperCase()
  const symbol = code === 'USD' ? '$' : `${code} `
  return `${symbol}${value.toFixed(4).replace(/0+$/, '').replace(/\.$/, '')}`
}

// 仅在存在进行中任务时走动计时器，页面空闲时不产生额外负担。
watch(hasActive, (active) => {
  if (timer !== undefined) {
    window.clearInterval(timer)
    timer = undefined
  }
  if (!active) return
  nowTick.value = Date.now()
  timer = window.setInterval(() => { nowTick.value = Date.now() }, 1000)
}, { immediate: true })

onUnmounted(() => {
  if (timer !== undefined) window.clearInterval(timer)
})
</script>

<style scoped>
.job-bar {
  @apply overflow-hidden rounded-2xl border border-gray-200 bg-white/95 shadow-sm backdrop-blur dark:border-dark-700 dark:bg-dark-900/95;
}

.job-bar-header {
  @apply flex items-center justify-between gap-3 border-b border-gray-100 px-3 py-2 dark:border-dark-800;
}

.job-count-pill {
  @apply rounded-full bg-primary-500/10 px-2 py-0.5 text-[11px] font-medium text-primary-600 dark:text-primary-300;
}

.job-bar-list {
  @apply max-h-56 divide-y divide-gray-100 overflow-y-auto dark:divide-dark-800;
}

.job-row {
  @apply flex items-center gap-3 px-3 py-2;
}

.job-progress-fill {
  @apply h-full rounded-full bg-primary-500 transition-[width] duration-500;
}

.job-progress-indeterminate {
  animation: job-progress-slide 1.4s ease-in-out infinite;
}

.job-action {
  @apply text-[11px] font-semibold underline underline-offset-4 transition-colors hover:text-gray-900 dark:hover:text-white;
}

@keyframes job-progress-slide {
  0% {
    transform: translateX(-100%);
    width: 45%;
  }
  50% {
    width: 65%;
  }
  100% {
    transform: translateX(220%);
    width: 45%;
  }
}
</style>
