<script setup lang="ts">
import { reactive } from 'vue'
import { useI18n } from 'vue-i18n'
import GroupEditorMediaPricingFields from '@/features/admin-groups/presentation/widgets/GroupEditorMediaPricingFields.vue'
import { createCreateGroupFormState } from '@/features/admin-groups/presentation/composables/groupEditorFormSupport'
const form = reactive(createCreateGroupFormState())
form.platform = 'openai'
form.allow_image_generation = true
const { locale } = useI18n()
const context = { form, imageFinalPricePreview: [], videoFinalPricePreview: [] }
</script>
<template>
  <main class="mx-auto max-w-3xl p-4">
    <div class="mb-4 flex gap-3">
      <button @click="locale='zh'">中文</button><button @click="locale='en'">English</button>
      <select v-model="form.platform" aria-label="Platform"><option>openai</option><option>composite</option><option>gemini</option></select>
    </div>
    <GroupEditorMediaPricingFields :context="context" />
  </main>
</template>
