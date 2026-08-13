<template>
  <form class="border-t border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900" @submit.prevent="submit">
    <transition name="support-panel">
      <div
        v-if="activePanel"
        class="mb-3 rounded-2xl border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-950/60"
        @click="closeQuickReplyMenu"
      >
        <div v-if="activePanel === 'tools'" class="flex gap-2 overflow-x-auto pb-1">
          <button
            v-for="tool in toolActions"
            :key="tool.id"
            type="button"
            class="inline-flex shrink-0 items-center gap-2 rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm font-medium text-gray-700 transition-colors hover:border-primary-300 hover:bg-primary-50 hover:text-primary-700 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-200 dark:hover:border-primary-700 dark:hover:bg-primary-900/20 dark:hover:text-primary-200"
            @click="insertSnippet(tool.template)"
          >
            <span class="text-base">{{ tool.icon }}</span>
            <span>{{ tool.label }}</span>
          </button>
        </div>

        <div v-else-if="activePanel === 'imageLibrary'" class="space-y-3">
          <div class="flex items-center justify-between gap-3">
            <div>
              <p class="text-sm font-medium text-gray-800 dark:text-dark-100">
                {{ t('supportChat.composer.imageLibrary') }}
              </p>
              <p class="text-xs text-gray-500 dark:text-dark-400">
                {{ t('supportChat.composer.imageLibraryHint') }}
              </p>
            </div>
            <button type="button" class="btn btn-secondary btn-sm" @click.stop="openLibraryImagePicker">
              {{ t('supportChat.composer.addLibraryImage') }}
            </button>
          </div>
          <input
            ref="libraryImageInputRef"
            type="file"
            class="sr-only"
            accept="image/png,image/jpeg,image/gif,image/webp"
            @change="handleLibraryImageInputChange"
          />
          <div v-if="imageLibrary.length" class="grid max-h-64 grid-cols-2 gap-3 overflow-y-auto sm:grid-cols-3 lg:grid-cols-4">
            <button
              v-for="image in imageLibrary"
              :key="image.id"
              type="button"
              class="group relative overflow-hidden rounded-xl border border-gray-200 bg-white text-left transition-colors hover:border-primary-300 hover:bg-primary-50 dark:border-dark-700 dark:bg-dark-800 dark:hover:border-primary-700 dark:hover:bg-primary-900/20"
              :title="image.name"
              @click.stop="sendLibraryImage(image)"
            >
              <span
                class="absolute right-1.5 top-1.5 z-10 hidden h-6 w-6 items-center justify-center rounded-full bg-red-600 text-xs font-bold text-white shadow-sm group-hover:inline-flex"
                :title="t('common.delete')"
                @click.stop="deleteLibraryImage(image)"
              >×</span>
              <img :src="image.url" :alt="image.name" class="h-24 w-full bg-gray-100 object-contain dark:bg-dark-900" />
              <div class="space-y-0.5 px-2 py-1.5">
                <p class="truncate text-xs font-medium text-gray-700 dark:text-dark-100">{{ image.name }}</p>
                <p class="truncate text-[11px] text-gray-500 dark:text-dark-400">{{ image.category }}</p>
              </div>
            </button>
          </div>
          <div v-else class="rounded-xl border border-dashed border-gray-300 px-4 py-6 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-dark-400">
            {{ t('supportChat.composer.imageLibraryEmpty') }}
          </div>
        </div>

        <div v-else-if="activePanel === 'stickers'" class="space-y-3">
          <div class="flex items-center justify-between gap-3">
            <div>
              <p class="text-sm font-medium text-gray-800 dark:text-dark-100">
                {{ t('supportChat.composer.stickers') }}
              </p>
              <p class="text-xs text-gray-500 dark:text-dark-400">
                {{ t('supportChat.composer.stickersHint') }}
              </p>
            </div>
            <button type="button" class="btn btn-secondary btn-sm" @click.stop="openStickerImagePicker">
              {{ t('supportChat.composer.addSticker') }}
            </button>
          </div>
          <input
            ref="stickerImageInputRef"
            type="file"
            class="sr-only"
            accept="image/png,image/jpeg,image/gif,image/webp"
            @change="handleStickerImageInputChange"
          />
          <div v-if="stickers.length" class="grid max-h-64 grid-cols-4 gap-2 overflow-y-auto sm:grid-cols-6 lg:grid-cols-8">
            <button
              v-for="sticker in stickers"
              :key="sticker.id"
              type="button"
              class="group relative flex h-24 flex-col items-center justify-center gap-1 rounded-xl border border-gray-200 bg-white px-2 text-center transition-colors hover:border-primary-300 hover:bg-primary-50 dark:border-dark-700 dark:bg-dark-800 dark:hover:border-primary-700 dark:hover:bg-primary-900/20"
              :title="sticker.name"
              @click.stop="sendSticker(sticker)"
            >
              <span
                v-if="sticker.url"
                class="absolute right-1 top-1 z-10 hidden h-5 w-5 items-center justify-center rounded-full bg-red-600 text-xs font-bold text-white shadow-sm group-hover:inline-flex"
                :title="t('common.delete')"
                @click.stop="deleteSticker(sticker)"
              >×</span>
              <img v-if="sticker.url" :src="sticker.url" :alt="sticker.name" class="h-14 w-14 object-contain" />
              <span v-else class="text-2xl leading-none">{{ sticker.emoji }}</span>
              <span class="max-w-full truncate text-[11px] text-gray-500 dark:text-dark-400">{{ sticker.name }}</span>
            </button>
          </div>
          <div v-else class="rounded-xl border border-dashed border-gray-300 px-4 py-6 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-dark-400">
            {{ t('supportChat.composer.stickersEmpty') }}
          </div>
        </div>

        <div v-else class="space-y-3">
          <div class="flex items-center gap-2 overflow-x-auto pb-2">
            <button
              type="button"
              class="inline-flex shrink-0 items-center gap-2 rounded-xl border px-3 py-2 text-sm font-medium transition-colors"
              :class="oneClickReplyEnabled ? 'border-primary-300 bg-primary-50 text-primary-700 dark:border-primary-700 dark:bg-primary-900/20 dark:text-primary-200' : 'border-gray-200 bg-white text-gray-700 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-200'"
              @click.stop="toggleOneClickReply"
            >
              <span class="inline-flex h-5 w-9 items-center rounded-full transition-colors" :class="oneClickReplyEnabled ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'">
                <span class="ml-0.5 h-4 w-4 rounded-full bg-white transition-transform" :class="oneClickReplyEnabled ? 'translate-x-4' : 'translate-x-0'"></span>
              </span>
              <span>{{ t('supportChat.composer.oneClickReply') }}</span>
            </button>

            <div class="flex gap-2 overflow-x-auto pb-1">
              <div
                v-for="reply in allQuickReplies"
                :key="reply.id"
                class="quick-reply-chip group relative inline-flex shrink-0"
                @contextmenu.prevent.stop="reply.custom && openQuickReplyMenu(reply.id, $event)"
                @pointerdown="startLongPress(reply, $event)"
                @pointerup="cancelLongPress"
                @pointerleave="cancelLongPress"
                @pointercancel="cancelLongPress"
              >
                <button
                  type="button"
                  class="inline-flex items-center rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm font-medium text-gray-700 transition-colors hover:border-primary-300 hover:bg-primary-50 hover:text-primary-700 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-200 dark:hover:border-primary-700 dark:hover:bg-primary-900/20 dark:hover:text-primary-200"
                  :title="reply.custom ? t('supportChat.composer.customReplyHint') : undefined"
                  @click.stop="handleQuickReply(reply)"
                  @keydown.f2.prevent.stop="reply.custom && startEditReply(reply)"
                  @keydown.shift.f10.prevent.stop="reply.custom && openQuickReplyMenu(reply.id, $event)"
                >
                  <span class="max-w-40 truncate">{{ reply.title }}</span>
                </button>
              </div>
              <button
                type="button"
                class="inline-flex shrink-0 items-center gap-2 rounded-xl border border-dashed border-primary-300 bg-primary-50 px-3 py-2 text-sm font-medium text-primary-700 transition-colors hover:bg-primary-100 dark:border-primary-700 dark:bg-primary-900/20 dark:text-primary-200"
                @click.stop="openCustomReplyEditor()"
              >
                <PlusMiniIcon />
                <span>{{ t('supportChat.composer.addCustomReply') }}</span>
              </button>
            </div>
          </div>
          <p v-if="customReplies.length" class="px-1 text-xs text-gray-500 dark:text-dark-400">
            {{ t('supportChat.composer.customReplyHint') }}
          </p>

          <div
            v-if="showReplyEditor"
            class="grid gap-3 rounded-xl border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800 lg:grid-cols-[220px_minmax(0,1fr)]"
            @click.stop
          >
            <input
              v-model="customReplyTitle"
              type="text"
              class="rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm text-gray-900 outline-none focus:border-primary-500 focus:ring-2 focus:ring-primary-500/20 dark:border-dark-700 dark:bg-dark-900 dark:text-white"
              :placeholder="t('supportChat.composer.customReplyTitle')"
            />
            <textarea
              v-model="customReplyContent"
              class="min-h-[88px] resize-y rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm text-gray-900 outline-none focus:border-primary-500 focus:ring-2 focus:ring-primary-500/20 dark:border-dark-700 dark:bg-dark-900 dark:text-white"
              :placeholder="t('supportChat.composer.customReplyHtml')"
            />
            <div class="lg:col-span-2 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div class="min-w-0 flex-1 rounded-lg bg-gray-50 p-3 text-sm text-gray-700 dark:bg-dark-900 dark:text-dark-200">
                <div class="mb-1 text-xs font-medium text-gray-500 dark:text-dark-400">
                  {{ t('supportChat.composer.htmlPreview') }}
                </div>
                <div class="support-chat-preview prose prose-sm max-w-none dark:prose-invert" v-html="customReplyPreview"></div>
              </div>
              <div class="flex shrink-0 gap-2">
                <button type="button" class="btn btn-secondary" @click="cancelReplyEdit">
                  {{ t('common.cancel') }}
                </button>
                <button type="button" class="btn btn-primary" :disabled="!canSaveCustomReply" @click="saveCustomReply">
                  {{ editingReplyId ? t('supportChat.composer.updateCustomReply') : t('supportChat.composer.saveCustomReply') }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </transition>

    <label class="sr-only" for="support-chat-content">{{ t('supportChat.inputLabel') }}</label>
    <div class="mx-auto flex w-full max-w-4xl flex-col gap-3">
      <div class="flex items-center justify-between gap-3">
        <input
          ref="imageInputRef"
          type="file"
          class="sr-only"
          accept="image/png,image/jpeg,image/gif,image/webp"
          @change="handleImageInputChange"
        />
        <div class="flex flex-wrap items-center gap-2">
          <button
            type="button"
            class="composer-tool-button"
            :title="t('supportChat.composer.sendImage')"
            :disabled="disabled || sending"
            @click="openImagePicker"
          >
            <ImageIcon />
            <span>{{ t('supportChat.composer.sendImage') }}</span>
          </button>
          <button
            v-if="showAssistantTools"
            type="button"
            class="composer-tool-button"
            :class="activePanel === 'imageLibrary' ? 'composer-tool-button-active' : ''"
            :title="t('supportChat.composer.imageLibrary')"
            @click="togglePanel('imageLibrary')"
          >
            <GalleryIcon />
            <span>{{ t('supportChat.composer.imageLibrary') }}</span>
          </button>
          <button
            v-if="showAssistantTools"
            type="button"
            class="composer-tool-button"
            :class="activePanel === 'replies' ? 'composer-tool-button-active' : ''"
            :title="t('supportChat.composer.quickReplies')"
            @click="togglePanel('replies')"
          >
            <MessageIcon />
            <span>{{ t('supportChat.composer.quickReplies') }}</span>
          </button>
          <button
            v-if="showAssistantTools"
            type="button"
            class="composer-tool-button"
            :class="activePanel === 'stickers' ? 'composer-tool-button-active' : ''"
            :title="t('supportChat.composer.stickers')"
            @click="togglePanel('stickers')"
          >
            <span class="text-sm">😊</span>
            <span>{{ t('supportChat.composer.stickers') }}</span>
          </button>
          <button
            v-if="showAssistantTools"
            type="button"
            class="composer-tool-button"
            :class="activePanel === 'tools' ? 'composer-tool-button-active' : ''"
            :title="t('supportChat.composer.moreActions')"
            @click="togglePanel('tools')"
          >
            <PlusIcon />
            <span>{{ t('supportChat.composer.moreActions') }}</span>
          </button>
        </div>
        <span class="shrink-0 text-xs text-gray-500 dark:text-dark-400">{{ draft.length }}/{{ maxLength }}</span>
      </div>

      <div
        class="support-composer-input"
        :class="pendingAttachment ? 'support-composer-input-has-attachment' : ''"
        @click="focusTextarea"
      >
        <textarea
          id="support-chat-content"
          ref="textareaRef"
          v-model="draft"
          class="support-composer-textarea"
          :class="pendingAttachment ? 'support-composer-textarea-with-attachment' : ''"
          :maxlength="maxLength"
          :placeholder="inputPlaceholder"
          :disabled="disabled || sending"
          @keydown.enter.exact.prevent="submit"
          @paste="handlePaste"
        />

        <div
          v-if="pendingAttachment"
          class="support-composer-attachment"
          @click.stop
        >
          <div v-if="pendingAttachment.type === 'imageFile'" class="support-composer-attachment-media">
            <img :src="pendingAttachment.previewUrl" :alt="pendingAttachment.name" class="h-full w-full object-contain" />
          </div>
          <div v-else-if="pendingAttachment.type === 'imageUrl'" class="support-composer-attachment-media">
            <img :src="pendingAttachment.url" :alt="pendingAttachment.name" class="h-full w-full object-contain" />
          </div>
          <div v-else class="support-composer-attachment-sticker">
            <img v-if="pendingAttachment.url" :src="pendingAttachment.url" :alt="pendingAttachment.name" class="h-16 w-16 object-contain" />
            <span v-else class="text-4xl leading-none">{{ pendingAttachment.emoji }}</span>
            <span class="mt-1 max-w-full truncate px-1 text-[11px] font-medium text-gray-500 dark:text-dark-400">{{ pendingAttachment.name }}</span>
          </div>
          <button
            type="button"
            class="support-composer-attachment-remove"
            :title="t('common.cancel')"
            @click="clearPendingAttachment"
          >
            ×
          </button>
        </div>
      </div>

      <div class="flex items-center justify-between gap-3">
        <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('supportChat.enterHint') }}</span>
        <button
          type="submit"
          class="btn btn-primary min-w-24"
          :disabled="disabled || sending || (!draft.trim() && !pendingAttachment)"
        >
          {{ sending ? t('common.submitting') : t('supportChat.send') }}
        </button>
      </div>
    </div>

    <Teleport to="body">
      <div
        v-if="quickReplyMenuReply"
        class="fixed z-[9999] w-44 overflow-hidden rounded-xl border border-gray-200 bg-white py-1 text-sm shadow-xl dark:border-dark-700 dark:bg-dark-800"
        :style="quickReplyMenuStyle"
        @click.stop
      >
        <div class="border-b border-gray-100 px-3 py-2 text-xs font-medium text-gray-500 dark:border-dark-700 dark:text-dark-400">
          <span class="block truncate">{{ quickReplyMenuReply.title }}</span>
        </div>
        <button type="button" class="quick-reply-menu-item" @click="startEditReply(quickReplyMenuReply)">
          <EditIcon />
          <span>{{ t('supportChat.composer.editReply') }}</span>
        </button>
        <button type="button" class="quick-reply-menu-item text-red-600 hover:bg-red-50 dark:text-red-300 dark:hover:bg-red-900/20" @click="deleteCustomReply(quickReplyMenuReply.id)">
          <TrashIcon />
          <span>{{ t('supportChat.composer.deleteReply') }}</span>
        </button>
      </div>
    </Teleport>
  </form>
