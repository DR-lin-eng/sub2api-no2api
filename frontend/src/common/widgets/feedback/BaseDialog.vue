<template>
  <Teleport v-if="mode === 'dialog'" to="body">
    <Transition name="modal">
      <div
        v-if="show"
        class="modal-overlay"
        :style="zIndexStyle"
        :aria-labelledby="dialogId"
        role="dialog"
        aria-modal="true"
        @click.self="handleClose"
      >
        <!-- Modal panel -->
        <div
          ref="dialogRef"
          :class="['modal-content', widthClasses]"
          tabindex="-1"
          @click.stop
        >
          <!-- Header -->
          <div class="modal-header">
            <h3 :id="dialogId" class="modal-title">
              {{ title }}
            </h3>
            <button
              v-if="showCloseButton"
              @click="emit('close')"
              class="-mr-2 rounded-xl p-2 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 focus-visible:ring-offset-2 dark:text-dark-500 dark:hover:bg-dark-700 dark:hover:text-dark-300 dark:focus-visible:ring-offset-dark-900"
              aria-label="Close modal"
            >
              <Icon name="x" size="md" />
            </button>
          </div>

          <!-- Body -->
          <div class="modal-body">
            <slot></slot>
          </div>

          <!-- Footer -->
          <div v-if="$slots.footer" class="modal-footer">
            <slot name="footer"></slot>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
  <section v-else class="mx-auto w-full max-w-7xl bg-white dark:bg-dark-900">
    <header class="flex items-center justify-between border-b border-gray-200 px-5 py-4 dark:border-dark-700">
      <h1 class="text-lg font-semibold text-gray-900 dark:text-white">{{ title }}</h1>
      <button v-if="showCloseButton" type="button" class="rounded-lg p-2 text-gray-400 hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-800 dark:hover:text-white" aria-label="Close" @click="emit('close')">
        <Icon name="x" size="md" />
      </button>
    </header>
    <div class="p-5"><slot></slot></div>
    <footer v-if="$slots.footer" class="border-t border-gray-200 px-5 py-4 dark:border-dark-700"><slot name="footer"></slot></footer>
  </section>
</template>

<script setup lang="ts">
import { computed, watch, onMounted, onUnmounted, ref, nextTick } from 'vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import { acquireBodyScrollLock } from '@/core/utils/bodyScrollLock'

// 生成唯一ID以避免多个对话框时ID冲突
let dialogIdCounter = 0
const dialogId = `modal-title-${++dialogIdCounter}`

// 焦点管理
const dialogRef = ref<HTMLElement | null>(null)
let previousActiveElement: HTMLElement | null = null
let releaseBodyScrollLock: (() => void) | null = null

type DialogWidth = 'narrow' | 'normal' | 'wide' | 'extra-wide' | 'full'
type InitialFocus = 'first' | 'dialog'

interface Props {
  show: boolean
  title: string
  width?: DialogWidth
  closeOnEscape?: boolean
  closeOnClickOutside?: boolean
  showCloseButton?: boolean
  zIndex?: number
  initialFocus?: InitialFocus
  mode?: 'dialog' | 'page'
}

interface Emits {
  (e: 'close'): void
}

const props = withDefaults(defineProps<Props>(), {
  width: 'normal',
  closeOnEscape: true,
  closeOnClickOutside: false,
  showCloseButton: true,
  zIndex: 50,
  initialFocus: 'first',
  mode: 'dialog'
})

const emit = defineEmits<Emits>()

// Custom z-index style (overrides the default z-50 from CSS)
const zIndexStyle = computed(() => {
  return props.zIndex !== 50 ? { zIndex: props.zIndex } : undefined
})

const widthClasses = computed(() => {
  // Width guidance: narrow=confirm/short prompts, normal=standard forms,
  // wide=multi-section forms or rich content, extra-wide=analytics/tables,
  // full=full-screen or very dense layouts.
  const widths: Record<DialogWidth, string> = {
    narrow: 'max-w-md',
    normal: 'max-w-lg',
    wide: 'w-full sm:max-w-2xl md:max-w-3xl lg:max-w-4xl',
    'extra-wide': 'w-full sm:max-w-3xl md:max-w-4xl lg:max-w-5xl xl:max-w-6xl',
    full: 'w-full sm:max-w-4xl md:max-w-5xl lg:max-w-6xl xl:max-w-7xl'
  }
  return widths[props.width]
})

const handleClose = () => {
  if (props.closeOnClickOutside) {
    emit('close')
  }
}

const handleEscape = (event: KeyboardEvent) => {
  if (props.show && props.closeOnEscape && event.key === 'Escape') {
    emit('close')
  }
}

const syncBodyScrollLock = (locked: boolean) => {
  if (locked && !releaseBodyScrollLock) {
    releaseBodyScrollLock = acquireBodyScrollLock()
  } else if (!locked && releaseBodyScrollLock) {
    releaseBodyScrollLock()
    releaseBodyScrollLock = null
  }
}

// Prevent body scroll when modal is open and manage focus
watch(
  () => props.show,
  async (isOpen) => {
    if (isOpen && props.mode === 'dialog') {
      // 保存当前焦点元素
      previousActiveElement = document.activeElement as HTMLElement
      syncBodyScrollLock(true)

      // 等待DOM更新后设置焦点到对话框
      await nextTick()
      if (dialogRef.value) {
        if (props.initialFocus === 'dialog') {
          dialogRef.value.focus({ preventScroll: true })
          return
        }

        const firstFocusable = dialogRef.value.querySelector<HTMLElement>(
          'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
        )
        firstFocusable?.focus()
      }
    } else if (!isOpen || props.mode !== 'dialog') {
      syncBodyScrollLock(false)
      // 恢复之前的焦点
      if (previousActiveElement && typeof previousActiveElement.focus === 'function') {
        previousActiveElement.focus()
      }
      previousActiveElement = null
    }
  },
  { immediate: true }
)

onMounted(() => {
  document.addEventListener('keydown', handleEscape)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleEscape)
  syncBodyScrollLock(false)
})
</script>
