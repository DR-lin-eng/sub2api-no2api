import { nextTick, ref } from 'vue'

export type AccountFormTab = 'connection' | 'openai' | 'scheduling'

export function useAccountFormTab() {
  const tab = ref<AccountFormTab>('connection')

  function revealInvalid(event: Event) {
    const input = event.target as HTMLInputElement
    if (input.form?.querySelector('input:invalid, select:invalid, textarea:invalid') !== input) return
    const panel = input.closest<HTMLElement>('[data-account-tab]')?.dataset.accountTab
    if (panel !== 'scheduling' && panel !== 'connection' && panel !== 'openai') return
    tab.value = panel
    nextTick(() => input.focus())
  }

  return { tab, revealInvalid }
}
