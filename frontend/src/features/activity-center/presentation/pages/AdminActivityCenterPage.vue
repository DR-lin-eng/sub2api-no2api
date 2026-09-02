<template>
  <AppLayout>
    <TablePageLayout v-if="!isEditorRoute">
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <div class="flex-1 sm:max-w-64">
            <input
              v-model="searchQuery"
              type="text"
              :placeholder="t('admin.activityCenter.searchCampaigns')"
              class="input"
              @input="handleSearch"
            />
          </div>
          <Select v-model="filters.type" :options="typeOptions" class="w-40" @change="reload" />
          <Select v-model="filters.status" :options="statusOptions" class="w-36" @change="reload" />
          <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
            <button @click="reload" :disabled="loading" class="btn btn-secondary" :title="t('common.refresh')">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button @click="openCreate" class="btn btn-primary">
              <Icon name="plus" size="md" class="mr-1" />
              {{ t('admin.activityCenter.createCampaign') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="campaigns"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="created_at"
          default-sort-order="desc"
          @sort="handleSort"
        >
          <template #cell-title="{ value, row }">
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <span class="truncate font-medium text-gray-900 dark:text-white">{{ value }}</span>
              </div>
              <p class="mt-1 line-clamp-1 text-xs text-gray-500 dark:text-dark-400">
                {{ row.subtitle || t('admin.activityCenter.noSubtitle') }}
              </p>
            </div>
          </template>

          <template #cell-status="{ row }">
            <span :class="['badge', statusClass(row)]">
              {{ statusLabel(row) }}
            </span>
          </template>

          <template #cell-type="{ value }">
              <span class="badge badge-gray">{{ typeLabel(value) }}</span>
          </template>

          <template #cell-timeRange="{ row }">
            <div class="text-sm text-gray-600 dark:text-gray-300">
              <div><span class="font-medium">{{ t('admin.activityCenter.form.startsAt') }}:</span> {{ formatTime(row.starts_at) }}</div>
              <div class="mt-0.5"><span class="font-medium">{{ t('admin.activityCenter.form.endsAt') }}:</span> {{ formatTime(row.ends_at) }}</div>
            </div>
          </template>

          <template #cell-created_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatDateTime(value) }}</span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center space-x-1">
              <button @click="openEdit(row)" class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-600 dark:hover:text-gray-300" :title="t('common.edit')">
                <Icon name="edit" size="sm" />
              </button>
              <button @click="handleDelete(row)" class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400" :title="t('common.delete')">
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('empty.noData')"
              :description="t('admin.activityCenter.noCampaigns')"
              :action-text="t('admin.activityCenter.createCampaign')"
              @action="openCreate"
            />
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <section v-if="!isEditorRoute" class="mt-6 rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
      <div class="mb-4 flex flex-wrap items-center gap-3">
        <div>
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.activityCenter.records.title') }}</h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.activityCenter.records.description') }}</p>
        </div>
        <div class="ml-auto flex flex-wrap items-center gap-2">
          <input
            v-model="recordSearchQuery"
            type="text"
            class="input w-64"
            :placeholder="t('admin.activityCenter.records.searchPlaceholder')"
            @input="handleRecordSearch"
          />
          <button type="button" class="btn btn-secondary" :disabled="recordsLoading" :title="t('common.refresh')" @click="reloadRecords">
            <Icon name="refresh" size="md" :class="recordsLoading ? 'animate-spin' : ''" />
          </button>
        </div>
      </div>
      <DataTable
        :columns="recordColumns"
        :data="records"
        :loading="recordsLoading"
        :server-side-sort="true"
        default-sort-key="created_at"
        default-sort-order="desc"
        @sort="handleRecordSort"
      >
        <template #cell-user="{ row }">
          <div class="min-w-0">
            <div class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ row.user_email || row.user_name || `#${row.user_id}` }}</div>
            <div class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">#{{ row.user_id }}</div>
          </div>
        </template>
        <template #cell-campaign="{ row }">
          <div class="min-w-0">
            <div class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ row.campaign_title }}</div>
            <div class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ typeLabel(row.campaign_type) }}</div>
          </div>
        </template>
        <template #cell-prize="{ row }">
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <span v-if="row.prize_color" class="h-2.5 w-2.5 rounded-full" :style="{ backgroundColor: row.prize_color }"></span>
              <span class="truncate text-sm text-gray-700 dark:text-dark-300">{{ row.prize_label || t('admin.activityCenter.records.noPrize') }}</span>
            </div>
            <code v-if="recordRewardDetail(row)" class="mt-1 block max-w-56 truncate text-xs text-gray-500 dark:text-dark-400">{{ recordRewardDetail(row) }}</code>
          </div>
        </template>
        <template #cell-reward_status="{ row }">
          <span class="badge badge-gray">{{ recordRewardStatusLabel(row.reward_status) }}</span>
        </template>
        <template #cell-created_at="{ value }">
          <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatDateTime(value) }}</span>
        </template>
        <template #empty>
          <EmptyState :title="t('empty.noData')" :description="t('admin.activityCenter.records.empty')" />
        </template>
      </DataTable>
      <Pagination
        v-if="recordsPagination.total > 0"
        class="mt-4"
        :page="recordsPagination.page"
        :total="recordsPagination.total"
        :page-size="recordsPagination.page_size"
        @update:page="handleRecordPageChange"
        @update:pageSize="handleRecordPageSizeChange"
      />
    </section>

    <BaseDialog
      :show="isEditorRoute || showDialog"
      :mode="isEditorRoute ? 'page' : 'dialog'"
      :title="editorTitle"
      width="wide"
      @close="closeDialog"
    >
      <form id="activity-center-form" @submit.prevent="handleSave" class="space-y-3">
        <nav class="flex items-center gap-1 border-b border-gray-200 pb-2 dark:border-dark-700" aria-label="Activity editor sections">
          <button v-for="section in editorSections" :key="section.id" type="button" :class="['rounded-md px-3 py-2 text-sm font-medium transition-colors', editorSection === section.id ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'text-gray-500 hover:bg-gray-100 hover:text-gray-800 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white']" @click="editorSection = section.id">
            {{ section.label }}
          </button>
        </nav>

        <template v-if="showEditorSection('basic')">
        <div class="grid gap-3 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.activityCenter.form.title') }}</label>
            <input v-model="form.title" class="input" required />
          </div>
          <div>
            <label class="input-label">{{ t('admin.activityCenter.form.subtitle') }}</label>
            <input v-model="form.subtitle" class="input" />
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.activityCenter.form.bannerHtml') }}</label>
          <textarea v-model="form.banner_html" rows="3" class="input" :placeholder="t('admin.activityCenter.form.bannerHtmlPlaceholder')"></textarea>
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.activityCenter.form.bannerHtmlHint') }}</p>
        </div>
        <div class="grid gap-3 md:grid-cols-3">
          <div>
            <label class="input-label">{{ t('admin.activityCenter.form.type') }}</label>
            <Select v-model="form.type" :options="formTypeOptions" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.activityCenter.form.status') }}</label>
            <Select v-model="form.status" :options="formStatusOptions" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.activityCenter.form.sortOrder') }}</label>
            <input v-model.number="form.sort_order" type="number" min="0" class="input" />
          </div>
        </div>
        </template>

        <template v-if="showEditorSection('lottery')">
        <div v-if="form.type === 'lottery'" class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
          <div class="mb-3 flex items-center justify-between gap-3">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.activityCenter.config.lottery') }}</h3>
            <button type="button" class="btn btn-secondary btn-sm" @click="addLotteryPool">
              <Icon name="plus" size="sm" class="mr-1" />
              {{ t('admin.activityCenter.config.addPool') }}
            </button>
          </div>
          <div class="space-y-3">
            <div
              v-for="(pool, poolIndex) in lotteryPools"
              :key="pool.id"
              class="rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-800"
            >
              <div class="flex items-center justify-between gap-3">
                <button
                  type="button"
                  class="flex min-w-0 flex-1 items-center gap-3 rounded-lg px-1 py-1 text-left transition-colors hover:bg-white/70 dark:hover:bg-dark-700"
                  :data-test="`pool-toggle-${poolIndex}`"
                  @click="toggleLotteryPool(pool)"
                >
                  <Icon name="chevronDown" size="sm" :class="['shrink-0 text-gray-400 transition-transform', pool.collapsed && '-rotate-90']" />
                  <span class="min-w-0 flex-1">
                    <span class="block truncate text-sm font-medium text-gray-900 dark:text-white">{{ pool.name || t('admin.activityCenter.config.pool') }}</span>
                    <span class="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-500 dark:text-dark-400">
                      <span>{{ pool.enabled ? t('admin.activityCenter.config.enabled') : t('admin.activityCenter.config.disabled') }}</span>
                      <span>{{ t('admin.activityCenter.config.prizeCount', { count: pool.prizes.length }) }}</span>
                      <span>{{ pool.required_group_ids.length === 0 ? t('admin.activityCenter.config.allUsers') : t('admin.activityCenter.config.groupCount', { count: pool.required_group_ids.length }) }}</span>
                      <span>{{ t('admin.activityCenter.config.dailyLimitSummary', { count: pool.daily_limit }) }}</span>
                    </span>
                  </span>
                </button>
                <div class="flex shrink-0 items-center gap-2">
                  <label class="flex items-center gap-2 rounded-lg px-2 py-1 text-xs text-gray-600 dark:text-dark-300" @click.stop>
                    <input v-model="pool.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300" />
                    {{ t('admin.activityCenter.config.enabled') }}
                  </label>
                  <button type="button" class="rounded-lg p-1.5 text-gray-500 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20" @click="removeLotteryPool(poolIndex)" :title="t('common.delete')">
                    <Icon name="trash" size="sm" />
                  </button>
                </div>
              </div>
              <div v-if="!pool.collapsed" class="mt-3">
                <div class="grid gap-3 md:grid-cols-4">
                  <div>
                    <label class="input-label">{{ t('admin.activityCenter.config.poolTier') }}</label>
                    <input v-model="pool.tier" class="input" />
                  </div>
                  <div>
                    <label class="input-label">{{ t('admin.activityCenter.config.poolName') }}</label>
                    <input v-model="pool.name" class="input" />
                  </div>
                  <div>
                    <label class="input-label">{{ t('admin.activityCenter.config.dailyLimit') }}</label>
                    <input v-model.number="pool.daily_limit" type="number" min="0" class="input" />
                  </div>
                  <div>
                    <label class="input-label">{{ t('admin.activityCenter.form.sortOrder') }}</label>
                    <input v-model.number="pool.sort_order" type="number" min="0" class="input" />
                  </div>
                </div>
                <div class="mt-3 grid gap-3 md:grid-cols-2">
                  <div>
                    <label class="input-label">{{ t('admin.activityCenter.config.poolDescription') }}</label>
                    <input v-model="pool.description" class="input" />
                  </div>
                  <div>
                    <GroupSelector
                      v-model="pool.required_group_ids"
                      :groups="adminGroups"
                      :searchable="true"
                    />
                  </div>
                </div>
                <div class="mb-2 flex items-center justify-between">
                  <h4 class="text-xs font-semibold uppercase text-gray-500 dark:text-dark-400">{{ t('admin.activityCenter.config.prizes') }}</h4>
                  <button type="button" class="btn btn-secondary btn-sm" @click="addLotteryPrize(pool)">
                    <Icon name="plus" size="sm" class="mr-1" />
                    {{ t('admin.activityCenter.config.addPrize') }}
                  </button>
                </div>
                <div v-if="pool.prizes.length === 0" class="rounded-lg border border-dashed border-gray-300 p-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400">
                  {{ t('admin.activityCenter.config.noPrizes') }}
                </div>
                <div v-else class="divide-y divide-gray-200 border-y border-gray-200 dark:divide-dark-600 dark:border-dark-600">
                  <div
                    v-for="(prize, prizeIndex) in pool.prizes"
                    :key="prize.id"
                    class="py-3 first:pt-0 last:pb-0"
                  >
                    <div class="mb-2 flex items-center justify-between">
                      <div class="flex items-center gap-2">
                        <span class="h-3 w-3 rounded-full" :style="{ backgroundColor: prize.color || '#8b5cf6' }"></span>
                        <span class="text-sm font-medium text-gray-900 dark:text-white">{{ prize.label || t('admin.activityCenter.config.prize') }}</span>
                      </div>
                      <button type="button" data-test="remove-prize" class="rounded-lg p-1.5 text-gray-500 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20" @click="removeLotteryPrize(pool, prizeIndex)" :title="t('common.delete')">
                        <Icon name="trash" size="sm" />
                      </button>
                    </div>
                    <div class="grid gap-3 md:grid-cols-4">
                      <div>
                        <label class="input-label">{{ t('admin.activityCenter.config.prizeLabel') }}</label>
                        <input v-model="prize.label" class="input" />
                      </div>
                      <div>
                        <label class="input-label">{{ t('admin.activityCenter.config.prizeType') }}</label>
                        <Select v-model="prize.prize_type" :options="prizeTypeOptions" />
                      </div>
                      <div>
                        <label class="input-label">{{ t('admin.activityCenter.config.weight') }}</label>
                        <input v-model.number="prize.weight" type="number" min="0" class="input" />
                      </div>
                      <div>
                        <label class="input-label">{{ t('admin.activityCenter.form.sortOrder') }}</label>
                        <input v-model.number="prize.sort_order" type="number" min="0" class="input" />
                      </div>
                    </div>
                    <div class="mt-2 grid gap-2 md:grid-cols-4">
                      <div>
                        <label class="input-label">{{ t('admin.activityCenter.config.color') }}</label>
                        <input v-model="prize.color" type="color" class="input h-10 p-1" />
                      </div>
                      <div v-if="prize.prize_type === 'balance' || prize.prize_type === 'concurrency'">
                        <label class="input-label">{{ t('admin.activityCenter.config.valueAmount') }}</label>
                        <input v-model="prize.value_amount" class="input" />
                      </div>
                      <div v-if="prize.prize_type === 'subscription'">
                        <label class="input-label">{{ t('admin.activityCenter.config.subscriptionGroup') }}</label>
                        <Select v-model="prize.reward_group_id" :options="rewardGroupOptions" :placeholder="t('admin.activityCenter.config.selectSubscriptionGroup')" searchable />
                      </div>
                      <div v-if="prize.prize_type !== 'none' && prize.prize_type !== 'card'">
                        <label class="input-label">{{ t('admin.activityCenter.config.availableCount') }}</label>
                        <input v-model="prize.available_count_text" type="number" min="0" class="input" />
                        <div
                          v-if="prizeStockSummary(pool, prize)"
                          :class="['mt-2 flex items-center gap-2 rounded-md border px-3 py-2 text-sm font-semibold', prizeStockClass(pool, prize)]"
                        >
                          <Icon name="database" size="sm" class="shrink-0" />
                          <span>{{ prizeStockSummary(pool, prize) }}</span>
                        </div>
                      </div>
                    </div>
                    <div v-if="prize.prize_type === 'card'" class="mt-2 grid gap-2 md:grid-cols-[minmax(180px,0.7fr)_minmax(0,1.3fr)]">
                      <div>
                        <label class="input-label">{{ t('admin.activityCenter.config.prizeValue') }}</label>
                        <input v-model="prize.value" class="input" />
                      </div>
                    <div>
                        <label class="input-label">{{ t('admin.activityCenter.config.codes') }}</label>
                        <button type="button" class="mt-1 flex w-full items-center justify-between gap-2 rounded-md border border-gray-200 bg-gray-50 px-3 py-2 text-left text-sm transition-colors hover:border-primary-300 hover:bg-primary-50 dark:border-dark-600 dark:bg-dark-800 dark:hover:border-primary-700 dark:hover:bg-primary-900/20" @click="openCardCodeDialog(prize)">
                          <span class="flex min-w-0 items-center gap-2 font-medium text-gray-700 dark:text-dark-200">
                            <Icon name="key" size="sm" class="shrink-0 text-primary-500" />
                            <span class="truncate">{{ cardCodesSummary(prize) }}</span>
                          </span>
                          <Icon name="chevronRight" size="sm" class="shrink-0 text-gray-400" />
                        </button>
                      </div>
                    </div>
                  </div>
                </div>
                <div v-if="pool.prizes.length > 0" class="mt-4 rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-700">
                  <h4 class="mb-3 text-xs font-semibold uppercase text-gray-500 dark:text-dark-400">{{ t('admin.activityCenter.config.weightPreview') }}</h4>
                  <div class="grid gap-4 md:grid-cols-[140px_1fr]">
                    <svg viewBox="0 0 120 120" class="h-32 w-32">
                      <circle cx="60" cy="60" r="52" fill="none" stroke="currentColor" stroke-width="14" class="text-gray-100 dark:text-dark-600" />
                      <path
                        v-for="slice in prizeWeightSlices(pool)"
                        :key="slice.id"
                        :d="slice.path"
                        :stroke="slice.color"
                        stroke-width="14"
                        fill="none"
                        stroke-linecap="butt"
                      />
                    </svg>
                    <div class="space-y-2">
                      <div v-for="prize in pool.prizes" :key="`${prize.id}-weight`" class="grid gap-2 md:grid-cols-[minmax(120px,1fr)_minmax(160px,2fr)_64px] md:items-center">
                        <div class="truncate text-sm text-gray-700 dark:text-dark-300">{{ prize.label || t('admin.activityCenter.config.prize') }}</div>
                        <input v-model.number="prize.weight" type="range" min="0" max="1000" class="w-full" />
                        <input v-model.number="prize.weight" type="number" min="0" class="input h-9 px-2" />
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
        <div v-else-if="form.type === 'redeem'" class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
          <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.activityCenter.config.redeem') }}</h3>
          <div class="grid gap-4 md:grid-cols-3">
            <div>
              <label class="input-label">{{ t('admin.activityCenter.config.codeMode') }}</label>
              <Select v-model="redeemConfig.code_mode" :options="redeemModeOptions" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.activityCenter.config.placeholder') }}</label>
              <input v-model="redeemConfig.placeholder" class="input" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.activityCenter.config.successMessage') }}</label>
              <input v-model="redeemConfig.success_message" class="input" />
            </div>
          </div>
        </div>
        <div v-else class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
          <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.activityCenter.config.custom') }}</h3>
          <div class="grid gap-4 md:grid-cols-2">
            <div>
              <label class="input-label">{{ t('admin.activityCenter.config.actionLabel') }}</label>
              <input v-model="customConfig.action_label" class="input" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.activityCenter.config.actionHint') }}</label>
              <input v-model="customConfig.action_hint" class="input" />
            </div>
          </div>
        </div>
        </template>

        <template v-if="showEditorSection('detail')">
        <div class="grid gap-3 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.activityCenter.form.startsAt') }}</label>
            <input v-model="form.starts_at_str" type="datetime-local" class="input" @change="handleStartTimeChange" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.activityCenter.form.endsAt') }}</label>
            <input v-model="form.ends_at_str" type="datetime-local" class="input" :min="currentDateTimeLocalMin" />
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.activityCenter.form.content') }}</label>
          <textarea v-model="form.content" rows="4" class="input"></textarea>
        </div>
        </template>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" @click="closeDialog" class="btn btn-secondary">{{ t('common.cancel') }}</button>
          <button type="submit" form="activity-center-form" :disabled="saving" class="btn btn-primary">
            {{ saving ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="activeCodePrize !== null"
      :title="activeCodePrize ? `${t('admin.activityCenter.config.codes')} · ${activeCodePrize.label}` : t('admin.activityCenter.config.codes')"
      width="extra-wide"
      @close="activeCodePrize = null"
    >
      <div v-if="activeCodePrize" class="space-y-3">
        <div class="flex items-center justify-between gap-3">
          <div>
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.activityCenter.config.cardCodeManager') }}</h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.activityCenter.config.cardCodeManagerHint') }}</p>
          </div>
          <span :class="['rounded-md border px-3 py-1.5 text-sm font-semibold', cardCodesClass(activeCodePrize)]">
            {{ cardCodesSummary(activeCodePrize) }}
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
              <button type="button" class="btn btn-primary btn-sm" :disabled="!batchCodesText.trim()" @click="importCardCodes(activeCodePrize)">
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
            <div v-for="(code, codeIndex) in activeCardCodes" :key="`${activeCodePrize.id}-code-${activeCodePage * cardCodesPageSize + codeIndex}`" class="flex items-center gap-2">
              <span class="w-8 shrink-0 text-right text-xs font-semibold tabular-nums text-gray-400">{{ activeCodePage * cardCodesPageSize + codeIndex + 1 }}</span>
              <input :value="code" type="text" class="input min-w-0 flex-1 py-1.5 font-mono text-sm" @input="updateCardCode(activeCodePrize, activeCodePage * cardCodesPageSize + codeIndex, ($event.target as HTMLInputElement).value)" />
              <button type="button" class="shrink-0 rounded-lg p-1.5 text-gray-500 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400" :title="t('common.delete')" @click="removeCardCode(activeCodePrize, activeCodePage * cardCodesPageSize + codeIndex)">
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </div>
          <div class="border-t border-gray-200 p-2 dark:border-dark-600">
            <div class="flex items-center justify-between gap-2">
              <button type="button" class="btn btn-secondary btn-sm" @click="addCardCode(activeCodePrize)">
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

    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.activityCenter.deleteCampaign')"
      :message="t('admin.activityCenter.deleteConfirm')"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useAppStore } from '@/core/stores/appStore'