</template>

<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { sanitizeChatHtml } from '@/features/support-chat/presentation/utils/sanitizeChatHtml'

interface QuickReply {
  id: string
  title: string
  content: string
  custom?: boolean
}

interface ToolAction {
  id: string
  label: string
  icon: string
  template: string
}

interface ImageLibraryItem {
  id: string
  name: string
  category: string
  url: string
}

interface StickerItem {
  id: string
  name: string
  group?: string
  url?: string
  emoji?: string
}

type PendingAttachment =
  | { type: 'imageFile'; file: File; previewUrl: string; name: string }
  | { type: 'imageUrl'; url: string; name: string }
  | { type: 'sticker'; id: string; name: string; url?: string; emoji?: string }

interface SubmitPayload {
  text: string
  attachment: PendingAttachment | null
}

const props = withDefaults(defineProps<{
  sending?: boolean
  disabled?: boolean
  maxLength?: number
  showAssistantTools?: boolean
  draftKey?: string
  clearNonce?: number
  imageLibrary?: ImageLibraryItem[]
  stickers?: StickerItem[]
}>(), {
  sending: false,
  disabled: false,
  maxLength: 10000,
  showAssistantTools: false,
  draftKey: 'default',
  clearNonce: 0,
  imageLibrary: () => [],
  stickers: () => [],
})

