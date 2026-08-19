<template>
  <div class="mt-2 rounded-xl border border-dashed border-gray-300 bg-white/70 p-3 dark:border-dark-700 dark:bg-dark-900/70">
    <input
      ref="fileInput"
      type="file"
      accept="image/*"
      multiple
      class="hidden"
      @change="handleFileInput"
    />

    <button
      type="button"
      class="flex w-full items-center justify-center gap-2 rounded-xl border border-gray-200 bg-gray-100 px-3 py-3 text-sm text-gray-700 transition hover:border-gray-300 hover:bg-white dark:border-dark-700 dark:bg-dark-800 dark:text-gray-200 dark:hover:border-dark-500 dark:hover:bg-dark-700"
      :class="dragging ? 'border-primary-400 bg-primary-50 dark:bg-primary-500/10' : ''"
      @click="fileInput?.click()"
      @dragenter.prevent="dragging = true"
      @dragover.prevent="dragging = true"
      @dragleave.prevent="dragging = false"
      @drop.prevent="handleDrop"
      @paste="handlePaste"
    >
      <Icon name="upload" size="sm" />
      <span>{{ t('mediaStudio.composer.imageEdit.attachHint') }}</span>
      <span class="text-xs text-gray-400 dark:text-gray-500">
        {{ attachments.length }}/{{ MAX_ATTACHMENTS }}
      </span>
    </button>

    <div v-if="attachments.length > 0" class="mt-3 grid grid-cols-3 gap-2 sm:grid-cols-5">
      <div
        v-for="attachment in attachments"
        :key="attachment.id"
        class="group relative aspect-square overflow-hidden rounded-xl border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800"
      >
        <img :src="attachment.previewUrl" :alt="attachment.name" class="h-full w-full object-cover" />
        <button
          type="button"
          class="absolute right-1 top-1 rounded-full bg-black/60 p-1 text-white opacity-0 transition group-hover:opacity-100"
          :title="t('mediaStudio.composer.imageEdit.remove')"
          @click="removeAttachment(attachment.id)"
        >
          <Icon name="x" size="xs" />
        </button>
      </div>
    </div>

    <p v-if="errorMessage" class="mt-2 text-xs text-red-600 dark:text-red-300">{{ errorMessage }}</p>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/common/widgets/icons/Icon.vue'
import {
  addMediaStudioImageAttachments,
  MEDIA_STUDIO_MAX_IMAGE_ATTACHMENTS,
  revokeMediaStudioImageAttachments,
  type MediaStudioImageAttachment,
} from '@/features/media-studio/presentation/composables/useMediaStudioAttachments'

const props = defineProps<{ attachments: MediaStudioImageAttachment[] }>()
const emit = defineEmits<{ update: [attachments: MediaStudioImageAttachment[]] }>()
const { t } = useI18n()

const MAX_ATTACHMENTS = MEDIA_STUDIO_MAX_IMAGE_ATTACHMENTS
const fileInput = ref<HTMLInputElement | null>(null)
const dragging = ref(false)
const errorMessage = ref('')

function addFiles(files: File[]): void {
  const result = addMediaStudioImageAttachments(props.attachments, files)
  if (result.rejected.length > 0) {
    errorMessage.value = result.rejected[0]
  } else {
    errorMessage.value = ''
  }
  emit('update', result.attachments)
}

function handleFileInput(event: Event): void {
  const input = event.target as HTMLInputElement
  addFiles(Array.from(input.files || []))
  input.value = ''
}

function handleDrop(event: DragEvent): void {
  dragging.value = false
  addFiles(Array.from(event.dataTransfer?.files || []))
}

function handlePaste(event: ClipboardEvent): void {
  const files = Array.from(event.clipboardData?.items || [])
    .filter((item) => item.kind === 'file' && item.type.startsWith('image/'))
    .map((item) => item.getAsFile())
    .filter((file): file is File => Boolean(file))
  if (files.length > 0) {
    event.preventDefault()
    addFiles(files)
  }
}

function removeAttachment(id: string): void {
  const removed = props.attachments.find((attachment) => attachment.id === id)
  if (removed) URL.revokeObjectURL(removed.previewUrl)
  emit('update', props.attachments.filter((attachment) => attachment.id !== id))
}

onBeforeUnmount(() => {
  revokeMediaStudioImageAttachments(props.attachments)
})
</script>
