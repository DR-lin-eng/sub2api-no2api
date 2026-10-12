<script setup lang="ts">
import { ref, defineAsyncComponent } from 'vue'
import AccountGroupsCell from '@/features/admin-accounts/presentation/widgets/AccountGroupsCell.vue'
const query = new URLSearchParams(location.search)
const translated = query.get('translation') === '1'
const translationURL = '/@fs/app/frontend/src/features/admin-groups/presentation/widgets/GroupI18nField.vue'
const GroupI18nField = translated ? defineAsyncComponent(() => import(/* @vite-ignore */ translationURL)) : null
const name = ref('默认名称')
const i18n = ref({ en: { name: 'English version', description: 'English description' } })
const groups = Array.from({ length: 6 }, (_, i) => ({ id: i + 1, name: `${i + 1} 中文长分组名称 English name for a very long group that must wrap`, platform: 'openai', subscription_type: 'standard', rate_multiplier: 1 }))
</script>
<template>
  <main class="p-4">
    <div data-testid="groups-cell" style="width:224px"><AccountGroupsCell :groups="groups" :max-display="5" /></div>
    <div v-if="translated" style="max-width:400px;margin-top:40px">
      <component :is="GroupI18nField" v-model="name" v-model:i18n="i18n" label="Group name" field="name" required data-tour="group-form-name" />
      <output data-testid="default-name">{{ name }}</output>
      <output data-testid="translated-name">{{ i18n.en.name }}</output>
    </div>
  </main>
</template>
