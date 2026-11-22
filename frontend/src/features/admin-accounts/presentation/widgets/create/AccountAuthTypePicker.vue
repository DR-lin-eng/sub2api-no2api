<template>
  <fieldset>
    <legend class="input-label mb-2">{{ t('admin.accounts.accountType') }}</legend>
    <div class="auth-grid">
      <label v-for="option in options" :key="option.kind"
        class="auth-option min-w-0 cursor-pointer rounded-lg border px-3.5 py-2.5 transition-colors focus-within:ring-2 focus-within:ring-primary-400"
        :class="choice === option.kind ? 'border-primary-500 bg-primary-50 dark:border-primary-400 dark:bg-primary-900/20' : 'border-gray-200 hover:border-gray-400 dark:border-dark-600'">
        <input v-model="choice" type="radio" name="create-account-auth-type" :value="option.kind" class="sr-only"
          :data-testid="platform === 'grok' && option.kind === 'apikey' ? 'grok-account-type-api-key' : undefined" />
        <span class="min-w-0">
          <span class="block text-sm font-semibold leading-5 text-gray-900 dark:text-white">{{ option.title }}</span>
          <span v-if="option.detail" class="block text-xs leading-5 text-gray-500 dark:text-gray-400">{{ option.detail }}</span>
        </span>
        <span aria-hidden="true" class="grid h-4 w-4 shrink-0 place-items-center rounded-full border"
          :class="choice === option.kind ? 'border-primary-600 bg-primary-600 text-white' : 'border-gray-300 dark:border-dark-500'">
          <Icon v-if="choice === option.kind" name="check" size="xs" />
        </span>
      </label>
    </div>
  </fieldset>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/common/widgets/icons/Icon.vue'
import type { AccountPlatform } from '@/types'
import type { CreateAccountPlatformContext } from '../../accountEditorContext'

const props = defineProps<{ platform: AccountPlatform }>()
const { t } = useI18n()
type Method = CreateAccountPlatformContext['accountCategory']['value']
interface Choice { kind: Method; title: string; detail?: string }

const choice = defineModel<Method>({ required: true })
const options = computed<Choice[]>(() => {
  const api: Choice = { kind: 'apikey', title: 'API Key' }
  const oauth: Choice = { kind: 'oauth-based', title: 'OAuth' }
  const vertex: Choice = { kind: 'service_account', title: 'Vertex', detail: 'Service Account' }
  switch (props.platform) {
    case 'anthropic': return [
      { ...oauth, title: t('admin.accounts.claudeCode'), detail: t('admin.accounts.oauthSetupToken') },
      { ...api, title: t('admin.accounts.claudeConsole'), detail: api.title },
      { kind: 'bedrock', title: t('admin.accounts.bedrockLabel'), detail: t('admin.accounts.bedrockDesc') },
      vertex,
    ]
    case 'openai': return [{ ...oauth, detail: 'ChatGPT' }, { ...api, detail: 'OpenAI API' }]
    case 'gemini': return [{ ...oauth, detail: 'Google' }, { ...api, detail: 'AI Studio' }, vertex]
    case 'grok': return [{ ...oauth, detail: 'Grok' }, { ...api, detail: 'xAI API' }]
    case 'antigravity': return [{ ...oauth, detail: 'Google' }, { ...api, detail: t('admin.accounts.types.antigravityApikey') }]
    default: return [api]
  }
})
</script>

<style scoped>
.auth-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 13rem), 1fr));
  gap: .5rem;
}
.auth-option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: .65rem;
}
</style>