const emit = defineEmits<{
  submit: [content: string]
  submitRich: [payload: SubmitPayload]
  libraryImageAddSelected: [file: File]
  libraryImageDelete: [image: ImageLibraryItem]
  stickerAddSelected: [file: File]
  stickerDelete: [sticker: StickerItem]
}>()

const { t } = useI18n()
const draft = ref('')
const pendingAttachment = ref<PendingAttachment | null>(null)
const imageInputRef = ref<HTMLInputElement | null>(null)
const libraryImageInputRef = ref<HTMLInputElement | null>(null)
const stickerImageInputRef = ref<HTMLInputElement | null>(null)
const textareaRef = ref<HTMLTextAreaElement | null>(null)
const activePanel = ref<'tools' | 'replies' | 'imageLibrary' | 'stickers' | null>(null)
const oneClickReplyEnabled = ref(false)
const showReplyEditor = ref(false)
const editingReplyId = ref<string | null>(null)
const customReplyTitle = ref('')
const customReplyContent = ref('')
const customReplies = ref<QuickReply[]>([])
const openReplyMenuId = ref<string | null>(null)
const quickReplyMenuStyle = ref<Record<string, string>>({})
const customReplyStorageKey = 'support_chat_custom_replies_v1'
const oneClickReplyStorageKey = 'support_chat_one_click_reply_v1'
let longPressTimer: ReturnType<typeof setTimeout> | null = null
let suppressedReplyId: string | null = null
let loadingDraft = false

