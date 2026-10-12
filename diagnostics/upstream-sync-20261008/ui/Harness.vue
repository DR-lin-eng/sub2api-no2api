<script setup lang="ts">
import { computed, ref } from 'vue'
import AccountTableFilters from '@/features/admin-accounts/presentation/widgets/AccountTableFilters.vue'
import OpsMetricsGrid from '@/features/admin-ops/presentation/widgets/OpsMetricsGrid.vue'
import type { OpsDashboardOverview } from '@/features/admin-ops/data/dtos/opsDashboardDtos'
const search = ref('')
const filters = ref({ platform: '', status: '', type: '', oauth_quota: 'has_quota', excel_bps: 'enabled', privacy_mode: '', group: '' })
const mobile = ref(false)
const width = computed(() => mobile.value ? '375px' : '1200px')
const overview = {
  request_count_total: 240, request_count_sla: 240, token_consumed: 180000,
  success_count: 237, error_count_total: 3, error_count_sla: 3, sla: 0.9875,
  qps: {current: 3, peak: 8, avg: 2}, tps: {current: 500, peak: 900, avg: 300},
  duration: {p50_ms: 3000, p90_ms: 5000, p95_ms: 6000, p99_ms: 8000, avg_ms: 3500, max_ms: 9000},
  ttft: {p50_ms: 120, p90_ms: 180, p95_ms: 200, p99_ms: 250, avg_ms: 150, max_ms: 300},
  output_tps: {p5: 10, p10: 15, p50: 50, avg: 55, sample_count: 240},
} as OpsDashboardOverview
</script>
<template>
  <main class="mx-auto p-5">
    <div class="mb-5 flex gap-3">
      <h1 class="text-xl font-bold">账号筛选与输出速率</h1>
      <button class="btn btn-secondary" @click="mobile = !mobile">{{ mobile ? '桌面宽度' : '移动宽度' }}</button>
    </div>
    <div data-testid="viewport" :style="{ width, maxWidth: '100%' }" class="space-y-6">
      <section class="rounded-xl border p-4">
        <AccountTableFilters :search-query="search" :filters="filters" :groups="[]" @update:search-query="search = $event" @update:filters="filters = $event" />
        <p class="mt-4 text-sm text-gray-600">BPS：{{ filters.excel_bps }}；额度：{{ filters.oauth_quota }}</p>
      </section>
      <OpsMetricsGrid :overview="overview" />
    </div>
  </main>
</template>
