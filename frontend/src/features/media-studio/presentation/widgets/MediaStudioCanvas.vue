<template>
  <section
    class="relative flex min-h-[calc(100vh-7rem)] flex-col bg-transparent"
    @dragenter.prevent="handlePageDragEnter"
    @dragover.prevent="handlePageDragOver"
    @dragleave.prevent="handlePageDragLeave"
    @drop.prevent="handlePageDrop"
  >
    <div
      v-if="draggingPage"
      class="pointer-events-none fixed inset-0 z-50 flex items-center justify-center border-2 border-dashed border-primary-400 bg-primary-500/10 p-6 backdrop-blur-[1px]"
    >
      <div class="rounded-2xl border border-primary-300 bg-white/95 px-5 py-4 text-sm font-medium text-primary-700 shadow-xl dark:border-primary-500/40 dark:bg-dark-900/95 dark:text-primary-200">
        <Icon name="upload" size="sm" class="mr-2 inline-block" />
        {{ t('mediaStudio.composer.upload.dropHint') }}
      </div>
    </div>
    <div v-if="hasMessages" class="mx-auto flex w-full max-w-5xl items-center justify-between gap-3 px-1 pb-4">
      <div>
        <h1 class="text-lg font-semibold tracking-tight text-gray-950 dark:text-white">{{ t('mediaStudio.title') }}</h1>
        <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('mediaStudio.session.localHint') }}</p>
      </div>
      <button type="button" class="session-action" @click="emit('clear')">
        <Icon name="trash" size="sm" />
        {{ t('mediaStudio.session.clear') }}
      </button>
    </div>

    <div
      v-if="hasMessages"
      class="mx-auto flex w-full max-w-5xl flex-1 flex-col gap-5 overflow-hidden px-1 pb-5"
    >
      <article
        v-for="message in conversation.messages"
        :key="message.id"
        class="group flex"
        :class="message.role === 'user' ? 'justify-end' : 'justify-start'"
      >
        <div
          class="relative max-w-[min(760px,92%)] rounded-[1.35rem] border px-4 py-3 shadow-sm"
          :class="message.role === 'user'
            ? 'border-gray-300 bg-gray-100 text-gray-900 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-100'
            : 'border-gray-200 bg-white/88 text-gray-900 dark:border-dark-700 dark:bg-dark-900/86 dark:text-gray-100'"
        >
          <button
            type="button"
            class="message-select-button"
            :class="[
              message.role === 'user' ? '-left-2' : '-right-2',
              selectedMessageIDs.includes(message.id) ? 'message-select-button-active' : '',
            ]"
            :aria-label="t('mediaStudio.session.selectMessage')"
            :aria-pressed="selectedMessageIDs.includes(message.id)"
            @click="selectMessage(message.id)"
          >
            <Icon v-if="selectedMessageIDs.includes(message.id)" name="check" size="xs" />
            <span v-else class="h-1.5 w-1.5 rounded-full bg-current opacity-40"></span>
          </button>
          <div class="flex items-center justify-between gap-4">
            <span class="text-xs font-medium opacity-70">
              {{ message.role === 'user' ? t('mediaStudio.session.you') : t('mediaStudio.session.studio') }}
            </span>
            <span class="text-[11px] opacity-50">{{ formatTime(message.createdAt) }}</span>
          </div>
          <p class="mt-2 whitespace-pre-wrap text-sm leading-6">{{ message.prompt }}</p>
          <div v-if="message.role === 'user' && message.inputImages?.length" class="mt-3 flex flex-wrap gap-2">
            <img
              v-for="image in message.inputImages"
              :key="image.id"
              :src="image.src"
              :alt="image.name"
              class="h-16 w-16 rounded-xl border border-black/10 object-cover dark:border-white/15"
              loading="lazy"
            />
          </div>
          <div v-if="message.role === 'user' && message.inputReferences?.length" class="mt-3 flex flex-wrap gap-2">
            <div
              v-for="reference in message.inputReferences"
              :key="reference.id"
              class="h-16 w-16 overflow-hidden rounded-xl border border-black/10 bg-gray-50 dark:border-white/15 dark:bg-dark-800"
              :title="reference.name"
            >
              <img
                v-if="reference.kind === 'image'"
                :src="reference.src"
                :alt="reference.name"
                class="h-full w-full object-cover"
                loading="lazy"
              />
              <video
                v-else-if="reference.kind === 'video'"
                :src="reference.src"
                class="h-full w-full object-cover"
                muted
                playsinline
                preload="metadata"
              ></video>
              <div v-else class="flex h-full w-full flex-col items-center justify-center gap-1 px-1 text-center">
                <Icon name="play" size="sm" class="text-gray-400" />
                <span class="line-clamp-2 text-[10px] leading-tight text-gray-500 dark:text-gray-400">{{ reference.name }}</span>
              </div>
            </div>
          </div>

          <div v-if="message.role === 'assistant'" class="mt-3">
            <div class="mb-3 flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
              <span class="meta-pill">{{ message.model || model }}</span>
              <template v-if="message.mode === 'image'">
                <span class="meta-pill">{{ message.imageResolution || imageResolution }}</span>
                <span class="meta-pill">{{ message.imageAspectRatio || imageAspectRatio }}</span>
                <span class="meta-pill">{{ t('mediaStudio.composer.countValue', { count: message.count || 1 }) }}</span>
              </template>
              <template v-else>
                <span class="meta-pill">{{ message.resolution || resolution }}</span>
                <span class="meta-pill">{{ message.aspectRatio || videoAspectRatio }}</span>
                <span class="meta-pill">{{ t('mediaStudio.composer.durationValue', { count: message.duration || duration }) }}</span>
              </template>
              <span v-if="message.taskId && message.status !== 'completed'" class="meta-pill font-mono">{{ message.taskId }}</span>
              <span v-if="message.status === 'completed' && message.completedAt" class="meta-pill">
                {{ t('mediaStudio.session.elapsed', { time: formatElapsed(message) }) }}
              </span>
            </div>

            <div
              v-if="message.mode === 'image' && (message.status === 'processing' || message.status === 'queued')"
              class="grid gap-3"
              :class="imageGridClass(message)"
            >
              <button
                v-for="image in message.images || []"
                :key="image.id"
                type="button"
                class="group mx-auto w-full cursor-zoom-in overflow-hidden rounded-2xl border border-gray-200 bg-gray-50 text-left dark:border-dark-700 dark:bg-dark-800"
                :style="imageFrameStyle(message)"
                :aria-label="t('mediaStudio.session.enlargeImage')"
                @click="openImagePreview(image)"
              >
                <img
                  :src="image.src"
                  :alt="image.revisedPrompt || message.prompt"
                  referrerpolicy="no-referrer"
                  class="w-full object-cover transition duration-200 group-hover:scale-[1.02]"
                  :style="imageFrameStyle(message)"
                />
              </button>
              <div
                v-for="index in Math.max((message.count || 1) - (message.images?.length || 0), 0)"
                :key="`pending-${index}`"
                class="generating-frame mx-auto w-full"
                :style="imageFrameStyle(message)"
              >
                <div class="generating-glow"></div>
              </div>
            </div>

            <div
              v-else-if="(message.status === 'processing' || message.status === 'queued') && message.mode === 'image'"
              class="grid gap-3"
              :class="imageGridClass(message)"
            >
              <div
                v-for="index in (message.count || 1)"
                :key="index"
                class="generating-frame mx-auto w-full"
                :style="imageFrameStyle(message)"
              >
                <div class="generating-glow"></div>
              </div>
            </div>

            <div
              v-else-if="message.status === 'processing' || message.status === 'queued'"
              class="flex justify-center"
            >
              <div class="generating-frame relative w-full" :style="videoFrameStyle(message)">
                <div class="generating-glow"></div>
                <div class="absolute inset-x-4 bottom-4 z-10">
                  <div class="h-2 overflow-hidden rounded-full bg-white/50 dark:bg-dark-700">
                    <div
                      class="generating-progress h-full rounded-full transition-[width] duration-500"
                      :style="{ width: `${videoProgress(message)}%` }"
                    ></div>
                  </div>
                </div>
              </div>
            </div>

            <div
              v-else-if="message.status === 'completed' && message.mode === 'image' && message.images?.length"
              class="grid gap-3"
              :class="imageGridClass(message)"
            >
              <div
                v-for="image in message.images"
                :key="image.id"
                class="image-preview-card group relative mx-auto w-full overflow-hidden rounded-2xl border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800"
                :style="imageFrameStyle(message)"
              >
                <button
                  type="button"
                  class="block w-full cursor-zoom-in text-left"
                  :aria-label="t('mediaStudio.session.enlargeImage')"
                  @click="openImagePreview(image)"
                >
                  <img
                    :src="image.src"
                    :alt="image.revisedPrompt || message.prompt"
                    referrerpolicy="no-referrer"
                    class="w-full object-cover transition duration-200 group-hover:scale-[1.02]"
                    :style="imageFrameStyle(message)"
                  />
                </button>
                <button
                  type="button"
                  class="image-edit-action"
                  :aria-label="t('mediaStudio.session.editImage')"
                  @click.stop="startImageEdit(image)"
                >
                  <Icon name="edit" size="sm" />
                </button>
              </div>
            </div>

            <div
              v-else-if="message.status === 'completed' && message.mode === 'video' && message.video?.src"
              class="mx-auto flex w-full justify-center overflow-hidden rounded-2xl border border-gray-200 bg-black dark:border-dark-700"
              :style="videoFrameStyle(message)"
            >
              <video
                :src="message.video.src"
                :type="message.video.mimeType"
                class="h-full w-full object-contain"
                controls
                playsinline
                preload="metadata"
              ></video>
            </div>

            <div
              v-else-if="message.status === 'failed'"
              class="rounded-2xl border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-200"
            >
              <div class="flex items-start gap-2">
                <Icon name="exclamationCircle" size="sm" class="mt-0.5 flex-shrink-0" />
                <p class="min-w-0 flex-1">{{ message.error || t('mediaStudio.session.failed') }}</p>
                <button type="button" class="text-xs font-semibold underline underline-offset-4" @click="emit('retry', message)">
                  {{ t('mediaStudio.session.retry') }}
                </button>
              </div>
            </div>

            <div
              v-else-if="message.status === 'completed'"
              class="rounded-2xl border border-gray-200 bg-gray-50 px-3 py-2 text-sm text-gray-600 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300"
            >
              {{ message.mode === 'video' ? t('mediaStudio.session.noVideoResult') : t('mediaStudio.session.noImageResult') }}
            </div>
          </div>
        </div>
      </article>
    </div>

    <div
      class="mx-auto w-full max-w-5xl px-1"
      :class="hasMessages ? 'sticky bottom-0 mt-auto bg-gradient-to-t from-gray-50 via-gray-50/95 to-transparent pb-1 pt-5 dark:from-dark-950 dark:via-dark-950/95' : 'flex flex-1 items-center'"
    >
      <div class="w-full">
        <h2 v-if="!hasMessages" class="mb-8 text-center text-3xl font-semibold tracking-tight text-gray-950 dark:text-white">
          {{ t('mediaStudio.composer.greeting') }}
        </h2>

        <div v-if="hasMessages" class="mx-auto mb-2 flex max-w-4xl items-center justify-end gap-2">
          <template v-if="selectionMode">
            <span class="mr-auto text-xs text-gray-500 dark:text-gray-400">
              {{ t('mediaStudio.session.selectedCount', { count: selectedMessageIDs.length }) }}
            </span>
            <button type="button" class="session-action" @click="toggleSelectAll">
              <Icon name="check" size="sm" />
              {{ allMessagesSelected ? t('mediaStudio.session.deselectAll') : t('mediaStudio.session.selectAll') }}
            </button>
            <button
              type="button"
              class="session-action text-red-600 dark:text-red-300"
              :disabled="selectedMessageIDs.length === 0"
              @click="deleteSelected"
            >
              <Icon name="trash" size="sm" />
              {{ t('mediaStudio.session.deleteSelected') }}
            </button>
            <button type="button" class="icon-action" :aria-label="t('mediaStudio.session.cancelSelect')" @click="exitSelectionMode">
              <Icon name="x" size="sm" />
            </button>
          </template>
        </div>

        <div class="relative mx-auto max-w-4xl">
          <div
            v-if="typeMenuOpen"
            ref="typeMenuRef"
            class="absolute bottom-16 left-3 z-20 w-56 rounded-2xl border border-gray-200 bg-white p-1.5 shadow-xl shadow-gray-900/10 dark:border-dark-700 dark:bg-dark-900"
          >
            <button
              v-for="mode in modes"
              :key="mode.id"
              type="button"
              class="flex w-full items-center justify-between rounded-xl px-3 py-2 text-sm transition-colors"
              :class="modeButtonClass(mode)"
              @click="selectMode(mode.id)"
            >
              <span class="flex items-center gap-2.5">
                <Icon :name="mode.iconName" size="sm" />
                <span>{{ t(`mediaStudio.modeItems.${mode.id}.title`) }}</span>
              </span>
              <span v-if="!mode.available" class="text-xs text-gray-400">{{ t('mediaStudio.modeItems.disabled') }}</span>
              <Icon v-else-if="mode.id === selectedModeId" name="check" size="sm" />
            </button>
          </div>

          <div class="rounded-[1.65rem] border border-gray-200 bg-white/92 p-3 shadow-[0_16px_48px_rgba(15,23,42,0.07)] backdrop-blur dark:border-dark-700 dark:bg-dark-900/90 dark:shadow-black/20">
            <textarea
              ref="promptInputRef"
              :value="prompt"
              rows="3"
              class="min-h-24 w-full resize-none rounded-[1.1rem] border-0 bg-transparent px-2 py-2 text-base leading-7 text-gray-900 outline-none placeholder:text-gray-400 dark:text-white dark:placeholder:text-gray-500"
              :placeholder="selectedModeId === 'video' ? t('mediaStudio.composer.videoPlaceholder') : t('mediaStudio.composer.placeholder')"
              @input="emit('update:prompt', ($event.target as HTMLTextAreaElement).value)"
              @keydown.meta.enter.prevent="emit('submit')"
              @keydown.ctrl.enter.prevent="emit('submit')"
            />

            <div v-if="imageAttachments.length > 0" class="mt-2 grid grid-cols-3 gap-2 sm:grid-cols-5">
              <div
                v-for="attachment in imageAttachments"
                :key="attachment.id"
                class="group relative aspect-square overflow-hidden rounded-xl border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800"
              >
                <img :src="attachment.previewUrl" :alt="attachment.name" class="h-full w-full object-cover" />
                <button
                  type="button"
                  class="absolute right-1 top-1 rounded-full bg-black/60 p-1 text-white opacity-0 transition group-hover:opacity-100 focus-visible:opacity-100"
                  :title="t('mediaStudio.composer.imageEdit.remove')"
                  @click="emit('removeImageAttachment', attachment.id)"
                >
                  <Icon name="x" size="xs" />
                </button>
              </div>
            </div>
            <div v-if="selectedModeId === 'video' && referenceAttachments.length > 0" class="mt-2 grid grid-cols-3 gap-2 sm:grid-cols-5">
              <div
                v-for="attachment in referenceAttachments"
                :key="attachment.id"
                class="group relative aspect-square overflow-hidden rounded-xl border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800"
              >
                <img
                  v-if="attachment.kind === 'image'"
                  :src="attachment.previewUrl"
                  :alt="attachment.name"
                  class="h-full w-full object-cover"
                />
                <video
                  v-else-if="attachment.kind === 'video'"
                  :src="attachment.previewUrl"
                  class="h-full w-full object-cover"
                  muted
                  playsinline
                ></video>
                <div v-else class="flex h-full w-full flex-col items-center justify-center gap-1 px-2 text-center">
                  <Icon name="play" size="sm" class="text-gray-400" />
                  <span class="line-clamp-2 text-[10px] leading-tight text-gray-500 dark:text-gray-400">{{ attachment.name }}</span>
                </div>

                <div v-if="attachment.status === 'uploading'" class="absolute inset-0 flex items-center justify-center bg-black/50 text-white">
                  <Icon name="refresh" size="sm" class="animate-spin" />
                </div>
                <div
                  v-else-if="attachment.status === 'failed'"
                  class="absolute inset-0 flex flex-col items-center justify-center gap-1 bg-black/60 px-1 text-center"
                  :title="attachment.error"
                >
                  <Icon name="exclamationCircle" size="sm" class="text-red-300" />
                  <button
                    type="button"
                    class="text-[10px] font-medium text-white underline underline-offset-2"
                    @click="emit('retryReferenceUpload', attachment.id)"
                  >
                    {{ t('mediaStudio.composer.upload.retry') }}
                  </button>
                </div>

                <button
                  type="button"
                  class="absolute right-1 top-1 rounded-full bg-black/60 p-1 text-white opacity-0 transition group-hover:opacity-100 focus-visible:opacity-100"
                  :title="t('mediaStudio.composer.imageEdit.remove')"
                  @click="emit('removeReferenceAttachment', attachment.id)"
                >
                  <Icon name="x" size="xs" />
                </button>
              </div>
            </div>
            <p v-if="attachmentErrorText" class="mt-2 text-xs text-red-600 dark:text-red-300">{{ attachmentErrorText }}</p>

            <div v-if="groupLoadError || modelLoadError || submitError" class="mb-2 rounded-xl border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-200">
              {{ submitError || groupLoadError || modelLoadError }}
            </div>

            <div class="flex flex-wrap items-center gap-2">
              <input
                ref="fileInput"
                type="file"
                :accept="uploadAccept"
                multiple
                class="hidden"
                @change="handleFileInput"
              />

              <div class="relative">
                <button
                  type="button"
                  class="icon-action disabled:cursor-not-allowed disabled:opacity-40"
                  :aria-label="uploadHint"
                  :title="uploadHint"
                  :disabled="!canUpload"
                  @click="fileInput?.click()"
                >
                  <Icon name="upload" size="sm" />
                </button>

                <span
                  v-if="uploadBadge"
                  class="absolute -right-2 -top-2 inline-flex min-w-5 items-center justify-center rounded-full border border-primary-200 bg-white px-1.5 py-0.5 text-[10px] font-medium leading-none text-primary-700 shadow dark:border-primary-400/40 dark:bg-dark-900 dark:text-primary-300"
                >
                  {{ uploadBadge }}
                </span>
              </div>

              <button
                type="button"
                ref="typeMenuTriggerRef"
                class="composer-chip bg-gray-100 text-gray-900 dark:bg-dark-800 dark:text-white"
                :aria-expanded="typeMenuOpen"
                @click="typeMenuOpen = !typeMenuOpen"
              >
                <Icon :name="selectedMode.iconName" size="sm" />
                <span>{{ t(`mediaStudio.modeItems.${selectedMode.id}.title`) }}</span>
                <Icon :name="typeMenuOpen ? 'chevronUp' : 'chevronDown'" size="xs" class="text-gray-400" />
              </button>

              <button
                type="button"
                ref="settingsMenuTriggerRef"
                class="composer-chip bg-gray-100 text-gray-900 dark:bg-dark-800 dark:text-white"
                :aria-expanded="settingsMenuOpen"
                @click="toggleSettingsMenu"
              >
                <Icon name="cog" size="sm" />
                <span>{{ t('mediaStudio.composer.advancedSettings') }}</span>
                <Icon :name="settingsMenuOpen ? 'chevronUp' : 'chevronDown'" size="xs" class="text-gray-400" />
              </button>

              <button
                type="button"
                class="ml-auto flex h-10 w-10 items-center justify-center rounded-full transition"
                :class="canSubmit ? 'bg-gray-950 text-white hover:bg-gray-800 dark:bg-white dark:text-gray-950 dark:hover:bg-gray-200' : 'bg-gray-200 text-gray-400 dark:bg-dark-700 dark:text-gray-500'"
                :aria-label="t('mediaStudio.composer.send')"
                :disabled="!canSubmit"
                @click="emit('submit')"
              >
                <Icon :name="submitting ? 'refresh' : 'arrowUp'" size="sm" :class="submitting ? 'animate-spin' : ''" />
              </button>
            </div>

            <div
              v-if="settingsMenuOpen"
              ref="settingsMenuRef"
              class="absolute bottom-16 left-0 z-20 max-h-[min(72vh,36rem)] w-[min(94vw,44rem)] overflow-y-auto rounded-2xl border border-gray-200 bg-white p-4 shadow-xl shadow-gray-900/10 dark:border-dark-700 dark:bg-dark-900 sm:p-5"
            >
              <div class="grid gap-5 md:grid-cols-2 md:gap-0">
                <section class="space-y-2.5 md:pr-5">
                  <p class="section-title">
                    {{ t('mediaStudio.composer.resourceSettings') }}
                  </p>

                  <label class="param-field">
                    <span class="param-label">{{ t('mediaStudio.composer.group') }}</span>
                    <select
                      :value="selectedGroupId"
                      class="param-control"
                      :disabled="loadingGroups"
                      @change="emit('update:selectedGroupId', Number(($event.target as HTMLSelectElement).value))"
                    >
                      <option :value="0">{{ loadingGroups ? t('mediaStudio.composer.loadingGroups') : t('mediaStudio.composer.selectGroup') }}</option>
                      <option v-for="group in groupOptions" :key="group.group_id" :value="group.group_id">
                        {{ group.group_name }} · {{ group.platform }}
                      </option>
                    </select>
                  </label>

                  <label class="param-field">
                    <span class="param-label">{{ t('mediaStudio.composer.model') }}</span>
                    <select
                      v-if="modelSelectionLocked"
                      :value="model"
                      class="param-control"
                      :disabled="loadingModels || modelOptions.length === 0"
                      @change="emit('update:model', ($event.target as HTMLSelectElement).value)"
                    >
                      <option value="">{{ loadingModels ? t('mediaStudio.composer.loadingModels') : t('mediaStudio.composer.model') }}</option>
                      <option v-for="option in modelOptions" :key="option.id" :value="option.id">{{ option.id }}</option>
                    </select>
                    <template v-else>
                      <input
                        :value="model"
                        list="media-studio-models"
                        class="param-control"
                        :placeholder="loadingModels ? t('mediaStudio.composer.loadingModels') : t('mediaStudio.composer.model')"
                        @input="emit('update:model', ($event.target as HTMLInputElement).value)"
                      />
                      <datalist id="media-studio-models">
                        <option v-for="option in modelOptions" :key="option.id" :value="option.id" />
                      </datalist>
                    </template>
                  </label>
                </section>

                <section class="space-y-2.5 md:border-l md:border-gray-100 md:pl-5 dark:md:border-dark-800">
                  <p class="section-title">
                    {{ selectedModeId === 'image' ? t('mediaStudio.composer.imageSettings') : t('mediaStudio.composer.videoSettings') }}
                  </p>

                  <div v-if="selectedModeId === 'image'" class="grid grid-cols-2 gap-x-3 gap-y-2.5">
                    <ModelAttributeFieldControl
                      v-for="field in attributeFields"
                      :key="`attr:${field.key}`"
                      :field="field"
                      :model-value="attributeValues[field.key]"
                      @update:model-value="value => emit('update:attribute', { key: field.key, value })"
                    />

                    <label v-if="!hiddenParameterControls.includes('image.size')" class="param-field">
                      <span class="param-label">{{ t('mediaStudio.composer.resolution') }}</span>
                      <select
                        :value="imageResolution"
                        class="param-control"
                        @change="handleImageResolutionSelect"
                      >
                        <option v-for="option in imageResolutionOptions" :key="option" :value="option">{{ option }}</option>
                      </select>
                    </label>

                    <label v-if="!hiddenParameterControls.includes('image.size')" class="param-field">
                      <span class="param-label">{{ t('mediaStudio.composer.aspectRatio') }}</span>
                      <select
                        :value="imageAspectRatio"
                        class="param-control"
                        @change="handleAspectRatioSelect"
                      >
                        <option v-for="option in imageAspectRatioOptions" :key="option" :value="option">{{ option }}</option>
                        <option v-for="option in customImageAspectRatios" :key="`custom:${option}`" :value="`custom:${option}`">
                          {{ option }}
                        </option>
                        <option value="__custom__">{{ t('mediaStudio.composer.customAspectRatio.option') }}</option>
                      </select>
                    </label>

                    <label
                      v-if="!hiddenParameterControls.includes('image.quality') && imageQualityOptions.length > 0"
                      class="param-field"
                    >
                      <span class="param-label">{{ t('mediaStudio.composer.quality') }}</span>
                      <select
                        :value="quality"
                        class="param-control"
                        @change="emit('update:quality', ($event.target as HTMLSelectElement).value)"
                      >
                        <option v-for="option in imageQualityOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
                      </select>
                    </label>

                    <label v-if="!hiddenParameterControls.includes('image.count')" class="param-field">
                      <span class="param-label">{{ t('mediaStudio.composer.count') }}</span>
                      <select
                        :value="count"
                        class="param-control"
                        @change="emit('update:count', Number(($event.target as HTMLSelectElement).value))"
                      >
                        <option v-for="option in countOptions" :key="option" :value="option">{{ option }}</option>
                      </select>
                    </label>
                  </div>

                  <div v-else class="grid grid-cols-2 gap-x-3 gap-y-2.5">
                    <ModelAttributeFieldControl
                      v-for="field in attributeFields"
                      :key="`attr:${field.key}`"
                      :field="field"
                      :model-value="attributeValues[field.key]"
                      @update:model-value="value => emit('update:attribute', { key: field.key, value })"
                    />

                    <label v-if="!hiddenParameterControls.includes('video.resolution')" class="param-field">
                      <span class="param-label">{{ t('mediaStudio.composer.resolution') }}</span>
                      <select
                        :value="resolution"
                        class="param-control"
                        @change="emit('update:resolution', ($event.target as HTMLSelectElement).value as MediaStudioVideoResolution)"
                      >
                        <option v-for="option in resolutionOptions" :key="option" :value="option">{{ option }}</option>
                      </select>
                    </label>

                    <label v-if="!hiddenParameterControls.includes('video.aspectRatio')" class="param-field">
                      <span class="param-label">{{ t('mediaStudio.composer.aspectRatio') }}</span>
                      <select
                        :value="videoAspectRatio"
                        class="param-control"
                        @change="emit('update:videoAspectRatio', ($event.target as HTMLSelectElement).value as MediaStudioVideoAspectRatio)"
                      >
                        <option v-for="option in videoAspectRatioOptions" :key="option" :value="option">{{ option }}</option>
                      </select>
                    </label>

                    <label v-if="!hiddenParameterControls.includes('video.duration')" class="param-field">
                      <span class="param-label">{{ t('mediaStudio.composer.duration') }}</span>
                      <select
                        :value="duration"
                        class="param-control"
                        @change="emit('update:duration', Number(($event.target as HTMLSelectElement).value))"
                      >
                        <option v-for="option in durationOptions" :key="option" :value="option">{{ option }}s</option>
                      </select>
                    </label>
                  </div>
                </section>
              </div>
            </div>

            <div v-if="!loadingGroups && groupOptions.length === 0" class="mt-2 flex items-center gap-2 px-1 text-xs text-gray-500 dark:text-gray-400">
              <Icon name="infoCircle" size="xs" />
              <span>{{ groupLoadError || t('mediaStudio.composer.noGroups') }}</span>
              <button type="button" class="font-medium underline underline-offset-4" @click="emit('reloadGroups')">{{ t('mediaStudio.composer.reload') }}</button>
            </div>
            <div v-else-if="modelOptions.length === 0 && !loadingModels" class="mt-2 flex items-center gap-2 px-1 text-xs text-gray-500 dark:text-gray-400">
              <Icon name="infoCircle" size="xs" />
              <span>{{ t('mediaStudio.composer.manualModelHint') }}</span>
              <button type="button" class="font-medium underline underline-offset-4" @click="emit('reloadModels')">{{ t('mediaStudio.composer.reload') }}</button>
            </div>
          </div>

          <p v-if="!hasMessages" class="mx-auto mt-4 max-w-2xl text-center text-xs leading-5 text-gray-400 dark:text-gray-500">
            {{ t('mediaStudio.composer.shortHint') }}
          </p>
        </div>
      </div>
    </div>

    <Teleport to="body">
      <MediaStudioCustomResolutionDialog
        :visible="customAspectRatioDialogOpen"
        @close="customAspectRatioDialogOpen = false"
        @save="handleCustomResolutionSave"
      />

      <div
        v-if="selectedImage"
        class="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm sm:p-8"
        role="dialog"
        aria-modal="true"
        :aria-label="t('mediaStudio.session.enlargeImage')"
        @click.self="closeImagePreview"
      >
        <button
          type="button"
          class="absolute right-4 top-4 z-10 inline-flex h-10 w-10 items-center justify-center rounded-full bg-white/10 text-white transition hover:bg-white/20 focus:outline-none focus:ring-2 focus:ring-white/70"
          :aria-label="t('common.close')"
          @click="closeImagePreview"
        >
          <Icon name="x" size="md" />
        </button>
        <figure class="flex max-h-full max-w-full flex-col items-center gap-3" @click.stop>
          <img
            :src="selectedImage.url || selectedImage.src"
            :alt="selectedImage.revisedPrompt || t('mediaStudio.session.enlargeImage')"
            referrerpolicy="no-referrer"
            class="max-h-[calc(100vh-5rem)] max-w-[calc(100vw-2rem)] rounded-xl object-contain shadow-2xl sm:max-h-[calc(100vh-7rem)] sm:max-w-[calc(100vw-4rem)]"
          />
          <figcaption
            v-if="selectedImage.revisedPrompt"
            class="max-w-3xl text-center text-xs leading-5 text-white/80"
          >
            {{ selectedImage.revisedPrompt }}
          </figcaption>
        </figure>
      </div>
    </Teleport>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/common/widgets/icons/Icon.vue'