import { getPersistedPageSize } from '@/common/composables/usePersistedPageSize'
import { formatDateTime, formatDateTimeLocalInput, parseDateTimeLocalInput } from '@/core/utils/format'
import adminActivityCenterAPI from '@/features/activity-center/data/datasources/adminActivityCenterDatasource'
import { getAll as getAllAdminGroups } from '@/features/admin-groups/data/datasources/adminGroupQueries'
import type { ActivityCampaign, ActivityCampaignConfig, ActivityParticipationRecord, ActivityPrizeStockStat, ActivityPrizeType } from '@/types'
import type { Column } from '@/common/types/uiTypes'
import type { AdminGroup } from '@/features/admin-groups/data/dtos/adminGroupDtos'

import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import TablePageLayout from '@/common/widgets/layout/TablePageLayout.vue'
import DataTable from '@/common/widgets/data/DataTable.vue'
import Pagination from '@/common/widgets/data/Pagination.vue'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import ConfirmDialog from '@/common/widgets/feedback/ConfirmDialog.vue'
import EmptyState from '@/common/widgets/feedback/EmptyState.vue'
import Select from '@/common/widgets/forms/Select.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import GroupSelector from '@/common/widgets/data/GroupSelector.vue'

const { t } = useI18n()
const appStore = useAppStore()
const route = useRoute()
const router = useRouter()
const isEditorRoute = computed(() => ['AdminActivityCenterCampaignCreate', 'AdminActivityCenterCampaignEdit'].includes(String(route?.name || '')))
const editorTitle = computed(() => route?.name === 'AdminActivityCenterCampaignEdit' || editingCampaign.value
  ? t('admin.activityCenter.editCampaign')
  : t('admin.activityCenter.createCampaign'))