const PlusIcon = {
  render: () => h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.8' }, [
    h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', d: 'M12 5v14m7-7H5' }),
  ]),
}

const PlusMiniIcon = {
  render: () => h('svg', { class: 'h-4 w-4', fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.8' }, [
    h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', d: 'M12 5v14m7-7H5' }),
  ]),
}

const MessageIcon = {
  render: () => h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.8' }, [
    h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', d: 'M8.625 12a.375.375 0 11-.75 0 .375.375 0 01.75 0zm3.75 0a.375.375 0 11-.75 0 .375.375 0 01.75 0zm3.75 0a.375.375 0 11-.75 0 .375.375 0 01.75 0z' }),
    h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', d: 'M21 12c0 4.556-4.03 8.25-9 8.25a9.77 9.77 0 01-2.555-.337A5.972 5.972 0 015.41 21a4.48 4.48 0 00.978-2.025c.09-.457-.133-.901-.467-1.226C3.93 16.253 3 14.224 3 12c0-4.556 4.03-8.25 9-8.25s9 3.694 9 8.25z' }),
  ]),
}

const ImageIcon = {
  render: () => h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.8' }, [
    h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', d: 'M2.25 15.75l5.159-5.159a2.25 2.25 0 013.182 0l5.159 5.159m-1.5-1.5l1.409-1.409a2.25 2.25 0 013.182 0l2.909 2.909' }),
    h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', d: 'M3.75 6.75A2.25 2.25 0 016 4.5h12a2.25 2.25 0 012.25 2.25v10.5A2.25 2.25 0 0118 19.5H6a2.25 2.25 0 01-2.25-2.25V6.75z' }),
    h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', d: 'M14.25 8.25h.008v.008h-.008V8.25z' }),
  ]),
}