import MediaStudioCustomResolutionDialog from '@/features/media-studio/presentation/widgets/MediaStudioCustomResolutionDialog.vue'
import {
  mediaStudioKindEnabled,
  mediaStudioUploadAccept,
} from '@/features/media-studio/data/datasources/mediaStudioDatasource'
import type {
  MediaStudioGroupOption,
  MediaStudioParameterControl,
  MediaStudioResolvedLimits,
  MediaStudioVideoAspectRatio,
  MediaStudioVideoResolution,
  ModelAttributeField,
  ModelAttributeValues,
} from '@/features/media-studio/data/datasources/mediaStudioDatasource'
import type { MediaStudioModelOption, MediaStudioParameterOption } from '@/features/media-studio/presentation/composables/useMediaStudioController'
import ModelAttributeFieldControl from '@/common/widgets/model-attributes/ModelAttributeField.vue'
import type {
  MediaStudioAttachmentRejection,
  MediaStudioImageAttachment,
  MediaStudioImageAttachmentLimits,
  MediaStudioReferenceAttachment,
  MediaStudioReferenceLimits,
} from '@/features/media-studio/presentation/composables/useMediaStudioAttachments'
import type { MediaStudioMode, MediaStudioModeId } from '@/features/media-studio/presentation/composables/useMediaStudioPreview'
import type {
  MediaStudioConversation,
  MediaStudioGeneratedImagePreview,
  MediaStudioImageAspectRatio,
  MediaStudioImageResolution,
  MediaStudioMessage,
} from '@/features/media-studio/presentation/composables/useMediaStudioController'

