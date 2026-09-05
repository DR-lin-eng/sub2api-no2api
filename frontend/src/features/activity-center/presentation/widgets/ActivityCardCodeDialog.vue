<template>
    <BaseDialog
      :show="show"
      :title="`${t('admin.activityCenter.config.codes')} · ${label}`"
      width="extra-wide"
      @close="emit('close')"
    >
      <div v-if="show" class="space-y-3">
        <div class="flex items-center justify-between gap-3">
          <div>
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.activityCenter.config.cardCodeManager') }}</h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.activityCenter.config.cardCodeManagerHint') }}</p>
          </div>
          <span :class="['rounded-md border px-3 py-1.5 text-sm font-semibold', statusClass]">
            {{ statusText }}
          </span>
        </div>
        <div class="rounded-md border border-primary-100 bg-primary-50/60 p-3 dark:border-primary-900/40 dark:bg-primary-900/10">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <span class="text-sm font-semibold text-gray-800 dark:text-gray-100">{{ t('admin.activityCenter.config.batchImport') }}</span>
            <div class="flex items-center gap-2">
              <select v-model="cardCodeImportMode" class="input h-8 w-auto py-1 text-xs">
                <option value="append">{{ t('admin.activityCenter.config.batchAppend') }}</option>
                <option value="replace">{{ t('admin.activityCenter.config.batchReplace') }}</option>
              </select>
              <button type="button" class="btn btn-primary btn-sm" :disabled="!batchCodesText.trim()" @click="importCardCodes()">
                {{ t('admin.activityCenter.config.importNow') }}
              </button>
            </div>
          </div>
          <textarea v-model="batchCodesText" rows="4" class="input mt-2 font-mono text-sm leading-6" :placeholder="t('admin.activityCenter.config.cardCodePlaceholder')"></textarea>
        </div>
        <div class="rounded-md border border-gray-200 dark:border-dark-600">
          <div class="flex items-center justify-between border-b border-gray-200 bg-gray-50 px-3 py-2 text-xs font-medium text-gray-500 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-400">
            <span>{{ t('admin.activityCenter.config.codes') }}</span>
            <span>{{ t('admin.activityCenter.config.cardCodeManagerFooter') }}</span>
          </div>
          <div class="max-h-[60vh] space-y-1 overflow-y-auto p-2">
            <div v-for="(code, codeIndex) in activeCardCodes" :key="`${prizeId}-code-${activeCodePage * cardCodesPageSize + codeIndex}`" class="flex items-center gap-2">
              <span class="w-8 shrink-0 text-right text-xs font-semibold tabular-nums text-gray-400">{{ activeCodePage * cardCodesPageSize + codeIndex + 1 }}</span>
              <input :value="code" type="text" class="input min-w-0 flex-1 py-1.5 font-mono text-sm" @input="updateCardCode(activeCodePage * cardCodesPageSize + codeIndex, ($event.target as HTMLInputElement).value)" />
              <button type="button" class="shrink-0 rounded-lg p-1.5 text-gray-500 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400" :title="t('common.delete')" @click="removeCardCode(activeCodePage * cardCodesPageSize + codeIndex)">
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </div>
          <div class="border-t border-gray-200 p-2 dark:border-dark-600">
            <div class="flex items-center justify-between gap-2">
              <button type="button" class="btn btn-secondary btn-sm" @click="addCardCode()">
                <Icon name="plus" size="sm" class="mr-1" />
                {{ t('admin.activityCenter.config.addCardCode') }}
              </button>
              <div v-if="activeCodeTotalPages > 1" class="flex items-center gap-2 text-xs text-gray-500 dark:text-dark-400">
                <button type="button" class="rounded px-2 py-1 hover:bg-gray-100 disabled:opacity-40 dark:hover:bg-dark-700" :disabled="activeCodePage === 0" @click="activeCodePage--">{{ t('admin.activityCenter.config.previousPage') }}</button>
                <span>{{ activeCodePage + 1 }} / {{ activeCodeTotalPages }}</span>
                <button type="button" class="rounded px-2 py-1 hover:bg-gray-100 disabled:opacity-40 dark:hover:bg-dark-700" :disabled="activeCodePage >= activeCodeTotalPages - 1" @click="activeCodePage++">{{ t('admin.activityCenter.config.nextPage') }}</button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
const props = defineProps<{ show: boolean; prizeId: string; label: string; codesText: string; statusClass: string; statusText: string }>()
const emit = defineEmits<{ close: []; 'update:codesText': [value: string] }>()
const { t } = useI18n()
const activeCodePage = ref(0)
const batchCodesText = ref('')
const cardCodeImportMode = ref<'append' | 'replace'>('append')
const cardCodesPageSize = 20
const codes = computed(() => props.codesText ? props.codesText.split(/\r?\n/) : [''])
const activeCardCodes = computed(() => codes.value.slice(activeCodePage.value * cardCodesPageSize, (activeCodePage.value + 1) * cardCodesPageSize))
const activeCodeTotalPages = computed(() => Math.max(1, Math.ceil(codes.value.length / cardCodesPageSize)))
function splitLines(value: string) { return value.split(/\r?\n/).map(item => item.trim()).filter(Boolean) }
function importCardCodes() {
 const incoming = splitLines(batchCodesText.value)
 if (!incoming.length) return
 const next = Array.from(new Set(cardCodeImportMode.value === 'replace' ? incoming : [...splitLines(props.codesText), ...incoming]))
 emit('update:codesText', next.join('\n'))
 batchCodesText.value = ''
 activeCodePage.value = Math.max(0, Math.ceil(next.length / cardCodesPageSize) - 1)
}
function updateCardCode(index: number, value: string) { const next = [...codes.value]; next[index] = value; emit('update:codesText', next.join('\n')) }
function addCardCode() { const next = [...codes.value, '']; emit('update:codesText', next.join('\n')); activeCodePage.value = Math.max(0, Math.ceil(next.length / cardCodesPageSize) - 1) }
function removeCardCode(index: number) { const next = [...codes.value]; next.splice(index, 1); emit('update:codesText', next.join('\n')); activeCodePage.value = Math.min(activeCodePage.value, Math.max(0, Math.ceil(next.length / cardCodesPageSize) - 1)) }
watch(() => props.show, () => { activeCodePage.value = 0; batchCodesText.value = ''; cardCodeImportMode.value = 'append' })
</script>
