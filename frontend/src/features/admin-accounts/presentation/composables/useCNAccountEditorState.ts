import { ref, watch } from 'vue'
import type { AccountPlatform } from '@/types'
import {
  defaultCNAdaptiveBaseURLs,
  defaultOpenCodeProtocolRules,
  isCNAccountPlatform,
  type CNAPIProtocol,
} from '@/core/constants/account'

export function useCNAccountEditorState(platform: () => AccountPlatform) {
  const cnAccountMode = ref<'payg' | 'coding'>('payg')
  const cnAPIProtocol = ref<CNAPIProtocol>('chat_completions')
  const openCodeAccountMode = ref<'zen' | 'go'>('go')
  const cnAdaptiveBaseURLs = ref(defaultCNAdaptiveBaseURLs('kimi', 'payg'))
  const openCodeProtocolRules = ref(defaultOpenCodeProtocolRules('go'))
  const zhipuOrganization = ref('')
  const zhipuProject = ref('')

  const reset = () => {
    cnAccountMode.value = 'payg'
    cnAPIProtocol.value = 'chat_completions'
    openCodeAccountMode.value = 'go'
    cnAdaptiveBaseURLs.value = defaultCNAdaptiveBaseURLs('kimi', 'payg')
    openCodeProtocolRules.value = defaultOpenCodeProtocolRules('go')
    zhipuOrganization.value = ''
    zhipuProject.value = ''
  }

  watch(platform, (nextPlatform) => {
    if (isCNAccountPlatform(nextPlatform)) {
      if (nextPlatform === 'deepseek') cnAccountMode.value = 'payg'
      if (nextPlatform === 'zhipu' && cnAPIProtocol.value === 'responses') {
        cnAPIProtocol.value = 'chat_completions'
      }
      cnAdaptiveBaseURLs.value = defaultCNAdaptiveBaseURLs(nextPlatform, cnAccountMode.value)
    } else if (nextPlatform === 'opencode_go') {
      cnAdaptiveBaseURLs.value = defaultCNAdaptiveBaseURLs(nextPlatform, openCodeAccountMode.value)
      openCodeProtocolRules.value = defaultOpenCodeProtocolRules(openCodeAccountMode.value)
    }
  })

  return {
    cnAccountMode,
    cnAPIProtocol,
    cnAdaptiveBaseURLs,
    openCodeAccountMode,
    openCodeProtocolRules,
    resetCNAccountEditorState: reset,
    zhipuOrganization,
    zhipuProject,
  }
}