const props = defineProps<{
  modes: MediaStudioMode[]
  selectedMode: MediaStudioMode
  selectedModeId: MediaStudioModeId
  prompt: string
  selectedGroupId: number
  model: string
  modelSelectionLocked: boolean
  imageResolution: MediaStudioImageResolution
  imageAspectRatio: MediaStudioImageAspectRatio
  customImageAspectRatios: string[]
  quality: string
  count: number
  resolution: MediaStudioVideoResolution
  videoAspectRatio: MediaStudioVideoAspectRatio
  duration: number
  durationOptions: number[]
  imageQualityOptions: MediaStudioParameterOption[]
  groupOptions: MediaStudioGroupOption[]
  loadingGroups: boolean
  groupLoadError: string
  imageAttachments: MediaStudioImageAttachment[]
  imageAttachmentLimits: MediaStudioImageAttachmentLimits
  referenceAttachments: MediaStudioReferenceAttachment[]
  referenceLimits: MediaStudioReferenceLimits
  modelLimits: MediaStudioResolvedLimits
  storageAvailable: boolean
  attachmentRejection: MediaStudioAttachmentRejection | null
  modelOptions: MediaStudioModelOption[]
  attributeFields: ModelAttributeField[]
  attributeValues: ModelAttributeValues
  hiddenParameterControls: MediaStudioParameterControl[]
  loadingModels: boolean
  modelLoadError: string
  conversation: MediaStudioConversation
  hasMessages: boolean
  canSubmit: boolean
  submitting: boolean
  submitError: string
}>()