const editorSection = ref<'basic' | 'lottery' | 'detail'>('basic')
const editorSections = computed(() => [
  { id: 'basic' as const, label: t('admin.activityCenter.editorSections.basic') },
  { id: 'lottery' as const, label: t('admin.activityCenter.editorSections.lottery') },
  { id: 'detail' as const, label: t('admin.activityCenter.editorSections.detail') }
])

function showEditorSection(section: 'basic' | 'lottery' | 'detail') {
  return !isEditorRoute.value || editorSection.value === section
}

const campaigns = ref<ActivityCampaign[]>([])
const records = ref<ActivityParticipationRecord[]>([])
const loading = ref(false)
const recordsLoading = ref(false)
const saving = ref(false)
const searchQuery = ref('')
const recordSearchQuery = ref('')
const showDialog = ref(false)
const showDeleteDialog = ref(false)
const editingCampaign = ref<ActivityCampaign | null>(null)
const deletingCampaign = ref<ActivityCampaign | null>(null)
const currentDateTimeLocalMin = ref(nowLocalMinuteInput())
const adminGroups = ref<AdminGroup[]>([])

const filters = reactive({
  status: '',
  type: ''
})

const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0
})

const recordsPagination = reactive({
  page: 1,
  page_size: 20,
  total: 0
})