const GalleryIcon = {
  render: () => h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.8' }, [
    h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', d: 'M8.25 6.75h12M8.25 12h12m-12 5.25h12' }),
    h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', d: 'M3.75 6.75h.008v.008H3.75V6.75zm0 5.25h.008v.008H3.75V12zm0 5.25h.008v.008H3.75v-.008z' }),
  ]),
}

const EditIcon = {
  render: () => h('svg', { class: 'h-3.5 w-3.5', fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.8' }, [
    h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', d: 'M16.862 4.487l1.65-1.65a1.875 1.875 0 112.652 2.652L7.5 19.153l-4 1 1-4L16.862 4.487z' }),
  ]),
}

const TrashIcon = {
  render: () => h('svg', { class: 'h-3.5 w-3.5', fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.8' }, [
    h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', d: 'M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.684.107 1.026.163m-1.026-.163L18.16 19.673a2.25 2.25 0 01-2.245 2.077H8.085a2.25 2.25 0 01-2.245-2.077L4.772 5.79m14.5 0a48.108 48.108 0 00-3.478-.397m-10.85.563c.34-.053.683-.11 1.026-.166m0 0a48.11 48.11 0 013.478-.397m7.372 0v-.526A2.25 2.25 0 0012 2.25h-3.75A2.25 2.25 0 006 4.5v.526m7.372 0a48.11 48.11 0 00-7.372 0' }),
  ]),
}

const toolActions = computed<ToolAction[]>(() => [
  { id: 'order-info', label: t('supportChat.composer.toolOrderInfo'), icon: '🧾', template: t('supportChat.composer.toolOrderInfoTemplate') },
  { id: 'api-key', label: t('supportChat.composer.toolApiKey'), icon: '🔑', template: t('supportChat.composer.toolApiKeyTemplate') },
  { id: 'usage-check', label: t('supportChat.composer.toolUsageCheck'), icon: '📊', template: t('supportChat.composer.toolUsageCheckTemplate') },
])

const builtinReplies = computed<QuickReply[]>(() => [
  { id: 'hello', title: t('supportChat.composer.replyHello'), content: t('supportChat.composer.replyHelloContent') },
  { id: 'need-more-info', title: t('supportChat.composer.replyNeedMoreInfo'), content: t('supportChat.composer.replyNeedMoreInfoContent') },
  { id: 'resolved', title: t('supportChat.composer.replyResolved'), content: t('supportChat.composer.replyResolvedContent') },
])

const builtinStickers = computed<StickerItem[]>(() => [
  { id: 'romance', name: 'Romance', emoji: '💗' },
  { id: 'ok', name: 'OK', emoji: '👌' },
  { id: 'received', name: t('supportChat.composer.stickerReceived'), emoji: '🫡' },
  { id: 'thanks', name: t('supportChat.composer.stickerThanks'), emoji: '🙏' },
  { id: 'checking', name: t('supportChat.composer.stickerChecking'), emoji: '🔍' },
  { id: 'done', name: t('supportChat.composer.stickerDone'), emoji: '✅' },
  { id: 'smile', name: t('supportChat.composer.stickerSmile'), emoji: '😊' },
  { id: 'wait', name: t('supportChat.composer.stickerWait'), emoji: '⏳' },
  { id: 'paid', name: t('supportChat.composer.stickerPaid'), emoji: '💰' },
  { id: 'fixing', name: t('supportChat.composer.stickerFixing'), emoji: '🛠️' },
  { id: 'refresh', name: t('supportChat.composer.stickerRefresh'), emoji: '🔄' },
  { id: 'sorry', name: t('supportChat.composer.stickerSorry'), emoji: '🙇' },
  { id: 'celebrate', name: t('supportChat.composer.stickerCelebrate'), emoji: '🎉' },
  { id: 'question', name: t('supportChat.composer.stickerQuestion'), emoji: '❓' },
  { id: 'warning', name: t('supportChat.composer.stickerWarning'), emoji: '⚠️' },
  { id: 'rocket', name: t('supportChat.composer.stickerRocket'), emoji: '🚀' },
])