const emit = defineEmits<{
  'update:prompt': [value: string]
  'update:selectedGroupId': [value: number]
  'update:model': [value: string]
  'update:imageResolution': [value: MediaStudioImageResolution]
  'update:imageAspectRatio': [value: MediaStudioImageAspectRatio]
  addCustomImageAspectRatio: [value: string]
  'update:quality': [value: string]
  'update:count': [value: number]
  'update:resolution': [value: MediaStudioVideoResolution]
  'update:videoAspectRatio': [value: MediaStudioVideoAspectRatio]
  'update:duration': [value: number]
  'update:attribute': [payload: { key: string; value: string | number | boolean | undefined }]
  selectMode: [id: MediaStudioModeId]
  reloadGroups: []
  addImageAttachments: [files: File[]]
  removeImageAttachment: [id: string]
  addReferenceAttachments: [files: File[]]
  removeReferenceAttachment: [id: string]
  retryReferenceUpload: [id: string]
  reloadModels: []
  submit: []
  retry: [message: MediaStudioMessage]
  clear: []
  delete: [messageIDs: string[]]
  editImage: [image: MediaStudioGeneratedImagePreview]
}>()

const { t, locale } = useI18n()
const typeMenuOpen = ref(false)
const typeMenuRef = ref<HTMLElement | null>(null)
const typeMenuTriggerRef = ref<HTMLButtonElement | null>(null)
const settingsMenuOpen = ref(false)
const settingsMenuRef = ref<HTMLElement | null>(null)
const settingsMenuTriggerRef = ref<HTMLButtonElement | null>(null)
const selectedImage = ref<MediaStudioGeneratedImagePreview | null>(null)
const promptInputRef = ref<HTMLTextAreaElement | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)
const draggingPage = ref(false)
let dragDepth = 0
const customAspectRatioDialogOpen = ref(false)
const selectionMode = ref(false)
const selectedMessageIDs = ref<string[]>([])