const sortState = reactive({
  sort_by: 'created_at',
  sort_order: 'desc' as 'asc' | 'desc'
})

const recordSortState = reactive({
  sort_by: 'created_at',
  sort_order: 'desc' as 'asc' | 'desc'
})

const form = reactive({
  title: '',
  subtitle: '',
  banner_html: '',
  type: 'custom' as ActivityCampaign['type'],
  status: 'draft' as ActivityCampaign['status'],
  starts_at_str: '',
  ends_at_str: '',
  sort_order: 0,
  content: ''
})

interface LotteryPrizeForm {
  id: string
  label: string
  prize_type: ActivityPrizeType
  value_amount: string
  reward_group_id: number | null
  value: string
  discount_rate: string
  weight: number
  is_fallback: boolean
  color: string
  sort_order: number
  available_count_text: string
  codes_text: string
}

interface LotteryPoolForm {
  id: string
  tier: string
  name: string
  description: string
  required_group_ids: number[]
  enabled: boolean
  collapsed: boolean
  daily_limit: number
  sort_order: number
  prizes: LotteryPrizeForm[]
}

const lotteryPools = ref<LotteryPoolForm[]>([])
const redeemConfig = reactive({
  code_mode: 'manual' as 'manual' | 'generated',
  placeholder: '',
  success_message: ''
})
const customConfig = reactive({
  action_label: '',
  action_hint: ''
})
const activeCodePrize = ref<LotteryPrizeForm | null>(null)
const activeCodePage = ref(0)
const batchCodesText = ref('')
const cardCodeImportMode = ref<'append' | 'replace'>('append')
const cardCodesPageSize = 20
const activeCardCodes = computed(() => {
  if (!activeCodePrize.value) return []
  return cardCodeList(activeCodePrize.value).slice(activeCodePage.value * cardCodesPageSize, (activeCodePage.value + 1) * cardCodesPageSize)
})
const activeCodeTotalPages = computed(() => {
  if (!activeCodePrize.value) return 0
  return Math.max(1, Math.ceil(cardCodeList(activeCodePrize.value).length / cardCodesPageSize))
})

