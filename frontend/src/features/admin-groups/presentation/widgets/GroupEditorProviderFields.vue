<script setup lang="ts">
import { useI18n } from "vue-i18n";
import Select from "@/common/widgets/forms/Select.vue";
import Icon from "@/common/widgets/icons/Icon.vue";
import type { GroupEditorDialogContext } from "../groupEditorContext";
import { supportsMessagesDispatchPlatform } from "../groupsMessagesDispatchResolver";

const { context } = defineProps<{ context: GroupEditorDialogContext }>();
const { t } = useI18n();
const {
  addMessagesDispatchMapping,
  fallbackOptions,
  form,
  getMessagesDispatchRowKey,
  removeMessagesDispatchMapping,
  toggleLive,
  webSearchFinalPricePreview,
} = context;
</script>

<template>
    <!-- Claude Code 客户端限制（仅 anthropic 平台） -->
    <div v-if="form.platform === 'anthropic'" class="border-t pt-4">
      <div class="mb-1.5 flex items-center gap-1">
        <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t("admin.groups.claudeCode.title") }}
        </label>
        <!-- Help Tooltip -->
        <div class="group relative inline-flex">
          <Icon
            name="questionCircle"
            size="sm"
            :stroke-width="2"
            class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
          />
          <div
            class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
          >
            <div
              class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
            >
              <p class="text-xs leading-relaxed text-gray-300">
                {{ t("admin.groups.claudeCode.tooltip") }}
              </p>
              <div
                class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
              ></div>
            </div>
          </div>
        </div>
      </div>
      <div class="flex items-center gap-3">
        <button
          type="button"
          @click="
            form.claude_code_only = !form.claude_code_only
          "
          :class="[
            'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
            form.claude_code_only
              ? 'bg-primary-500'
              : 'bg-gray-300 dark:bg-dark-600',
          ]"
        >
          <span
            :class="[
              'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
              form.claude_code_only
                ? 'translate-x-6'
                : 'translate-x-1',
            ]"
          />
        </button>
        <span class="text-sm text-gray-500 dark:text-gray-400">
          {{
            form.claude_code_only
              ? t("admin.groups.claudeCode.enabled")
              : t("admin.groups.claudeCode.disabled")
          }}
        </span>
      </div>
      <!-- 降级分组选择（仅当启用 claude_code_only 时显示） -->
      <div v-if="form.claude_code_only" class="mt-3">
        <label class="input-label">{{
          t("admin.groups.claudeCode.fallbackGroup")
        }}</label>
        <Select
          v-model="form.fallback_group_id"
          :options="fallbackOptions"
          :placeholder="t('admin.groups.claudeCode.noFallback')"
        />
        <p class="input-hint">
          {{ t("admin.groups.claudeCode.fallbackHint") }}
        </p>
      </div>
    </div>

    <!-- Codex 网页搜索按次计费（仅 openai 平台） -->
    <div
      v-if="form.platform === 'openai'"
      class="border-t border-gray-200 dark:border-dark-400 pt-4 mt-4"
    >
      <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">
        {{ t("admin.groups.webSearchPricing.title") }}
      </h4>
      <div>
        <label class="input-label">{{
          t("admin.groups.webSearchPricing.pricePerCall")
        }}</label>
        <input
          v-model.number="form.web_search_price_per_call"
          type="number"
          step="0.001"
          min="0"
          placeholder="0.01"
          class="input"
        />
        <p class="input-hint">
          {{ t("admin.groups.webSearchPricing.pricePerCallHint") }}
        </p>
        <div
          class="mt-2 rounded-lg bg-gray-50 p-3 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300"
        >
          {{
            t("admin.groups.webSearchPricing.finalPricePreview", {
              price: webSearchFinalPricePreview,
            })
          }}
        </div>
      </div>
    </div>

    <!-- OpenAI Live 开关（仅 openai 平台） -->
    <div
      v-if="form.platform === 'openai'"
      class="border-t border-gray-200 dark:border-dark-400 pt-4 mt-4"
    >
      <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">
        {{ t("admin.groups.openaiLive.title") }}
      </h4>
      <div class="flex items-center justify-between">
        <label class="text-sm text-gray-600 dark:text-gray-400">{{
          t("admin.groups.openaiLive.allow")
        }}</label>
        <button
          type="button"
          @click="toggleLive()"
          class="relative inline-flex h-6 w-12 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
          :class="
            form.allow_live
              ? 'bg-primary-500'
              : 'bg-gray-300 dark:bg-dark-600'
          "
        >
          <span
            class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
            :class="form.allow_live ? 'translate-x-6' : 'translate-x-1'"
          />
        </button>
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">
        {{ t("admin.groups.openaiLive.hint") }}
      </p>
    </div>

    <!-- OpenAI Messages 调度配置（OpenAI 与 Composite 平台） -->
    <div
      v-if="supportsMessagesDispatchPlatform(form.platform)"
      class="border-t border-gray-200 dark:border-dark-400 pt-4 mt-4"
    >
      <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">
        {{ t("admin.groups.openaiMessages.title") }}
      </h4>

      <!-- 允许 Messages 调度开关 -->
      <div class="flex items-center justify-between">
        <label class="text-sm text-gray-600 dark:text-gray-400">{{
          t("admin.groups.openaiMessages.allowDispatch")
        }}</label>
        <button
          type="button"
          @click="
            form.allow_messages_dispatch =
              !form.allow_messages_dispatch
          "
          class="relative inline-flex h-6 w-12 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
          :class="
            form.allow_messages_dispatch
              ? 'bg-primary-500'
              : 'bg-gray-300 dark:bg-dark-600'
          "
        >
          <span
            class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
            :class="
              form.allow_messages_dispatch
                ? 'translate-x-6'
                : 'translate-x-1'
            "
          />
        </button>
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">
        {{ t("admin.groups.openaiMessages.allowDispatchHint") }}
      </p>

      <div v-if="form.allow_messages_dispatch" class="mt-3">
        <div
          class="relative overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-600 dark:bg-dark-800"
        >
          <div
            class="border-b border-gray-100 bg-gray-50/80 px-4 py-3 dark:border-dark-700 dark:bg-dark-700/50"
          >
            <div class="flex items-center gap-2">
              <div class="h-2 w-2 rounded-full bg-blue-500"></div>
              <label
                class="text-sm font-medium text-gray-900 dark:text-white"
                >{{
                  t("admin.groups.openaiMessages.familyMappingTitle")
                }}</label
              >
            </div>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.groups.openaiMessages.familyMappingHint") }}
            </p>
          </div>
          <div class="p-4">
            <div class="grid gap-4 md:grid-cols-3">
              <div>
                <label class="input-label">{{
                  t("admin.groups.openaiMessages.opusModel")
                }}</label>
                <input
                  v-model="form.opus_mapped_model"
                  type="text"
                  :placeholder="
                    t('admin.groups.openaiMessages.opusModelPlaceholder')
                  "
                  class="input"
                />
              </div>
              <div>
                <label class="input-label">{{
                  t("admin.groups.openaiMessages.sonnetModel")
                }}</label>
                <input
                  v-model="form.sonnet_mapped_model"
                  type="text"
                  :placeholder="
                    t('admin.groups.openaiMessages.sonnetModelPlaceholder')
                  "
                  class="input"
                />
              </div>
              <div>
                <label class="input-label">{{
                  t("admin.groups.openaiMessages.haikuModel")
                }}</label>
                <input
                  v-model="form.haiku_mapped_model"
                  type="text"
                  :placeholder="
                    t('admin.groups.openaiMessages.haikuModelPlaceholder')
                  "
                  class="input"
                />
              </div>
            </div>
          </div>
        </div>

        <div
          class="mt-5 relative overflow-hidden rounded-xl border border-primary-200 bg-white shadow-sm dark:border-primary-900/50 dark:bg-dark-800"
        >
          <div
            class="border-b border-primary-100 bg-primary-50/80 px-4 py-3 dark:border-primary-900/40 dark:bg-primary-900/20"
          >
            <div class="flex items-start justify-between gap-3">
              <div>
                <div class="flex items-center gap-2">
                  <div class="h-2 w-2 rounded-full bg-primary-500"></div>
                  <label
                    class="text-sm font-medium text-primary-900 dark:text-primary-100"
                    >{{
                      t("admin.groups.openaiMessages.exactMappingTitle")
                    }}</label
                  >
                </div>
                <p
                  class="mt-1 text-xs text-primary-600/90 dark:text-primary-400/90"
                >
                  {{ t("admin.groups.openaiMessages.exactMappingHint") }}
                </p>
              </div>
            </div>
          </div>

          <div class="p-4 bg-gray-50/30 dark:bg-dark-800/30">
            <div
              v-if="form.exact_model_mappings.length === 0"
              class="flex items-center justify-between gap-3 rounded-xl border-2 border-dashed border-primary-200 bg-white px-5 py-4 text-sm text-primary-700 transition-colors hover:border-primary-300 dark:border-primary-900/40 dark:bg-dark-800 dark:text-primary-300 dark:hover:border-primary-800"
            >
              <span>{{
                t("admin.groups.openaiMessages.noExactMappings")
              }}</span>
              <button
                type="button"
                @click="addMessagesDispatchMapping"
                class="flex items-center gap-1.5 text-sm font-medium text-primary-600 transition-colors hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
              >
                <Icon name="plus" size="sm" />
                {{ t("admin.groups.openaiMessages.addExactMapping") }}
              </button>
            </div>

            <div v-else class="space-y-3">
              <div
                v-for="row in form.exact_model_mappings"
                :key="getMessagesDispatchRowKey(row)"
                class="group relative rounded-xl border border-gray-200 bg-white p-4 shadow-sm transition-all hover:border-primary-300 hover:shadow-md dark:border-dark-600 dark:bg-dark-700 dark:hover:border-primary-700"
              >
                <div class="flex items-center gap-4">
                  <div
                    class="grid flex-1 gap-4 md:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] md:items-start"
                  >
                    <div>
                      <label class="input-label">{{
                        t("admin.groups.openaiMessages.claudeModel")
                      }}</label>
                      <input
                        v-model="row.claude_model"
                        type="text"
                        :placeholder="
                          t(
                            'admin.groups.openaiMessages.claudeModelPlaceholder',
                          )
                        "
                        class="input bg-gray-50 focus:bg-white dark:bg-dark-800 dark:focus:bg-dark-900"
                      />
                    </div>
                    <div
                      class="hidden md:flex md:justify-center md:pt-7 text-primary-300 dark:text-primary-700"
                    >
                      <Icon
                        name="arrowRight"
                        size="sm"
                        class="transition-transform group-hover:translate-x-1"
                      />
                    </div>
                    <div>
                      <label class="input-label">{{
                        t("admin.groups.openaiMessages.targetModel")
                      }}</label>
                      <input
                        v-model="row.target_model"
                        type="text"
                        :placeholder="
                          t(
                            'admin.groups.openaiMessages.targetModelPlaceholder',
                          )
                        "
                        class="input bg-gray-50 focus:bg-white dark:bg-dark-800 dark:focus:bg-dark-900"
                      />
                    </div>
                  </div>
                  <button
                    type="button"
                    @click="removeMessagesDispatchMapping(row)"
                    class="mt-6 flex h-9 w-9 items-center justify-center rounded-lg text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                    :title="
                      t('admin.groups.openaiMessages.removeExactMapping')
                    "
                  >
                    <Icon name="trash" size="sm" />
                  </button>
                </div>
              </div>

              <button
                type="button"
                @click="addMessagesDispatchMapping"
                class="flex w-full items-center justify-center gap-2 rounded-xl border-2 border-dashed border-gray-300 bg-white py-3 text-sm font-medium text-gray-500 transition-all hover:border-primary-300 hover:bg-primary-50/50 hover:text-primary-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400 dark:hover:border-primary-800 dark:hover:bg-primary-900/20 dark:hover:text-primary-400"
              >
                <Icon name="plus" size="sm" />
                {{ t("admin.groups.openaiMessages.addExactMapping") }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
</template>