const allMessagesSelected = computed(() => (
  props.conversation.messages.length > 0 &&
  props.conversation.messages.every(message => selectedMessageIDs.value.includes(message.id))
))

const countOptions = [1, 2, 3, 4]
const imageResolutionOptions: MediaStudioImageResolution[] = ['1K', '2K', '4K']
const imageAspectRatioOptions: MediaStudioImageAspectRatio[] = ['1:1', '3:2', '2:3', '4:3', '3:4', '16:9', '9:16']
const resolutionOptions: MediaStudioVideoResolution[] = ['480p', '720p', '1080p']
const videoAspectRatioOptions: MediaStudioVideoAspectRatio[] = ['16:9', '9:16', '1:1', '4:3', '3:4']

const uploadAccept = computed(() => (
  props.selectedModeId === 'image' ? 'image/*' : mediaStudioUploadAccept(props.modelLimits)
))
// 图片模式只关心图片是否可用；视频模式只要还有任意一种参考素材可上传就放开入口。
const modeUploadEnabled = computed(() => (
  props.selectedModeId === 'image'
    ? mediaStudioKindEnabled(props.modelLimits, 'image')
    : props.referenceLimits.enabled.length > 0
))
const canUpload = computed(() => props.storageAvailable && modeUploadEnabled.value)
const uploadHint = computed(() => {
  if (!props.storageAvailable) return t('mediaStudio.composer.upload.storageUnavailable')
  if (!props.modelLimits.allowUpload) return t('mediaStudio.composer.upload.disabled')
  if (!modeUploadEnabled.value) return t('mediaStudio.composer.upload.noTypes')
  return t('mediaStudio.composer.upload.addHint')
})
const uploadBadge = computed(() => {
  if (!canUpload.value) return ''
  if (props.selectedModeId === 'image') {
    return `${props.imageAttachments.length}/${props.imageAttachmentLimits.maxFiles}`
  }
  const capacity = props.referenceLimits.enabled
    .reduce((total, kind) => total + props.referenceLimits.maxFiles[kind], 0)
  return capacity > 0 ? `${props.referenceAttachments.length}/${capacity}` : ''
})
const attachmentErrorText = computed(() => {
  const rejection = props.attachmentRejection
  if (!rejection) return ''
  return t(`mediaStudio.composer.upload.rejections.${rejection.reason}`, {
    name: rejection.name,
    limit: rejection.limit,
  })
})

