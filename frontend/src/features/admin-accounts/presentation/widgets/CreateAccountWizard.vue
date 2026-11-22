<template>
  <BaseDialog :show="show" :title="t('admin.accounts.createAccount')" width="drawer"
    :close-label="t('common.close')" @close="close">
    <template #after-title>
      <div v-if="selected && !picking" class="ml-3 min-w-0 flex-1">
        <button v-if="editor?.step === 1" type="button" :disabled="editor.submitting" data-tour="account-change-provider"
          class="inline-flex items-center gap-1.5 rounded-full border border-gray-200 px-3 py-1.5 text-sm font-medium hover:border-primary-400 dark:border-dark-600"
          @click="picking = true">
          <PlatformIcon :platform="selected" size="sm" />{{ modelProviderLabel(t, selected) }}
          <span class="hidden sm:inline">{{ t('admin.accounts.providerSetup.change') }}</span>
          <span class="sr-only sm:hidden">{{ t('admin.accounts.providerSetup.change') }}</span><Icon name="chevronDown" size="xs" />
        </button>
        <span v-else class="text-sm text-gray-500">{{ modelProviderLabel(t, selected) }}</span>
      </div>
    </template>
    <ModelProviderPicker v-if="picking" grid-tour="account-form-platform"
      :hint="t('admin.accounts.providerSetup.chooseHint')" :current="selected" @select="choose" />
    <div v-if="show && selected" v-show="!picking" :inert="picking">
      <CreateAccountDialog ref="editor" :key="selected" :show="!picking" :platform="selected" :groups="groups" :proxies="proxies"
        @created="emit('created')" @close="emit('close')" />
    </div>
    <template #footer>
      <button v-if="picking" type="button" class="btn btn-secondary" @click="close">
        {{ selected ? t('admin.accounts.providerSetup.back') : t('common.cancel') }}
      </button>
      <CreateAccountDialogFooter v-else-if="editor" :context="editor.footer" />
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { modelProviderLabel } from '@/core/constants/modelProviders'
import { nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import ModelProviderPicker from '@/common/widgets/forms/ModelProviderPicker.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import PlatformIcon from '@/common/widgets/icons/PlatformIcon.vue'
import type { AccountPlatform, AdminGroup, Proxy } from '@/types'
import CreateAccountDialog from './CreateAccountDialog.vue'
import CreateAccountDialogFooter from './create/CreateAccountDialogFooter.vue'

const props = defineProps<{ show: boolean; proxies: Proxy[]; groups: AdminGroup[] }>()
const emit = defineEmits<{ close: []; created: [] }>()
const { t } = useI18n()
const selected = ref<AccountPlatform | null>(null)
const picking = ref(true)
const editor = ref<InstanceType<typeof CreateAccountDialog> | null>(null)

watch(() => props.show, show => {
  if (!show) {
    selected.value = null
    picking.value = true
  }
})

function choose(platform: AccountPlatform) {
  selected.value = platform
  picking.value = false
  nextTick(() => document.getElementById('create-account-name')?.focus({ preventScroll: true }))
}

function close() {
  if (picking.value && selected.value) choose(selected.value)
  else if (editor.value) editor.value.close()
  else emit('close')
}
</script>
