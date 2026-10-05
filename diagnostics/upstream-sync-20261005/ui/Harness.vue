<script setup lang="ts">
import { ref } from 'vue'
import AccountPriorityCell from '@/features/admin-accounts/presentation/widgets/AccountPriorityCell.vue'
const account = ref({ id: 7, name: '隔离测试账号', platform: 'anthropic', type: 'oauth', priority: 3 })
const updated = (value: typeof account.value) => { account.value = value }
</script>
<template>
  <main class="mx-auto max-w-2xl p-6">
    <h1 class="mb-3 text-xl font-semibold">上游同步：账号优先级控件</h1>
    <p class="mb-6 text-sm text-gray-500">本地隔离组件测试，使用生产组件与样式；保存到测试内存。</p>
    <div class="flex items-center justify-between rounded-xl border bg-white p-4">
      <span>{{ account.name }}</span><AccountPriorityCell :account="account" @updated="updated" />
    </div>
    <p class="mt-4 text-sm">已保存优先级：{{ account.priority }}</p>
  </main>
</template>