const columns = computed<Column[]>(() => [
  { key: 'title', label: t('admin.activityCenter.columns.title'), sortable: true },
  { key: 'type', label: t('admin.activityCenter.columns.type'), sortable: true },
  { key: 'status', label: t('admin.activityCenter.columns.status'), sortable: true },
  { key: 'timeRange', label: t('admin.activityCenter.columns.timeRange') },
  { key: 'sort_order', label: t('admin.activityCenter.columns.sortOrder'), sortable: true },
  { key: 'created_at', label: t('admin.activityCenter.columns.createdAt'), sortable: true },
  { key: 'actions', label: t('admin.activityCenter.columns.actions') }
])

const recordColumns = computed<Column[]>(() => [
  { key: 'user', label: t('admin.activityCenter.records.columns.user') },
  { key: 'campaign', label: t('admin.activityCenter.records.columns.campaign') },
  { key: 'pool_name', label: t('admin.activityCenter.records.columns.pool') },
  { key: 'prize', label: t('admin.activityCenter.records.columns.prize') },
  { key: 'reward_status', label: t('admin.activityCenter.records.columns.rewardStatus') },
  { key: 'created_at', label: t('admin.activityCenter.records.columns.createdAt'), sortable: true }
])

const typeOptions = computed(() => [
  { value: '', label: t('admin.activityCenter.filters.allTypes') },
  { value: 'lottery', label: t('admin.activityCenter.types.lottery') },
  { value: 'redeem', label: t('admin.activityCenter.types.redeem') },
  { value: 'custom', label: t('admin.activityCenter.types.custom') }
])

const statusOptions = computed(() => [
  { value: '', label: t('admin.activityCenter.filters.allStatus') },
  { value: 'draft', label: t('admin.activityCenter.status.draft') },
  { value: 'active', label: t('admin.activityCenter.status.active') },
  { value: 'archived', label: t('admin.activityCenter.status.archived') }
])

const formTypeOptions = computed(() => typeOptions.value.filter((item) => item.value))
const formStatusOptions = computed(() => statusOptions.value.filter((item) => item.value))
const prizeTypeOptions = computed(() => [
  { value: 'none', label: t('admin.activityCenter.prizeTypes.none') },
  { value: 'card', label: t('admin.activityCenter.prizeTypes.card') },
  { value: 'balance', label: t('admin.activityCenter.prizeTypes.balance') },
  { value: 'concurrency', label: t('admin.activityCenter.prizeTypes.concurrency') },
  { value: 'subscription', label: t('admin.activityCenter.prizeTypes.subscription') }
])
const prizeStockStats = computed(() => {
  const map = new Map<string, ActivityPrizeStockStat>()
  for (const stat of editingCampaign.value?.prize_stock_stats ?? []) {
    map.set(`${stat.pool_id}:${stat.prize_id}`, stat)
  }
  return map
})
const rewardGroupOptions = computed(() => [
  { value: null, label: t('admin.activityCenter.config.selectSubscriptionGroup') },
  ...adminGroups.value
    .filter((group) => group.subscription_type === 'subscription')
    .map((group) => ({ value: group.id, label: `${group.name} (#${group.id})` }))
])
const redeemModeOptions = computed(() => [
  { value: 'manual', label: t('admin.activityCenter.redeemModes.manual') },
  { value: 'generated', label: t('admin.activityCenter.redeemModes.generated') }
])