const allQuickReplies = computed(() => [...builtinReplies.value, ...customReplies.value])
const imageLibrary = computed(() => props.imageLibrary ?? [])
const stickers = computed(() => [...(props.stickers ?? []), ...builtinStickers.value])
const quickReplyMenuReply = computed(() => customReplies.value.find((reply) => reply.id === openReplyMenuId.value) ?? null)
const customReplyPreview = computed(() => sanitizeHtml(customReplyContent.value || t('supportChat.composer.emptyPreview')))
const canSaveCustomReply = computed(() => customReplyTitle.value.trim() !== '' && customReplyContent.value.trim() !== '')
const draftStorageKey = computed(() => `support_chat_draft_v1:${props.draftKey || 'default'}`)
const inputPlaceholder = computed(() => pendingAttachment.value ? '' : t('supportChat.inputPlaceholder'))

function sanitizeHtml(content: string): string {
  return sanitizeChatHtml(content)
}

function togglePanel(panel: 'tools' | 'replies' | 'imageLibrary' | 'stickers') {
  if (!props.showAssistantTools) return
  activePanel.value = activePanel.value === panel ? null : panel
  closeQuickReplyMenu()
  if (activePanel.value !== 'replies') {
    showReplyEditor.value = false
    editingReplyId.value = null
  }
}

function toggleOneClickReply() {
  if (!props.showAssistantTools) return
  oneClickReplyEnabled.value = !oneClickReplyEnabled.value
}

function insertSnippet(content: string) {
  const snippet = content.trim()
  if (!snippet) return
  draft.value = draft.value.trim() ? `${draft.value.trim()}\n${snippet}` : snippet
}

function replaceDraft(content: string) {
  draft.value = content.trim()
}

function sendContent(content: string) {
  const trimmed = content.trim()
  if (!trimmed || props.disabled || props.sending) return
  emit('submit', trimmed)
  activePanel.value = null
}

function openImagePicker() {
  if (props.disabled || props.sending) return
  imageInputRef.value?.click()
}

function submitImageFile(file: File | null | undefined) {
  if (!file || props.disabled || props.sending) return
  if (!file.type.startsWith('image/')) return
  setPendingAttachment({
    type: 'imageFile',
    file,
    previewUrl: URL.createObjectURL(file),
    name: file.name || t('supportChat.composer.imageAlt'),
  })
  activePanel.value = null
}

function handleImageInputChange(event: Event) {
  const input = event.target instanceof HTMLInputElement ? event.target : null
  submitImageFile(input?.files?.[0])
  if (input) input.value = ''
}

function handlePaste(event: ClipboardEvent) {
  const items = Array.from(event.clipboardData?.items ?? [])
  const imageItem = items.find((item) => item.kind === 'file' && item.type.startsWith('image/'))
  if (!imageItem) return
  const file = imageItem.getAsFile()
  if (!file) return
  event.preventDefault()
  submitImageFile(file)
}

function openLibraryImagePicker() {
  if (props.disabled || props.sending) return
  libraryImageInputRef.value?.click()
}

function handleLibraryImageInputChange(event: Event) {
  const input = event.target instanceof HTMLInputElement ? event.target : null
  const file = input?.files?.[0]
  if (file && file.type.startsWith('image/')) {
    emit('libraryImageAddSelected', file)
  }
  if (input) input.value = ''
}

function openStickerImagePicker() {
  if (props.disabled || props.sending) return
  stickerImageInputRef.value?.click()
}

function handleStickerImageInputChange(event: Event) {
  const input = event.target instanceof HTMLInputElement ? event.target : null
  const file = input?.files?.[0]
  if (file && file.type.startsWith('image/')) {
    emit('stickerAddSelected', file)
  }
  if (input) input.value = ''
}

function sendLibraryImage(image: ImageLibraryItem) {
  if (props.disabled || props.sending) return
  setPendingAttachment({
    type: 'imageUrl',
    url: image.url,
    name: image.name || t('supportChat.composer.imageAlt'),
  })
  activePanel.value = null
}

function deleteLibraryImage(image: ImageLibraryItem) {
  emit('libraryImageDelete', image)
}

function sendSticker(sticker: StickerItem) {
  if (props.disabled || props.sending) return
  setPendingAttachment({
    type: 'sticker',
    id: sticker.id,
    name: sticker.name,
    url: sticker.url,
    emoji: sticker.emoji,
  })
  activePanel.value = null
}

function deleteSticker(sticker: StickerItem) {
  if (!sticker.url) return
  emit('stickerDelete', sticker)
}