function selectMode(id: MediaStudioModeId) {
  const mode = props.modes.find(item => item.id === id)
  if (!mode?.available) return
  emit('selectMode', id)
  typeMenuOpen.value = false
  settingsMenuOpen.value = false
}

function modeButtonClass(mode: MediaStudioMode) {
  if (!mode.available) return 'cursor-not-allowed text-gray-400 dark:text-gray-600'
  if (mode.id === props.selectedModeId) return 'bg-gray-100 text-gray-950 dark:bg-dark-800 dark:text-white'
  return 'text-gray-700 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-dark-800'
}

function toggleSettingsMenu() {
  settingsMenuOpen.value = !settingsMenuOpen.value
  if (settingsMenuOpen.value) typeMenuOpen.value = false
}

function openImagePreview(image: MediaStudioGeneratedImagePreview) {
  selectedImage.value = image
}

function closeImagePreview() {
  selectedImage.value = null
}

function startImageEdit(image: MediaStudioGeneratedImagePreview) {
  emit('editImage', image)
  void nextTick(() => promptInputRef.value?.focus())
}

function addFiles(files: File[]) {
  if (!canUpload.value || files.length === 0) return
  if (props.selectedModeId === 'image') {
    emit('addImageAttachments', files)
    return
  }
  emit('addReferenceAttachments', files)
}