function uniqueConfigId(prefix: string) {
  return `${prefix}-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function createDefaultPrize(overrides: Partial<LotteryPrizeForm> = {}): LotteryPrizeForm {
  return {
    id: uniqueConfigId('prize'),
    label: t('admin.activityCenter.config.defaultPrize'),
    prize_type: 'none',
    value_amount: '',
    reward_group_id: null,
    value: '',
    discount_rate: '',
    weight: 100,
    is_fallback: true,
    color: '#8b5cf6',
    sort_order: 0,
    available_count_text: '',
    codes_text: '',
    ...overrides
  }
}

function createDefaultPool(overrides: Partial<LotteryPoolForm> = {}): LotteryPoolForm {
  return {
    id: uniqueConfigId('pool'),
    tier: 'basic',
    name: t('admin.activityCenter.config.defaultPool'),
    description: '',
    required_group_ids: [],
    enabled: true,
    collapsed: false,
    daily_limit: 1,
    sort_order: 0,
    prizes: [createDefaultPrize()],
    ...overrides
  }
}

function resetActivityConfig() {
  lotteryPools.value = [createDefaultPool()]
  redeemConfig.code_mode = 'manual'
  redeemConfig.placeholder = t('admin.activityCenter.config.defaultRedeemPlaceholder')
  redeemConfig.success_message = t('admin.activityCenter.config.defaultRedeemSuccess')
  customConfig.action_label = ''
  customConfig.action_hint = ''
}

function endOfSelectedStartDayLocalInput(value: string) {
  const selected = new Date(value)
  if (Number.isNaN(selected.getTime())) return ''
  selected.setHours(23, 59, 0, 0)
  return formatDateTimeLocalInput(Math.floor(selected.getTime() / 1000))
}

function nowLocalMinuteInput() {
  return formatDateTimeLocalInput(Math.floor(Date.now() / 1000))
}

function resetForm() {
  form.title = ''
  form.subtitle = ''
  form.banner_html = ''
  form.type = 'custom'
  form.status = 'draft'
  form.starts_at_str = ''
  form.ends_at_str = ''
  form.sort_order = 0
  form.content = ''
  resetActivityConfig()
  editorSection.value = 'basic'
}

function fillForm(item: ActivityCampaign) {
  form.title = item.title
  form.subtitle = item.subtitle
  form.banner_html = item.banner_html || ''
  form.type = item.type
  form.status = item.status
  form.starts_at_str = item.starts_at ? formatDateTimeLocalInput(Math.floor(new Date(item.starts_at).getTime() / 1000)) : ''
  form.ends_at_str = item.ends_at ? formatDateTimeLocalInput(Math.floor(new Date(item.ends_at).getTime() / 1000)) : ''
  form.sort_order = item.sort_order
  form.content = item.content
  applyActivityConfig(item.config_json)
}

function openCreate() {
  if (!router) {
    editingCampaign.value = null
    resetForm()
    showDialog.value = true
    return
  }
  void router.push('/admin/activity-center/campaigns/new')
}

function openEdit(item: ActivityCampaign) {
  if (!router) {
    editingCampaign.value = item
    fillForm(item)
    showDialog.value = true
    return
  }
  void router.push(`/admin/activity-center/campaigns/${item.id}/edit`)
}

function closeDialog() {
  if (isEditorRoute.value && router) {
    void router.push('/admin/activity-center/campaigns')
    return
  }
  showDialog.value = false
  editingCampaign.value = null
}

function handleStartTimeChange() {
  if (form.ends_at_str) return
  form.ends_at_str = endOfSelectedStartDayLocalInput(form.starts_at_str)
}

function validateEndTime() {
  if (!form.ends_at_str) return true
  const endsAt = parseDateTimeLocalInput(form.ends_at_str)
  if (endsAt == null) return true
  if (endsAt < Math.floor(Date.now() / 1000)) {
    appStore.showError(t('admin.activityCenter.endTimeBeforeNow'))
    return false
  }
  return true
}

function formatTime(value?: string) {
  return value ? formatDateTime(value) : t('admin.activityCenter.never')
}

function typeLabel(value: string) {
  return t(`admin.activityCenter.types.${value}`)
}

function statusLabel(row: ActivityCampaign) {
  const status = row.effective_status || row.status
  if (status === 'draft') return t('admin.activityCenter.status.draft')
  if (status === 'scheduled') {
    return t('admin.activityCenter.status.scheduled')
  }
  if (status === 'ended') {
    return t('admin.activityCenter.status.ended')
  }
  if (status === 'active') return t('admin.activityCenter.status.live')
  return t('admin.activityCenter.status.archived')
}

function statusClass(row: ActivityCampaign) {
  const status = row.effective_status || row.status
  if (status === 'scheduled') return 'badge-warning'
  if (status === 'active') return 'badge-success'
  return 'badge-gray'
}

function addLotteryPool() {
  lotteryPools.value.push(createDefaultPool({ sort_order: lotteryPools.value.length }))
}

function toggleLotteryPool(pool: LotteryPoolForm) {
  pool.collapsed = !pool.collapsed
}

function removeLotteryPool(index: number) {
  lotteryPools.value.splice(index, 1)
  if (lotteryPools.value.length === 0) {
    lotteryPools.value.push(createDefaultPool())
  }
}

function addLotteryPrize(pool: LotteryPoolForm) {
  pool.prizes.push(createDefaultPrize({ sort_order: pool.prizes.length, is_fallback: false }))
}

function removeLotteryPrize(pool: LotteryPoolForm, index: number) {
  pool.prizes.splice(index, 1)
}

function parseOptionalNumber(value: string) {
  const trimmed = String(value).trim()
  if (trimmed === '') return null
  const parsed = Number(trimmed)
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : null
}

function prizeStockSummary(pool: LotteryPoolForm, prize: LotteryPrizeForm) {
  const stat = prizeStockStats.value.get(`${pool.id}:${prize.id}`)
  if (!stat) return ''
  const availableCount = parseOptionalNumber(prize.available_count_text)
  if (availableCount == null) {
    return t('admin.activityCenter.config.stockUnlimited')
  }
  const issued = stat.issued_count
  const remaining = Math.max(0, availableCount - issued)
  return t('admin.activityCenter.config.stockUsage', {
    issued,
    remaining,
  })
}

function prizeStockClass(pool: LotteryPoolForm, prize: LotteryPrizeForm) {
  const stat = prizeStockStats.value.get(`${pool.id}:${prize.id}`)
  const availableCount = parseOptionalNumber(prize.available_count_text)
  if (!stat || availableCount == null) {
    return 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/50 dark:bg-emerald-900/20 dark:text-emerald-300'
  }
  const remaining = Math.max(0, availableCount - stat.issued_count)
  return remaining === 0
    ? 'border-red-200 bg-red-50 text-red-700 dark:border-red-900/50 dark:bg-red-900/20 dark:text-red-300'
    : 'border-blue-200 bg-blue-50 text-blue-700 dark:border-blue-900/50 dark:bg-blue-900/20 dark:text-blue-300'
}

function cardCodesSummary(prize: LotteryPrizeForm) {
  if (prize.prize_type !== 'card') return ''
  const count = splitLines(prize.codes_text).length
  return count > 0
    ? t('admin.activityCenter.config.cardStockUsage', { count })
    : t('admin.activityCenter.config.cardStockEmpty')
}

function openCardCodeDialog(prize: LotteryPrizeForm) {
  activeCodePrize.value = prize
  activeCodePage.value = 0
  batchCodesText.value = ''
  cardCodeImportMode.value = 'append'
}

function importCardCodes(prize: LotteryPrizeForm) {
  const importedCodes = splitLines(batchCodesText.value)
  if (importedCodes.length === 0) return
  const existingCodes = splitLines(prize.codes_text)
  const codes = cardCodeImportMode.value === 'replace'
    ? importedCodes
    : [...existingCodes, ...importedCodes]
  prize.codes_text = Array.from(new Set(codes)).join('\n')
  batchCodesText.value = ''
  activeCodePage.value = Math.max(0, Math.ceil(codes.length / cardCodesPageSize) - 1)
}

function cardCodeList(prize: LotteryPrizeForm) {
  if (!prize.codes_text) return ['']
  return prize.codes_text.split(/\r?\n/).map((code) => code.trim())
}

function updateCardCode(prize: LotteryPrizeForm, index: number, value: string) {
  const codes = cardCodeList(prize)
  codes[index] = value
  prize.codes_text = codes.join('\n')
}

function addCardCode(prize: LotteryPrizeForm) {
  const codes = cardCodeList(prize)
  codes.push('')
  prize.codes_text = codes.join('\n')
  activeCodePage.value = Math.max(0, Math.ceil(codes.length / cardCodesPageSize) - 1)
}

function removeCardCode(prize: LotteryPrizeForm, index: number) {
  const codes = cardCodeList(prize)
  codes.splice(index, 1)
  prize.codes_text = codes.join('\n')
  activeCodePage.value = Math.min(activeCodePage.value, Math.max(0, Math.ceil(codes.length / cardCodesPageSize) - 1))
}

function cardCodesClass(prize: LotteryPrizeForm) {
  const count = splitLines(prize.codes_text).length
  return count > 0
    ? 'border-blue-200 bg-blue-50 text-blue-700 dark:border-blue-900/50 dark:bg-blue-900/20 dark:text-blue-300'
    : 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-300'
}

function normalizeGroupIds(values: number[]) {
  return Array.from(
    new Set(values.map((item) => Number(item)).filter((item) => Number.isInteger(item) && item > 0))
  )
}

function splitLines(value: string) {
  return value
    .split(/\r?\n/)
    .map((item) => item.trim())
    .filter(Boolean)
}

interface PrizeWeightSlice {
  id: string
  color: string
  path: string
}

function polarToCartesian(cx: number, cy: number, radius: number, angleInDegrees: number) {
  const angleInRadians = (angleInDegrees - 90) * Math.PI / 180
  return {
    x: cx + radius * Math.cos(angleInRadians),
    y: cy + radius * Math.sin(angleInRadians)
  }
}

function describeArc(cx: number, cy: number, radius: number, startAngle: number, endAngle: number) {
  const start = polarToCartesian(cx, cy, radius, endAngle)
  const end = polarToCartesian(cx, cy, radius, startAngle)
  const largeArcFlag = endAngle - startAngle <= 180 ? '0' : '1'
  return `M ${start.x} ${start.y} A ${radius} ${radius} 0 ${largeArcFlag} 0 ${end.x} ${end.y}`
}

function prizeWeightSlices(pool: LotteryPoolForm): PrizeWeightSlice[] {
  const totalWeight = pool.prizes.reduce((sum, prize) => sum + Math.max(0, Number(prize.weight) || 0), 0)
  if (totalWeight <= 0) return []

  let currentAngle = 0
  return pool.prizes
    .filter((prize) => Math.max(0, Number(prize.weight) || 0) > 0)
    .map((prize) => {
      const angle = Math.max(0, Number(prize.weight) || 0) / totalWeight * 360
      const startAngle = currentAngle
      const endAngle = Math.min(359.99, currentAngle + angle)
      currentAngle += angle
      return {
        id: prize.id,
        color: prize.color || '#8b5cf6',
        path: describeArc(60, 60, 52, startAngle, endAngle)
      }
    })
}

function applyActivityConfig(raw: string | undefined) {
  resetActivityConfig()
  if (!raw) return
  let parsed: ActivityCampaignConfig
  try {
    parsed = JSON.parse(raw)
  } catch {
    return
  }
  if (parsed.lottery?.pools?.length) {
    lotteryPools.value = parsed.lottery.pools.map((pool, poolIndex) => ({
      id: pool.id || uniqueConfigId('pool'),
      tier: pool.tier || 'basic',
      name: pool.name || t('admin.activityCenter.config.defaultPool'),
      description: pool.description || '',
      required_group_ids: normalizeGroupIds(pool.required_group_ids || []),
      enabled: pool.enabled !== false,
      collapsed: false,
      daily_limit: Number.isFinite(pool.daily_limit) ? pool.daily_limit : 1,
      sort_order: Number.isFinite(pool.sort_order) ? pool.sort_order : poolIndex,
      prizes: (pool.prizes?.length ? pool.prizes : []).map((prize, prizeIndex) => ({
        id: prize.id || uniqueConfigId('prize'),
        label: prize.label || t('admin.activityCenter.config.defaultPrize'),
        prize_type: prize.prize_type || 'none',
        value_amount: prize.value_amount || '',
        reward_group_id: prize.reward_group_id || null,
        value: prize.value || '',
        discount_rate: prize.discount_rate || '',
        weight: Number.isFinite(prize.weight) ? prize.weight : 0,
        is_fallback: prize.is_fallback === true,
        color: prize.color || '#8b5cf6',
        sort_order: Number.isFinite(prize.sort_order) ? prize.sort_order : prizeIndex,
        available_count_text: prize.available_count == null ? '' : String(prize.available_count),
        codes_text: (prize.codes || []).join('\n')
      }))
    }))
    lotteryPools.value.forEach((pool) => {
      if (pool.prizes.length === 0) pool.prizes.push(createDefaultPrize())
    })
  }
  if (parsed.redeem) {
    redeemConfig.code_mode = parsed.redeem.code_mode === 'generated' ? 'generated' : 'manual'
    redeemConfig.placeholder = parsed.redeem.placeholder || ''
    redeemConfig.success_message = parsed.redeem.success_message || ''
  }
  if (parsed.custom) {
    customConfig.action_label = parsed.custom.action_label || ''
    customConfig.action_hint = parsed.custom.action_hint || ''
  }
}

function buildActivityConfigJSON() {
  const config: ActivityCampaignConfig = {}
  if (form.type === 'lottery') {
    config.lottery = {
      pools: lotteryPools.value.map((pool) => ({
        id: pool.id,
        tier: pool.tier.trim(),
        name: pool.name.trim(),
        description: pool.description.trim(),
        required_group_ids: normalizeGroupIds(pool.required_group_ids),
        enabled: pool.enabled,
        daily_limit: Number(pool.daily_limit) || 0,
        sort_order: Number(pool.sort_order) || 0,
        prizes: pool.prizes.map((prize) => ({
          id: prize.id,
          label: prize.label.trim(),
          prize_type: prize.prize_type,
          value_amount: prize.value_amount.trim(),
          reward_group_id: prize.reward_group_id,
          value: prize.value.trim(),
          discount_rate: prize.discount_rate.trim(),
          weight: Number(prize.weight) || 0,
          is_fallback: prize.is_fallback,
          color: prize.color || '#8b5cf6',
          sort_order: Number(prize.sort_order) || 0,
          available_count: parseOptionalNumber(prize.available_count_text),
          codes: splitLines(prize.codes_text)
        }))
      }))
    }
  } else if (form.type === 'redeem') {
    config.redeem = {
      code_mode: redeemConfig.code_mode,
      placeholder: redeemConfig.placeholder.trim(),
      success_message: redeemConfig.success_message.trim()
    }
  } else {
    config.custom = {
      action_label: customConfig.action_label.trim(),
      action_hint: customConfig.action_hint.trim()
    }
  }
  return JSON.stringify(config)
}

let abortController: AbortController | null = null
let recordSearchTimer: ReturnType<typeof setTimeout> | null = null

async function reload() {
  if (abortController) abortController.abort()
  const currentController = new AbortController()
  abortController = currentController
  loading.value = true
  try {
    const response = await adminActivityCenterAPI.list(
      pagination.page,
      pagination.page_size,
      {
        status: filters.status || undefined,
        type: filters.type || undefined,
        search: searchQuery.value || undefined,
        sort_by: sortState.sort_by,
        sort_order: sortState.sort_order
      },
      { signal: currentController.signal }
    )
    if (currentController.signal.aborted || abortController !== currentController) return
    campaigns.value = response.items
    pagination.total = response.total
    pagination.page = response.page
    pagination.page_size = response.page_size
  } catch (error: any) {
    if (currentController.signal.aborted || abortController !== currentController || error?.name === 'AbortError' || error?.code === 'ERR_CANCELED') return
    appStore.showError(error?.message || t('admin.activityCenter.failedToLoad'))
  } finally {
    if (abortController === currentController) {
      loading.value = false
      abortController = null
    }
  }
}

async function reloadRecords() {
  recordsLoading.value = true
  try {
    const response = await adminActivityCenterAPI.listRecords(
      recordsPagination.page,
      recordsPagination.page_size,
      {
        search: recordSearchQuery.value || undefined,
        sort_by: recordSortState.sort_by,
        sort_order: recordSortState.sort_order
      }
    )
    records.value = response.items
    recordsPagination.total = response.total
    recordsPagination.page = response.page
    recordsPagination.page_size = response.page_size
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.activityCenter.records.failedToLoad'))
  } finally {
    recordsLoading.value = false
  }
}

async function loadAdminGroups() {
  try {
    adminGroups.value = await getAllAdminGroups()
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.activityCenter.failedToLoadGroups'))
  }
}

function handlePageChange(page: number) {
  pagination.page = page
  reload()
}

function handlePageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  reload()
}

function handleRecordPageChange(page: number) {
  recordsPagination.page = page
  reloadRecords()
}

function handleRecordPageSizeChange(pageSize: number) {
  recordsPagination.page_size = pageSize
  recordsPagination.page = 1
  reloadRecords()
}

function handleSort(key: string, order: 'asc' | 'desc') {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  reload()
}

function handleRecordSort(key: string, order: 'asc' | 'desc') {
  recordSortState.sort_by = key
  recordSortState.sort_order = order
  recordsPagination.page = 1
  reloadRecords()
}

let searchTimer: ReturnType<typeof setTimeout> | null = null
function handleSearch() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    pagination.page = 1
    reload()
  }, 300)
}

function handleRecordSearch() {
  if (recordSearchTimer) clearTimeout(recordSearchTimer)
  recordSearchTimer = setTimeout(() => {
    recordsPagination.page = 1
    reloadRecords()
  }, 300)
}

function recordRewardStatusLabel(value: ActivityParticipationRecord['reward_status']) {
  return t(`admin.activityCenter.records.rewardStatus.${value}`)
}

function recordRewardDetail(record: ActivityParticipationRecord) {
  if (!record.reward_payload_json) return ''
  try {
    const payload = JSON.parse(record.reward_payload_json) as Record<string, unknown>
    return String(payload.code || payload.value || payload.value_amount || '')
  } catch {
    return ''
  }
}

async function handleSave() {
  currentDateTimeLocalMin.value = nowLocalMinuteInput()
  if (!validateEndTime()) return
  saving.value = true
  try {
    const startsAt = parseDateTimeLocalInput(form.starts_at_str)
    const endsAt = parseDateTimeLocalInput(form.ends_at_str)
    const payload = {
      title: form.title,
      subtitle: form.subtitle,
      banner_url: '',
      banner_html: form.banner_html,
      type: form.type,
      ref_id: '',
      config_json: buildActivityConfigJSON(),
      status: form.status,
      starts_at: startsAt ?? (editingCampaign.value ? 0 : undefined),
      ends_at: endsAt ?? (editingCampaign.value ? 0 : undefined),
      sort_order: form.sort_order,
      content: form.content
    }
    if (editingCampaign.value) {
      await adminActivityCenterAPI.update(editingCampaign.value.id, payload)
    } else {
      await adminActivityCenterAPI.create(payload)
    }
    appStore.showSuccess(t('common.success'))
    closeDialog()
    await reload()
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.activityCenter.failedToSave'))
  } finally {
    saving.value = false
  }
}

function handleDelete(row: ActivityCampaign) {
  deletingCampaign.value = row
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (!deletingCampaign.value) return
  try {
    await adminActivityCenterAPI.delete(deletingCampaign.value.id)
    appStore.showSuccess(t('common.success'))
    showDeleteDialog.value = false
    deletingCampaign.value = null
    await reload()
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.activityCenter.failedToDelete'))
  }
}

async function initializeEditorRoute() {
  if (!isEditorRoute.value) return
  const rawId = Array.isArray(route?.params?.id) ? route.params.id[0] : route?.params?.id
  if (!rawId) {
    editingCampaign.value = null
    resetForm()
    return
  }
  try {
    const item = await adminActivityCenterAPI.getById(Number(rawId))
    editingCampaign.value = item
    fillForm(item)
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.activityCenter.failedToLoad'))
    void router?.push('/admin/activity-center/campaigns')
  }
}

onMounted(() => {
  void loadAdminGroups()
  if (isEditorRoute.value) {
    void initializeEditorRoute()
  } else {
    resetActivityConfig()
    void reload()
    void reloadRecords()
  }
})

watch(
  () => `${String(route?.name || '')}:${String(route?.params?.id || '')}`,
  () => {
    if (isEditorRoute.value) void initializeEditorRoute()
  }
)

watch(
  () => form.type,
  () => {
    if (form.type === 'lottery' && lotteryPools.value.length === 0) {
      lotteryPools.value = [createDefaultPool()]
    }
  }
)

onUnmounted(() => {
  if (searchTimer) clearTimeout(searchTimer)
  if (recordSearchTimer) clearTimeout(recordSearchTimer)
  abortController?.abort()
})
</script>