function setPendingAttachment(next: PendingAttachment) {
  clearPendingAttachment()
  pendingAttachment.value = next
  requestAnimationFrame(() => {
    textareaRef.value?.focus()
  })
}

function clearPendingAttachment() {
  if (pendingAttachment.value?.type === 'imageFile') {
    URL.revokeObjectURL(pendingAttachment.value.previewUrl)
  }
  pendingAttachment.value = null
  requestAnimationFrame(() => {
    textareaRef.value?.focus()
  })
}

function focusTextarea() {
  textareaRef.value?.focus()
}

function handleQuickReply(reply: QuickReply) {
  if (suppressedReplyId === reply.id) {
    suppressedReplyId = null
    return
  }
  closeQuickReplyMenu()
  if (oneClickReplyEnabled.value) {
    sendContent(reply.content)
    return
  }
  replaceDraft(reply.content)
}

function openQuickReplyMenu(id: string, event?: MouseEvent | PointerEvent | KeyboardEvent) {
  cancelLongPress()
  quickReplyMenuStyle.value = getQuickReplyMenuStyle(event)
  openReplyMenuId.value = id
}

function closeQuickReplyMenu() {
  openReplyMenuId.value = null
}

function getQuickReplyMenuStyle(event?: MouseEvent | PointerEvent | KeyboardEvent): Record<string, string> {
  const menuWidth = 176
  const menuHeight = 132
  const viewportPadding = 8
  const gap = 8
  let left = viewportPadding
  let top = viewportPadding

  if (event && 'clientX' in event && event.clientX > 0 && event.clientY > 0 && event.type === 'contextmenu') {
    left = event.clientX
    top = event.clientY
  } else {
    const target = event?.currentTarget
    const anchor = target instanceof Element ? target.getBoundingClientRect() : null
    if (anchor) {
      left = anchor.left + anchor.width - menuWidth
      top = anchor.top - menuHeight - gap
      if (top < viewportPadding) top = anchor.bottom + gap
    }
  }

  left = Math.min(Math.max(left, viewportPadding), window.innerWidth - menuWidth - viewportPadding)
  top = Math.min(Math.max(top, viewportPadding), window.innerHeight - menuHeight - viewportPadding)

  return {
    left: `${left}px`,
    top: `${top}px`,
  }
}

function startLongPress(reply: QuickReply, event: PointerEvent) {
  cancelLongPress()
  if (!reply.custom || event.button !== 0) return
  const nextMenuStyle = getQuickReplyMenuStyle(event)
  longPressTimer = setTimeout(() => {
    suppressedReplyId = reply.id
    quickReplyMenuStyle.value = nextMenuStyle
    openReplyMenuId.value = reply.id
  }, 450)
}

function cancelLongPress() {
  if (longPressTimer) {
    clearTimeout(longPressTimer)
    longPressTimer = null
  }
}

function openCustomReplyEditor() {
  closeQuickReplyMenu()
  editingReplyId.value = null
  customReplyTitle.value = ''
  customReplyContent.value = ''
  showReplyEditor.value = true
  activePanel.value = 'replies'
}

function startEditReply(reply: QuickReply) {
  closeQuickReplyMenu()
  editingReplyId.value = reply.id
  customReplyTitle.value = reply.title
  customReplyContent.value = reply.content
  showReplyEditor.value = true
  activePanel.value = 'replies'
}

function cancelReplyEdit() {
  showReplyEditor.value = false
  editingReplyId.value = null
  customReplyTitle.value = ''
  customReplyContent.value = ''
}

function saveCustomReply() {
  if (!canSaveCustomReply.value) return
  const nextReply = { id: editingReplyId.value || `custom-${Date.now()}`, title: customReplyTitle.value.trim(), content: customReplyContent.value.trim(), custom: true }
  const existingIndex = customReplies.value.findIndex((reply) => reply.id === nextReply.id)
  if (existingIndex >= 0) customReplies.value.splice(existingIndex, 1, nextReply)
  else customReplies.value.push(nextReply)
  cancelReplyEdit()
}

function deleteCustomReply(id: string) {
  closeQuickReplyMenu()
  customReplies.value = customReplies.value.filter((reply) => reply.id !== id)
  if (editingReplyId.value === id) cancelReplyEdit()
}

function loadCustomReplies() {
  try {
    const raw = localStorage.getItem(customReplyStorageKey)
    if (raw) {
      const parsed = JSON.parse(raw) as QuickReply[]
      if (Array.isArray(parsed)) {
        customReplies.value = parsed.filter((item) => item && typeof item.title === 'string' && typeof item.content === 'string').slice(0, 20)
      }
    }
    const quickReplyMode = localStorage.getItem(oneClickReplyStorageKey)
    if (quickReplyMode !== null) oneClickReplyEnabled.value = quickReplyMode === 'true'
  } catch {
    customReplies.value = []
  }
}