function handleFileInput(event: Event) {
  const input = event.target as HTMLInputElement
  addFiles(Array.from(input.files || []))
  input.value = ''
}

function hasDraggedFiles(event: DragEvent): boolean {
  return Array.from(event.dataTransfer?.types || []).includes('Files')
}

function handlePageDragEnter(event: DragEvent) {
  if (!canUpload.value || !hasDraggedFiles(event)) return
  dragDepth += 1
  draggingPage.value = true
}

function handlePageDragOver(event: DragEvent) {
  if (canUpload.value && hasDraggedFiles(event)) draggingPage.value = true
}

function handlePageDragLeave(event: DragEvent) {
  if (!hasDraggedFiles(event)) return
  dragDepth = Math.max(0, dragDepth - 1)
  if (dragDepth === 0) draggingPage.value = false
}

function handlePageDrop(event: DragEvent) {
  dragDepth = 0
  draggingPage.value = false
  if (!canUpload.value) return
  addFiles(Array.from(event.dataTransfer?.files || []))
}

function handlePaste(event: ClipboardEvent) {
  // 剪贴板只可能是图片，视频模式不走粘贴。
  if (props.selectedModeId !== 'image' || !canUpload.value) return
  const files = Array.from(event.clipboardData?.items || [])
    .filter(item => item.kind === 'file' && item.type.startsWith('image/'))
    .map(item => item.getAsFile())
    .filter((file): file is File => Boolean(file))
  if (files.length === 0) return
  event.preventDefault()
  addFiles(files)
}

function toggleMessageSelection(messageID: string) {
  selectedMessageIDs.value = selectedMessageIDs.value.includes(messageID)
    ? selectedMessageIDs.value.filter(id => id !== messageID)
    : [...selectedMessageIDs.value, messageID]
}

function selectMessage(messageID: string) {
  selectionMode.value = true
  toggleMessageSelection(messageID)
}

function toggleSelectAll() {
  selectedMessageIDs.value = allMessagesSelected.value
    ? []
    : props.conversation.messages.map(message => message.id)
}

function exitSelectionMode() {
  selectionMode.value = false
  selectedMessageIDs.value = []
}

function deleteSelected() {
  if (selectedMessageIDs.value.length === 0) return
  emit('delete', [...selectedMessageIDs.value])
  exitSelectionMode()
}

function handleImageResolutionSelect(event: Event) {
  const value = (event.target as HTMLSelectElement).value
  emit('update:imageResolution', value as MediaStudioImageResolution)
}

function handleAspectRatioSelect(event: Event) {
  const value = (event.target as HTMLSelectElement).value
  if (value === '__custom__') {
    customAspectRatioDialogOpen.value = true
    return
  }
  if (value.startsWith('custom:')) {
    emit('update:imageAspectRatio', value as MediaStudioImageAspectRatio)
    return
  }
  emit('update:imageAspectRatio', value as MediaStudioImageAspectRatio)
}

function handleCustomResolutionSave(value: string) {
  emit('addCustomImageAspectRatio', value)
  customAspectRatioDialogOpen.value = false
}

function formatTime(value: number): string {
  try {
    return new Intl.DateTimeFormat(locale.value, {
      hour: '2-digit',
      minute: '2-digit',
    }).format(new Date(value))
  } catch {
    return ''
  }
}

function formatElapsed(message: MediaStudioMessage): string {
  const completedAt = message.completedAt
  if (typeof completedAt !== 'number') return ''
  const totalSeconds = Math.max(0, (completedAt - message.createdAt) / 1000)
  if (totalSeconds < 60) return `${totalSeconds.toFixed(1)}s`
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = Math.round(totalSeconds % 60)
  return `${minutes}m ${String(seconds).padStart(2, '0')}s`
}

function videoProgress(message: MediaStudioMessage): number {
  const value = message.progress
  if (typeof value !== 'number' || !Number.isFinite(value)) return 0
  return Math.min(100, Math.max(0, Math.round(value)))
}

function ratioNumber(value: string | undefined, fallback: string): number {
  const parse = (input: string): number => {
    const [width, height] = input.replace(/^custom:/, '').split(':').map(part => Number(part))
    return Number.isFinite(width) && Number.isFinite(height) && width > 0 && height > 0
      ? width / height
      : Number.NaN
  }
  const ratio = parse(value || '')
  if (Number.isFinite(ratio)) return ratio
  const fallbackRatio = parse(fallback)
  return Number.isFinite(fallbackRatio) ? fallbackRatio : 1
}

const MAX_MEDIA_PREVIEW_HEIGHT_PX = 384

function imageFrameStyle(message: MediaStudioMessage) {
  const ratio = ratioNumber(message.imageAspectRatio || props.imageAspectRatio, '1:1')
  return {
    aspectRatio: `${ratio}`,
    maxWidth: `min(100%, calc(${MAX_MEDIA_PREVIEW_HEIGHT_PX}px * ${ratio}))`,
  }
}

function videoFrameStyle(message: MediaStudioMessage) {
  const ratio = ratioNumber(message.aspectRatio || props.videoAspectRatio, '16:9')
  return {
    aspectRatio: `${ratio}`,
    maxWidth: `min(100%, calc(${MAX_MEDIA_PREVIEW_HEIGHT_PX}px * ${ratio}))`,
  }
}

function imageGridClass(message: MediaStudioMessage): string {
  return (message.count || 1) > 1 ? 'sm:grid-cols-2' : ''
}