function persistCustomReplies() {
  localStorage.setItem(customReplyStorageKey, JSON.stringify(customReplies.value))
}

function persistOneClickReply() {
  localStorage.setItem(oneClickReplyStorageKey, String(oneClickReplyEnabled.value))
}

function loadDraft() {
  loadingDraft = true
  try {
    draft.value = localStorage.getItem(draftStorageKey.value) || ''
  } catch {
    draft.value = ''
  } finally {
    loadingDraft = false
  }
}

function persistDraft() {
  if (loadingDraft) return
  try {
    if (draft.value) {
      localStorage.setItem(draftStorageKey.value, draft.value)
    } else {
      localStorage.removeItem(draftStorageKey.value)
    }
  } catch {
    // localStorage can be unavailable in private/embedded contexts.
  }
}

function clearCurrentDraft() {
  draft.value = ''
  clearPendingAttachment()
  try {
    localStorage.removeItem(draftStorageKey.value)
  } catch {
    // ignore storage failures
  }
}

function submit() {
  if (props.disabled || props.sending) return
  const content = draft.value.trim()
  if (!content && !pendingAttachment.value) return
  if (pendingAttachment.value) {
    const attachment = pendingAttachment.value
    pendingAttachment.value = null
    emit('submitRich', { text: content, attachment })
    return
  }
  emit('submit', content)
}

function handleOutsideQuickReplyMenuClick() {
  closeQuickReplyMenu()
}

watch(customReplies, persistCustomReplies, { deep: true })
watch(oneClickReplyEnabled, persistOneClickReply)
watch(draft, persistDraft)
watch(() => props.draftKey, loadDraft, { immediate: true })
watch(() => props.clearNonce, clearCurrentDraft)

onMounted(() => {
  loadCustomReplies()
  document.addEventListener('click', handleOutsideQuickReplyMenuClick)
  window.addEventListener('resize', closeQuickReplyMenu)
  window.addEventListener('scroll', closeQuickReplyMenu, true)
})

onBeforeUnmount(() => {
  cancelLongPress()
  clearPendingAttachment()
  document.removeEventListener('click', handleOutsideQuickReplyMenuClick)
  window.removeEventListener('resize', closeQuickReplyMenu)
  window.removeEventListener('scroll', closeQuickReplyMenu, true)
})
</script>

<style scoped>
.composer-tool-button {
  @apply inline-flex h-9 items-center gap-1.5 rounded-xl border border-gray-200 bg-white px-3 text-xs font-medium text-gray-600 transition-colors hover:border-primary-300 hover:bg-primary-50 hover:text-primary-700 disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-300 dark:hover:border-primary-700 dark:hover:bg-primary-900/20 dark:hover:text-primary-200;
}

.composer-tool-button :deep(svg) {
  @apply h-4 w-4;
}

.composer-tool-button-active {
  @apply border-primary-300 bg-primary-50 text-primary-700 dark:border-primary-700 dark:bg-primary-900/30 dark:text-primary-200;
}

.quick-reply-menu-item {
  @apply flex w-full items-center gap-2 px-3 py-2 text-left text-gray-700 transition-colors hover:bg-gray-50 dark:text-dark-100 dark:hover:bg-dark-700;
}

.support-composer-input {
  @apply relative min-h-[132px] cursor-text overflow-hidden rounded-2xl border border-gray-200 bg-white transition-colors focus-within:border-primary-500 focus-within:ring-2 focus-within:ring-primary-500/20 dark:border-dark-700 dark:bg-dark-800;
}

.support-composer-input-has-attachment {
  @apply min-h-[168px];
}

.support-composer-textarea {
  @apply min-h-[132px] w-full resize-none border-0 bg-transparent px-4 py-3 text-sm text-gray-900 outline-none placeholder:text-gray-400 focus:ring-0 dark:text-white dark:placeholder:text-dark-400;
}

.support-composer-textarea-with-attachment {
  @apply pb-28;
}

.support-composer-attachment {
  @apply absolute bottom-3 left-3 flex h-24 w-24 items-center justify-center overflow-hidden rounded-xl border border-gray-200 bg-gray-50 shadow-sm dark:border-dark-700 dark:bg-dark-950;
}

.support-composer-attachment-media {
  @apply h-full w-full bg-white dark:bg-dark-950;
}

.support-composer-attachment-sticker {
  @apply flex h-full w-full flex-col items-center justify-center bg-white dark:bg-dark-950;
}

.support-composer-attachment-remove {
  @apply absolute right-1.5 top-1.5 inline-flex h-5 w-5 items-center justify-center rounded-full bg-gray-900/70 text-sm leading-none text-white shadow-sm transition-colors hover:bg-red-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-red-500/40;
}

.support-panel-enter-active,
.support-panel-leave-active {
  transition: opacity 0.16s ease, transform 0.16s ease;
}

.support-panel-enter-from,
.support-panel-leave-to {
  opacity: 0;
  transform: translateY(6px);
}
</style>