function closeMenusOnOutsidePointer(event: PointerEvent) {
  const target = event.target
  if (!(target instanceof Node)) return
  if (
    typeMenuRef.value?.contains(target)
    || typeMenuTriggerRef.value?.contains(target)
    || settingsMenuRef.value?.contains(target)
    || settingsMenuTriggerRef.value?.contains(target)
  ) return
  typeMenuOpen.value = false
  settingsMenuOpen.value = false
}

function closeMenusOnEscape(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  typeMenuOpen.value = false
  settingsMenuOpen.value = false
  closeImagePreview()
}

onMounted(() => {
  document.addEventListener('pointerdown', closeMenusOnOutsidePointer)
  document.addEventListener('keydown', closeMenusOnEscape)
  document.addEventListener('paste', handlePaste)
})

onUnmounted(() => {
  document.removeEventListener('pointerdown', closeMenusOnOutsidePointer)
  document.removeEventListener('keydown', closeMenusOnEscape)
  document.removeEventListener('paste', handlePaste)
})
</script>

<style scoped>
.composer-chip {
  @apply inline-flex h-10 items-center gap-2 rounded-xl border border-gray-200 bg-white px-3 text-sm font-medium text-gray-700 shadow-sm transition-colors hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-200 dark:hover:bg-dark-800;
}

.session-action {
  @apply inline-flex h-9 items-center gap-2 rounded-xl border border-gray-200 bg-white/85 px-3 text-sm font-medium text-gray-700 shadow-sm transition hover:bg-white disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-700 dark:bg-dark-900/80 dark:text-gray-200 dark:hover:bg-dark-800;
}

.icon-action {
  @apply inline-flex h-9 w-9 items-center justify-center rounded-xl border border-gray-200 bg-white/85 text-gray-700 shadow-sm transition hover:bg-white dark:border-dark-700 dark:bg-dark-900/80 dark:text-gray-200 dark:hover:bg-dark-800;
}

.image-edit-action {
  @apply absolute bottom-2 right-2 inline-flex h-8 w-8 items-center justify-center rounded-full border border-white/60 bg-black/65 text-white opacity-0 shadow-lg transition hover:bg-black/80 focus-visible:opacity-100 dark:border-white/20;
}

.image-preview-card:hover .image-edit-action,
.image-preview-card:focus-within .image-edit-action {
  opacity: 1;
}

.message-select-button {
  @apply absolute -top-2 inline-flex h-5 w-5 items-center justify-center rounded-full border border-gray-200 bg-white/90 text-gray-500 opacity-0 shadow-sm transition hover:border-gray-400 hover:text-gray-900 focus-visible:opacity-100 dark:border-dark-700 dark:bg-dark-900/90 dark:text-gray-400 dark:hover:border-dark-500 dark:hover:text-white;
}

.group:hover .message-select-button,
.group:focus-within .message-select-button {
  opacity: 1;
}

.message-select-button-active {
  @apply border-primary-500 bg-primary-50 text-primary-600 opacity-100 dark:border-primary-400 dark:bg-primary-500/15 dark:text-primary-300;
}

.section-title {
  @apply text-[11px] font-semibold uppercase tracking-wider text-gray-400 dark:text-gray-500;
}

.param-field {
  @apply flex w-full min-w-0 flex-col gap-1.5;
}

.param-label {
  @apply truncate text-xs font-medium text-gray-500 dark:text-gray-400;
}

.param-control {
  @apply h-9 w-full min-w-0 rounded-lg border border-gray-200 bg-white px-2.5 text-sm font-medium text-gray-700 shadow-sm outline-none transition-colors hover:border-gray-300 focus:border-gray-400 disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-200 dark:hover:border-dark-600 dark:focus:border-dark-500;
  color-scheme: light dark;
}

/* 原生下拉箭头位置无法用 padding 调整，改为隐藏后自绘 */
select.param-control {
  @apply appearance-none bg-no-repeat pr-9;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='%236b7280' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E");
  background-position: right 0.75rem center;
  background-size: 1rem 1rem;
}

:global(.dark) select.param-control {
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='%239ca3af' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='m6 9 6 6 6-6'/%3E%3C/svg%3E");
}

.param-control option {
  @apply bg-white text-gray-800 dark:bg-dark-900 dark:text-gray-100;
}

.meta-pill {
  @apply inline-flex h-6 items-center rounded-full border border-gray-200 bg-gray-50 px-2 dark:border-dark-700 dark:bg-dark-800;
}

/* 生成中占位：彩色光斑缓慢流动，再覆盖一层朦胧雾面 */
.generating-frame {
  @apply relative overflow-hidden rounded-2xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900;
  isolation: isolate;
}

.generating-glow {
  position: absolute;
  inset: -60%;
  background: conic-gradient(
    from 0deg,
    #ffb3e6,
    #c4b5fd,
    #a5f3fc,
    #86efac,
    #fde68a,
    #fbcfe8,
    #ffb3e6
  );
  filter: blur(40px) saturate(1.5) brightness(1.18);
  opacity: 0.95;
  animation:
    generating-spin 18s linear infinite,
    generating-breathe 5s ease-in-out infinite;
  will-change: transform, opacity;
}

.generating-frame::after {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(
    135deg,
    rgba(255, 255, 255, 0.3),
    rgba(255, 255, 255, 0.04) 45%,
    rgba(255, 255, 255, 0.22)
  );
  backdrop-filter: blur(6px);
  pointer-events: none;
}

:global(.dark) .generating-frame::after {
  background: linear-gradient(
    135deg,
    rgba(255, 255, 255, 0.1),
    rgba(255, 255, 255, 0.01) 45%,
    rgba(255, 255, 255, 0.06)
  );
}

.generating-progress {
  background-image: linear-gradient(
    90deg,
    #ff9ae0,
    #c4b5fd,
    #7dd3fc,
    #86efac,
    #fde68a,
    #ff9ae0
  );
  background-size: 220% 100%;
  animation: generating-progress-flow 3.5s linear infinite;
}

@keyframes generating-progress-flow {
  from {
    background-position: 0% 50%;
  }
  to {
    background-position: 220% 50%;
  }
}

@keyframes generating-spin {
  to {
    transform: rotate(1turn);
  }
}

@keyframes generating-breathe {
  0%,
  100% {
    opacity: 0.68;
  }
  50% {
    opacity: 1;
  }
}

@media (prefers-reduced-motion: reduce) {
  .generating-glow {
    animation: none;
    opacity: 0.55;
  }

  .generating-progress {
    animation: none;
  }
}

</style>
