<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
          <!-- Left: Search + Filters -->
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <div class="relative w-full sm:w-64">
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
              />
              <input
                v-model="searchQuery"
                type="text"
                :placeholder="t('admin.channels.searchChannels', 'Search channels...')"
                class="input pl-10"
                @input="handleSearch"
              />
            </div>

            <Select
              v-model="filters.status"
              :options="statusFilterOptions"
              :placeholder="t('admin.channels.allStatus', 'All Status')"
              class="w-40"
              @change="loadChannels"
            />
          </div>

          <!-- Right: Actions -->
          <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
            <button
              @click="loadChannels"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh', 'Refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button @click="openCreateDialog" class="btn btn-primary">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.channels.createChannel', 'Create Channel') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="channels"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="created_at"
          default-sort-order="desc"
          @sort="handleSort"
        >
          <template #cell-name="{ value }">
            <span class="font-medium text-gray-900 dark:text-white">{{ value }}</span>
          </template>

          <template #cell-description="{ value }">
            <span class="text-sm text-gray-600 dark:text-gray-400">{{ value || '-' }}</span>
          </template>

          <template #cell-status="{ row }">
            <Toggle
              :modelValue="row.status === 'active'"
              @update:modelValue="toggleChannelStatus(row)"
            />
          </template>

          <template #cell-group_count="{ row }">
            <span
              class="inline-flex items-center rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-800 dark:bg-dark-600 dark:text-gray-300"
            >
              {{ (row.group_ids || []).length }}
              {{ t('admin.channels.groupsUnit', 'groups') }}
            </span>
          </template>

          <template #cell-pricing_count="{ row }">
            <span
              class="inline-flex items-center rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-800 dark:bg-dark-600 dark:text-gray-300"
            >
              {{ (row.model_pricing || []).length }}
              {{ t('admin.channels.pricingUnit', 'pricing rules') }}
            </span>
          </template>

          <template #cell-created_at="{ value }">
            <span class="text-sm text-gray-600 dark:text-gray-400">
              {{ formatDate(value) }}
            </span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button
                @click="openEditDialog(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              >
                <Icon name="edit" size="sm" />
                <span class="text-xs">{{ t('common.edit', 'Edit') }}</span>
              </button>
              <button
                @click="handleDelete(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              >
                <Icon name="trash" size="sm" />
                <span class="text-xs">{{ t('common.delete', 'Delete') }}</span>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.channels.noChannelsYet', 'No Channels Yet')"
              :description="t('admin.channels.createFirstChannel', 'Create your first channel to manage model pricing')"
              :action-text="t('admin.channels.createChannel', 'Create Channel')"
              @action="openCreateDialog"
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

    <BaseDialog
      :show="showDialog"
      :title="editingChannel ? t('admin.channels.editChannel') : t('admin.channels.createChannel')"
      width="drawer"
      :close-label="t('common.close')"
      @close="!submitting && closeDialog()"
    >
      <template #after-title>
        <div class="mx-3 flex min-w-0 flex-1 items-center">
          <button v-if="pickerMode && (activePlatform || editingChannel || form.name)" type="button" class="provider-action" @click="pickerMode = null">
            <Icon name="arrowLeft" size="sm" />{{ t('admin.channels.wizard.back') }}
          </button>
          <button v-else-if="activePlatform" type="button" class="provider-action" :disabled="submitting" @click="pickerMode = 'replace'">
            <PlatformIcon :platform="activePlatform" size="sm" :class="platformTextClass(activePlatform)" />
            <span class="truncate">{{ groupPlatformLabel(t, activePlatform) }}</span>
            <span class="hidden sm:inline">{{ t('admin.channels.wizard.changeProvider') }}</span>
            <span class="sr-only sm:hidden">{{ t('admin.channels.wizard.changeProvider') }}</span>
            <Icon name="chevronDown" size="xs" />
          </button>
        </div>
      </template>

      <ModelProviderPicker
        :hint="t('admin.channels.wizard.chooseHint')"
        v-if="pickerMode"
        :current="pickerMode === 'replace' ? activePlatform : null"
        :configured="activePlatforms"
        @select="chooseProvider"
      />
      <form v-else id="channel-form" :inert="submitting" class="mx-auto max-w-7xl pb-3" @invalid.capture="revealInvalidField" @submit.prevent="handleSubmit">
        <div class="flex flex-wrap items-center justify-between gap-3 py-1">
          <div v-if="activePlatforms.length > 1" class="flex flex-1 flex-wrap gap-1.5" role="group" :aria-label="t('admin.channels.wizard.configuredProviders')">
            <button
              v-for="platform in activePlatforms" :key="platform" type="button" class="provider-action"
              :class="{ 'provider-current': activePlatform === platform }" :aria-pressed="activePlatform === platform"
              @click="activePlatform = platform"
            >
              <PlatformIcon :platform="platform" size="xs" :class="platformTextClass(platform)" />
              {{ groupPlatformLabel(t, platform) }}
            </button>
          </div>
          <p v-else class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.channels.wizard.configureHint') }}</p>
          <div class="flex items-center gap-3">
            <button v-if="activePlatform" type="button" class="text-xs text-gray-500 hover:text-red-600" @click="removeActiveProvider">
              {{ t('admin.channels.wizard.removeProvider', { name: groupPlatformLabel(t, activePlatform) }) }}
            </button>
            <button v-if="activePlatforms.length < channelPlatformOrder.length" type="button" class="provider-action" @click="pickerMode = 'add'">
              <Icon name="plus" size="xs" />{{ t('admin.channels.wizard.addProvider') }}
            </button>
          </div>
        </div>
        <div class="sticky -top-3 z-10 mb-5 mt-3 flex border-b border-gray-200 bg-white dark:border-dark-600 dark:bg-dark-800 sm:-top-4" role="group" :aria-label="t('admin.channels.wizard.settings')">
          <button
            v-for="tab in configTabs" :key="tab.key" type="button" class="channel-tab"
            :aria-pressed="configTab === tab.key" :class="{ 'channel-tab-active': configTab === tab.key }" @click="configTab = tab.key"
          >
            <Icon :name="tab.icon" size="sm" class="hidden sm:block" />{{ t(tab.label) }}
          </button>
        </div>

        <div v-show="configTab === 'basic'" data-channel-tab="basic" class="grid items-start gap-5 lg:grid-cols-2">
          <section class="channel-panel space-y-4">
            <h4 class="font-semibold text-gray-900 dark:text-white">{{ t('admin.channels.form.basicSettings') }}</h4>
            <div>
              <label for="channel-name" class="input-label">{{ t('admin.channels.form.name') }} <span class="text-red-500">*</span></label>
              <input id="channel-name" v-model="form.name" type="text" required maxlength="100" class="input" :placeholder="t('admin.channels.form.namePlaceholder')" />
            </div>
            <div>
              <label for="channel-description" class="input-label">{{ t('admin.channels.form.description') }}</label>
              <textarea id="channel-description" v-model="form.description" rows="3" class="input" :placeholder="t('admin.channels.form.descriptionPlaceholder')"></textarea>
            </div>
            <div v-if="editingChannel">
              <label class="input-label">{{ t('admin.channels.form.status') }}</label>
              <Select v-model="form.status" :aria-label="t('admin.channels.form.status')" :options="statusEditOptions" />
            </div>
          </section>
          <div v-if="choicesState === 'error'" class="channel-panel text-sm" role="alert">
            {{ t('admin.groups.failedToLoad') }}
            <button type="button" class="ml-2 font-medium text-primary-600" @click="loadDialogChoices">{{ t('common.retry') }}</button>
          </div>
          <ChannelProviderSettings
            v-else-if="activeSection" :key="activeSection.platform" :model-value="activeSection"
            :groups="getGroupsForPlatform(activeSection.platform)" :occupied="groupToChannelMap"
            :loading="choicesState === 'loading'" :web-search-enabled="webSearchGlobalEnabled"
          />
        </div>

        <div v-show="configTab === 'models'" data-channel-tab="models" class="space-y-4">
          <label class="channel-panel flex cursor-pointer items-start justify-between gap-3">
            <span>
              <span class="block text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.channels.form.restrictModels') }}</span>
              <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('admin.channels.form.restrictModelsHint') }}</span>
            </span>
            <input v-model="form.restrict_models" type="checkbox" class="mt-1 h-4 w-4 rounded border-gray-300 text-primary-600" />
          </label>
          <ChannelModelSettings
            v-for="section in enabledSections" v-show="activePlatform === section.platform" :key="section.platform"
            :data-channel-platform="section.platform" :model-value="section"
            :syncing="syncingSections.has(section)" @sync="syncLatestModels(section)"
          />
        </div>

        <div v-show="configTab === 'billing'" data-channel-tab="billing" class="grid items-start gap-5 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
          <section class="channel-panel space-y-5">
            <div>
              <label class="input-label">{{ t('admin.channels.form.billingModelSource') }}</label>
              <Select v-model="form.billing_model_source" :aria-label="t('admin.channels.form.billingModelSource')" :options="billingModelSourceOptions" />
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.channels.form.billingModelSourceHint') }}</p>
            </div>
            <div class="flex items-center justify-between gap-4 border-t border-gray-200 pt-4 dark:border-dark-600">
              <div>
                <p class="text-sm font-medium text-gray-800 dark:text-gray-200">{{ t('admin.channels.form.applyPricingToAccountStats') }}</p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.channels.form.applyPricingToAccountStatsDesc') }}</p>
              </div>
              <Toggle v-model="form.apply_pricing_to_account_stats" :aria-label="t('admin.channels.form.applyPricingToAccountStats')" />
            </div>
          </section>
          <section
            v-for="section in enabledSections" v-show="activePlatform === section.platform" :key="section.platform"
            :data-channel-platform="section.platform" class="channel-panel space-y-3"
          >
            <div class="flex items-center justify-between">
              <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('admin.channels.form.accountStatsPricingRules') }}
              </h4>
              <button
                type="button"
                @click="addAccountStatsRule(section)"
                class="rounded-lg border border-primary-300 px-3 py-1 text-xs font-medium text-primary-600 hover:bg-primary-50 dark:border-primary-600 dark:text-primary-400 dark:hover:bg-primary-900/20"
              >
                + {{ t('admin.channels.form.addRule') }}
              </button>
            </div>

            <!-- Filter rules for this platform's groups -->
            <p
              v-if="section.account_stats_pricing_rules.length === 0"
              class="text-xs italic text-gray-400 dark:text-gray-500"
            >
              {{ t('admin.channels.form.noRulesConfigured') }}
            </p>

            <div
              v-for="(rule, ruleIndex) in section.account_stats_pricing_rules"
              :key="ruleIndex"
              class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-600"
            >
              <div class="flex items-center justify-between">
                <input
                  v-model="rule.name"
                  :placeholder="t('admin.channels.form.ruleName')"
                  class="bg-transparent text-sm font-medium text-gray-700 placeholder-gray-400 outline-none dark:text-gray-300"
                />
                <button type="button" @click="removeAccountStatsRule(section, ruleIndex)" class="text-xs text-red-500 hover:text-red-700">
                  {{ t('common.delete') }}
                </button>
              </div>

              <div>
                <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.channels.form.ruleGroups') }}</label>
                <div class="mt-1 flex flex-wrap gap-1">
                  <label
                    v-for="gid in section.group_ids"
                    :key="gid"
                    class="inline-flex cursor-pointer items-center gap-1 rounded-md border px-2 py-1 text-xs transition-colors"
                    :class="rule.group_ids.includes(gid)
                      ? 'border-primary-300 bg-primary-50 dark:border-primary-700 dark:bg-primary-900/20'
                      : 'border-gray-200 hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-700'"
                  >
                    <input type="checkbox" :checked="rule.group_ids.includes(gid)" class="h-3 w-3 rounded border-gray-300 text-primary-600 focus:ring-primary-500" @change="rule.group_ids.includes(gid) ? rule.group_ids.splice(rule.group_ids.indexOf(gid), 1) : rule.group_ids.push(gid)" />
                    <span :class="['font-medium', platformTextClass(section.platform)]">{{ getGroupNameById(gid) }}</span>
                  </label>
                </div>
                <p v-if="section.group_ids.length === 0" class="mt-1 text-xs text-gray-400">
                  {{ t('admin.channels.form.noGroupsInChannel') }}
                </p>
              </div>

              <div>
                <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.channels.form.ruleAccounts') }}</label>
                <!-- Selected account chips -->
                <div class="mt-1 flex flex-wrap gap-1">
                  <span
                    v-for="accountId in rule.account_ids"
                    :key="accountId"
                    class="inline-flex items-center gap-1 rounded-md border border-primary-300 bg-primary-50 px-2 py-0.5 text-xs dark:border-primary-700 dark:bg-primary-900/20"
                  >
                    <span :class="['font-medium', platformTextClass(section.platform)]">{{ getRuleAccountLabel(accountId) }}</span>
                    <button type="button" @click="removeRuleAccount(rule, accountId)" class="text-gray-400 hover:text-red-500">
                      <Icon name="x" size="xs" />
                    </button>
                  </span>
                </div>
                <!-- Account search input -->
                <div class="relative mt-1 rule-account-search-container">
                  <input
                    v-model="ruleAccountSearchKeyword[`${section.platform}-${ruleIndex}`]"
                    type="text"
                    class="input text-sm"
                    :placeholder="t('admin.channels.form.searchAccountPlaceholder')"
                    @input="onRuleAccountSearchInput(section.platform, ruleIndex)"
                    @focus="onRuleAccountSearchFocus(section.platform, ruleIndex)"
                  />
                  <!-- Search results dropdown -->
                  <div
                    v-if="showRuleAccountDropdown[`${section.platform}-${ruleIndex}`] && (ruleAccountSearchResults[`${section.platform}-${ruleIndex}`]?.length ?? 0) > 0"
                    class="absolute z-50 mt-1 max-h-48 w-full overflow-auto rounded-lg border bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800"
                  >
                    <button
                      v-for="account in ruleAccountSearchResults[`${section.platform}-${ruleIndex}`]"
                      :key="account.id"
                      type="button"
                      @click="selectRuleAccount(rule, account, section.platform, ruleIndex)"
                      class="w-full px-3 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-700"
                      :class="{ 'opacity-50': rule.account_ids.includes(account.id) }"
                      :disabled="rule.account_ids.includes(account.id)"
                    >
                      <span :class="platformTextClass(account.platform)">{{ account.name }}</span>
                      <span class="ml-2 text-xs text-gray-400">#{{ account.id }}</span>
                    </button>
                  </div>
                </div>
                <p class="mt-1 text-xs text-gray-400">
                  {{ t('admin.channels.form.ruleAccountsHint') }}
                </p>
              </div>

              <div>
                <div class="mb-1 flex items-center justify-between">
                  <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.channels.form.ruleModelPricing') }}</label>
                  <button type="button" @click="rule.pricing.push(newPricingEntry())" class="text-xs text-primary-600 hover:text-primary-700">
                    + {{ t('common.add') }}
                  </button>
                </div>
                <div v-if="rule.pricing.length === 0" class="rounded border border-dashed border-gray-300 p-2 text-center text-xs text-gray-400 dark:border-dark-500">
                  {{ t('admin.channels.form.noPricingRules') }}
                </div>
                <div v-else class="space-y-2">
                  <PricingEntryCard
                    v-for="(entry, pIdx) in rule.pricing"
                    :key="pIdx"
                    :entry="entry"
                    :platform="section.platform"
                    @update="rule.pricing.splice(pIdx, 1, $event)"
                    @remove="rule.pricing.splice(pIdx, 1)"
                  />
                </div>
              </div>
            </div>
          </section>
        </div>
      </form>

      <template #footer>
        <button v-if="pickerMode && !editingChannel && !activePlatforms.length" type="button" class="mr-auto text-sm text-gray-600 hover:text-primary-600 dark:text-gray-300" @click="pickerMode = null">{{ t('admin.channels.wizard.configureLater') }}</button>
        <button type="button" class="btn btn-secondary" :disabled="submitting" @click="closeDialog">{{ t('common.cancel') }}</button>
        <button v-if="!pickerMode" type="submit" form="channel-form" class="btn btn-primary" :disabled="submitting || choicesState !== 'ready'">
          {{ submitting ? t('common.submitting') : editingChannel ? t('common.update') : t('admin.channels.createChannel') }}
        </button>
      </template>
    </BaseDialog>

    <!-- Delete Confirmation -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.channels.deleteChannel', 'Delete Channel')"
      :message="deleteConfirmMessage"
      :confirm-text="t('common.delete', 'Delete')"
      :cancel-text="t('common.cancel', 'Cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, nextTick, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/core/stores/appStore'
import { extractApiErrorMessage } from '@/core/utils/apiError'
import type { BillingModelSource } from '@/core/constants/channel'
import { adminAPI } from '@/api/admin'
import { getAll as getAllAdminGroups } from '@/features/admin-groups/data/datasources/adminGroupQueries'
import { groupPlatformLabel } from '@/features/admin-groups/groupsLocale'
import type { Channel, CreateChannelRequest, UpdateChannelRequest, AccountStatsPricingRule } from '@/features/admin-channels/data/datasources/adminChannelsDatasource'
import { findModelConflict, newPricingEntry, validateIntervals } from '@/features/admin-channels/presentation/adminChannelSignals'
import type { PlatformSection } from '@/features/admin-channels/presentation/channelFormCodec'
import {
  buildAccountStatsPricingRules,
  buildChannelAPIFields,
  channelToPlatformSections,
  distributeAccountStatsPricingRules,
} from '@/features/admin-channels/presentation/channelFormCodec'
import { channelPlatformOrder, type ChannelPlatform } from '../channelProviders'
import ModelProviderPicker from '@/common/widgets/forms/ModelProviderPicker.vue'
import ChannelProviderSettings from '../widgets/ChannelProviderSettings.vue'
import ChannelModelSettings from '../widgets/ChannelModelSettings.vue'
import type { AdminGroup, GroupPlatform } from '@/types'
import type { Column } from '@/common/types/uiTypes'
import { platformTextClass } from '@/core/utils/platformColors'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import TablePageLayout from '@/common/widgets/layout/TablePageLayout.vue'
import DataTable from '@/common/widgets/data/DataTable.vue'
import Pagination from '@/common/widgets/data/Pagination.vue'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import ConfirmDialog from '@/common/widgets/feedback/ConfirmDialog.vue'
import EmptyState from '@/common/widgets/feedback/EmptyState.vue'
import Select from '@/common/widgets/forms/Select.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import PlatformIcon from '@/common/widgets/icons/PlatformIcon.vue'
import Toggle from '@/common/widgets/forms/Toggle.vue'
import PricingEntryCard from '@/features/admin-channels/presentation/widgets/PricingEntryCard.vue'
import { getPersistedPageSize } from '@/common/composables/usePersistedPageSize'
import { useKeyedDebouncedSearch } from '@/common/composables/useKeyedDebouncedSearch'

const { t } = useI18n()
const appStore = useAppStore()

// Web Search global enabled state (loaded once on mount)
const webSearchGlobalEnabled = ref(false)
async function loadWebSearchGlobalState() {
  try {
    const cfg = await adminAPI.settings.getWebSearchEmulationConfig()
    webSearchGlobalEnabled.value = cfg?.enabled === true && (cfg?.providers?.length ?? 0) > 0
  } catch (err: unknown) {
    console.warn('Failed to load web search global state:', err)
    webSearchGlobalEnabled.value = false
  }
}

// ── Table columns ──
const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.channels.columns.name', 'Name'), sortable: true },
  { key: 'description', label: t('admin.channels.columns.description', 'Description'), sortable: false },
  { key: 'status', label: t('admin.channels.columns.status', 'Status'), sortable: true },
  { key: 'group_count', label: t('admin.channels.columns.groups', 'Groups'), sortable: false },
  { key: 'pricing_count', label: t('admin.channels.columns.pricing', 'Pricing'), sortable: false },
  { key: 'created_at', label: t('admin.channels.columns.createdAt', 'Created'), sortable: true },
  { key: 'actions', label: t('admin.channels.columns.actions', 'Actions'), sortable: false }
])

const statusFilterOptions = computed(() => [
  { value: '', label: t('admin.channels.allStatus', 'All Status') },
  { value: 'active', label: t('admin.channels.statusActive', 'Active') },
  { value: 'disabled', label: t('admin.channels.statusDisabled', 'Disabled') }
])

const statusEditOptions = computed(() => [
  { value: 'active', label: t('admin.channels.statusActive', 'Active') },
  { value: 'disabled', label: t('admin.channels.statusDisabled', 'Disabled') }
])

const billingModelSourceOptions = computed(() => [
  { value: 'channel_mapped', label: t('admin.channels.form.billingModelSourceChannelMapped', 'Bill by channel-mapped model') },
  { value: 'requested', label: t('admin.channels.form.billingModelSourceRequested', 'Bill by requested model') },
  { value: 'upstream', label: t('admin.channels.form.billingModelSourceUpstream', 'Bill by final upstream model') },
  { value: 'response_model', label: t('admin.channels.form.billingModelSourceResponse', 'Bill by upstream response model') }
])

// ── State ──
const channels = ref<Channel[]>([])
const loading = ref(false)
const searchQuery = ref('')
const filters = reactive({ status: '' })
const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0
})
const sortState = reactive({
  sort_by: 'created_at',
  sort_order: 'desc' as 'asc' | 'desc'
})

// Dialog state
const showDialog = ref(false)
const editingChannel = ref<Channel | null>(null)
const submitting = ref(false)
const showDeleteDialog = ref(false)
const deletingChannel = ref<Channel | null>(null)
const configTabs = [
  { key: 'basic', label: 'admin.channels.wizard.basic', icon: 'server' },
  { key: 'models', label: 'admin.channels.wizard.models', icon: 'cube' },
  { key: 'billing', label: 'admin.channels.wizard.billing', icon: 'calculator' },
] as const
type ConfigTab = typeof configTabs[number]['key']
const configTab = ref<ConfigTab>('basic')
const pickerMode = ref<'add' | 'replace' | null>(null)
const activePlatform = ref<ChannelPlatform | null>(null)

// Groups
const allGroups = ref<AdminGroup[]>([])
const choicesState = ref<'loading' | 'ready' | 'error'>('loading')

// All channels for group-conflict detection (independent of current page)
const allChannelsForConflict = ref<Channel[]>([])

// Form data
const form = reactive({
  name: '',
  description: '',
  status: 'active',
  restrict_models: false,
  billing_model_source: 'channel_mapped' as BillingModelSource,
  platforms: [] as PlatformSection[],
  apply_pricing_to_account_stats: false,
})

let abortController: AbortController | null = null

// ── Helpers ──
function formatDate(value: string): string {
  if (!value) return '-'
  return new Date(value).toLocaleDateString()
}

// ── Platform section helpers ──
const enabledSections = computed(() => form.platforms.filter(section => section.enabled))
const activePlatforms = computed(() => enabledSections.value.map(section => section.platform))
const activeSection = computed(() => enabledSections.value.find(section => section.platform === activePlatform.value))

function addPlatformSection(platform: ChannelPlatform) {
  form.platforms.push({
    platform,
    enabled: true,
    collapsed: false,
    group_ids: [],
    model_mapping: {},
    model_pricing: [],
    web_search_emulation: false,
    codex_image_generation_bridge: false,
    bedrock_cc_compat: false,
    account_stats_pricing_rules: [],
  })
}

function chooseProvider(platform: ChannelPlatform) {
  const previous = activeSection.value
  const replacing = pickerMode.value === 'replace'
  if (replacing && previous) previous.enabled = false
  const draft = form.platforms.find(section => section.platform === platform)
  if (draft) draft.enabled = true
  else addPlatformSection(platform)

  if (!editingChannel.value && (!form.name || (replacing && previous && form.name === groupPlatformLabel(t, previous.platform)))) {
    form.name = groupPlatformLabel(t, platform)
  }
  activePlatform.value = platform
  configTab.value = 'basic'
  pickerMode.value = null
  nextTick(() => document.getElementById('channel-name')?.focus({ preventScroll: true }))
}

function removeActiveProvider() {
  if (!activeSection.value) return
  activeSection.value.enabled = false
  activePlatform.value = activePlatforms.value[0] ?? null
  configTab.value = 'basic'
}

function revealInvalidField(event: Event) {
  const input = event.target as HTMLInputElement
  if (input.form?.querySelector('input:invalid, textarea:invalid, select:invalid') !== input) return
  const tab = input.closest<HTMLElement>('[data-channel-tab]')?.dataset.channelTab as ConfigTab | undefined
  const platform = input.closest<HTMLElement>('[data-channel-platform]')?.dataset.channelPlatform as ChannelPlatform | undefined
  if (tab) configTab.value = tab
  if (platform) activePlatform.value = platform
  nextTick(() => input.focus())
}

function getGroupsForPlatform(platform: GroupPlatform): AdminGroup[] {
  return allGroups.value.filter(g => g.platform === platform || g.platform === 'composite')
}

// ── Group helpers ──
const groupToChannelMap = computed(() => {
  const map = new Map<number, string>()
  for (const ch of allChannelsForConflict.value) {
    if (editingChannel.value && ch.id === editingChannel.value.id) continue
    for (const gid of ch.group_ids || []) {
      map.set(gid, ch.name)
    }
  }
  return map
})

const deleteConfirmMessage = computed(() => {
  const name = deletingChannel.value?.name || ''
  return t(
    'admin.channels.deleteConfirm',
    { name },
    `Are you sure you want to delete channel "${name}"? This action cannot be undone.`
  )
})

const syncingSections = reactive(new Set<PlatformSection>())

async function syncLatestModels(section: PlatformSection) {
  if (syncingSections.has(section)) return
  syncingSections.add(section)
  try {
    const result = await adminAPI.channels.syncPricingModels(section.platform)
    // The user can close the drawer or replace this provider while the catalog loads.
    if (!form.platforms.includes(section) || !section.enabled) return
    const existingModels = new Set(section.model_pricing.flatMap(entry => entry.models))
    const newModels = result.models.filter(model => !existingModels.has(model))
    if (!newModels.length) {
      appStore.showSuccess(t('admin.channels.form.syncModelsAlreadyUpToDate'))
      return
    }
    // The channel API accepts up to 100 model names per pricing rule.
    for (let start = 0; start < newModels.length; start += 100) {
      section.model_pricing.push(newPricingEntry(newModels.slice(start, start + 100)))
    }
    appStore.showSuccess(t('admin.channels.form.syncModelsSuccess', { count: newModels.length }))
  } catch (error) {
    if (form.platforms.includes(section) && section.enabled) {
      appStore.showError(extractApiErrorMessage(error, t('admin.channels.form.syncModelsError')))
    }
  } finally {
    syncingSections.delete(section)
  }
}

// ── Account Stats Pricing helpers ──
function addAccountStatsRule(section: PlatformSection) {
  section.account_stats_pricing_rules.push({
    name: '',
    group_ids: [],
    account_ids: [],
    pricing: []
  })
}

function removeAccountStatsRule(section: PlatformSection, ruleIndex: number) {
  section.account_stats_pricing_rules.splice(ruleIndex, 1)
  // Clear all search state since indices shift after removal
  ruleAccountSearchRunner.clearAll()
  clearAllRuleAccountSearchState()
}

function getGroupNameById(groupId: number): string {
  const group = allGroups.value.find(g => g.id === groupId)
  return group ? group.name : `#${groupId}`
}

// ── Account search for pricing rules ──
interface SimpleAccount { id: number; name: string; platform: string }

const ruleAccountSearchKeyword = ref<Record<string, string>>({})
const ruleAccountSearchResults = ref<Record<string, SimpleAccount[]>>({})
const showRuleAccountDropdown = ref<Record<string, boolean>>({})
// Cache: account ID → name, populated when search results are selected
const ruleAccountNameCache = ref<Record<number, string>>({})

const ruleAccountSearchRunner = useKeyedDebouncedSearch<SimpleAccount[]>({
  delay: 300,
  search: async (keyword, { key, signal }) => {
    const platform = key.split('-')[0]
    const res = await adminAPI.accounts.list(1, 20, { platform, search: keyword }, { signal })
    return res.items.map(a => ({ id: a.id, name: a.name, platform: a.platform }))
  },
  onSuccess: (key, result) => { ruleAccountSearchResults.value[key] = result },
  onError: (key) => { ruleAccountSearchResults.value[key] = [] },
})

function onRuleAccountSearchInput(platform: string, ruleIndex: number) {
  const key = `${platform}-${ruleIndex}`
  showRuleAccountDropdown.value[key] = true
  ruleAccountSearchRunner.trigger(key, ruleAccountSearchKeyword.value[key] || '')
}

function onRuleAccountSearchFocus(platform: string, ruleIndex: number) {
  const key = `${platform}-${ruleIndex}`
  showRuleAccountDropdown.value[key] = true
  if (!ruleAccountSearchResults.value[key]?.length) {
    ruleAccountSearchRunner.trigger(key, ruleAccountSearchKeyword.value[key] || '')
  }
}

function selectRuleAccount(
  rule: { account_ids: number[] },
  account: SimpleAccount,
  platform: string,
  ruleIndex: number,
) {
  if (!rule.account_ids.includes(account.id)) {
    rule.account_ids.push(account.id)
    ruleAccountNameCache.value[account.id] = account.name
  }
  const key = `${platform}-${ruleIndex}`
  ruleAccountSearchKeyword.value[key] = ''
  showRuleAccountDropdown.value[key] = false
}

function removeRuleAccount(rule: { account_ids: number[] }, accountId: number) {
  const idx = rule.account_ids.indexOf(accountId)
  if (idx !== -1) rule.account_ids.splice(idx, 1)
}

function getRuleAccountLabel(accountId: number): string {
  const name = ruleAccountNameCache.value[accountId]
  return name ? `${name} #${accountId}` : `#${accountId}`
}

function handleRuleAccountClickOutside(event: MouseEvent) {
  const target = event.target as HTMLElement
  if (!target.closest('.rule-account-search-container')) {
    Object.keys(showRuleAccountDropdown.value).forEach(key => {
      showRuleAccountDropdown.value[key] = false
    })
  }
}

function clearAllRuleAccountSearchState() {
  ruleAccountSearchKeyword.value = {}
  ruleAccountSearchResults.value = {}
  showRuleAccountDropdown.value = {}
}

function accountStatsRulesToAPI(): AccountStatsPricingRule[] {
  return buildAccountStatsPricingRules(form.platforms)
}

// ── Form ↔ API conversion ──
function formToAPI() {
  return editingChannel.value
    ? buildChannelAPIFields(form.platforms, editingChannel.value.features_config)
    : buildChannelAPIFields(enabledSections.value)
}

function apiToForm(channel: Channel): PlatformSection[] {
  return channelToPlatformSections(channel, allGroups.value)
}

// ── Load data ──
async function loadChannels() {
  if (abortController) abortController.abort()
  const ctrl = new AbortController()
  abortController = ctrl
  loading.value = true

  try {
    const response = await adminAPI.channels.list(pagination.page, pagination.page_size, {
      status: filters.status || undefined,
      search: searchQuery.value || undefined,
      sort_by: sortState.sort_by,
      sort_order: sortState.sort_order
    }, { signal: ctrl.signal })

    if (ctrl.signal.aborted || abortController !== ctrl) return
    channels.value = response.items || []
    pagination.total = response.total
  } catch (error: unknown) {
    const e = error as { name?: string; code?: string }
    if (e?.name === 'AbortError' || e?.code === 'ERR_CANCELED') return
    appStore.showError(extractApiErrorMessage(error, t('admin.channels.loadError', 'Failed to load channels')))
  } finally {
    if (abortController === ctrl) {
      loading.value = false
      abortController = null
    }
  }
}

let choicesRequest: Promise<void> | undefined

function loadDialogChoices() {
  choicesRequest ??= fetchDialogChoices().finally(() => { choicesRequest = undefined })
  return choicesRequest
}

async function fetchDialogChoices() {
  choicesState.value = 'loading'
  try {
    const [groups, conflictCandidates] = await Promise.all([
      getAllAdminGroups(),
      adminAPI.channels.list(1, 1000).then(result => result.items || []).catch(() => channels.value),
    ])
    allGroups.value = groups
    allChannelsForConflict.value = conflictCandidates
    choicesState.value = 'ready'
  } catch (error) {
    choicesState.value = 'error'
    appStore.showError(extractApiErrorMessage(error, t('admin.groups.failedToLoad')))
  }
}

let searchTimeout: ReturnType<typeof setTimeout>
function handleSearch() {
  clearTimeout(searchTimeout)
  searchTimeout = setTimeout(() => {
    pagination.page = 1
    loadChannels()
  }, 300)
}

function handlePageChange(page: number) {
  pagination.page = page
  loadChannels()
}

function handlePageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  loadChannels()
}

function handleSort(key: string, order: 'asc' | 'desc') {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  loadChannels()
}

// ── Dialog ──
function resetForm() {
  form.name = ''
  form.description = ''
  form.status = 'active'
  form.restrict_models = false
  form.billing_model_source = 'channel_mapped'
  form.platforms = []
  form.apply_pricing_to_account_stats = false
  configTab.value = 'basic'
  activePlatform.value = null
  pickerMode.value = null
  ruleAccountSearchRunner.clearAll()
  clearAllRuleAccountSearchState()
  ruleAccountNameCache.value = {}
}

function openCreateDialog() {
  editingChannel.value = null
  resetForm()
  pickerMode.value = 'add'
  showDialog.value = true
  void loadDialogChoices()
}

async function openEditDialog(channel: Channel) {
  resetForm()
  editingChannel.value = channel
  form.name = channel.name
  form.description = channel.description || ''
  form.status = channel.status
  form.restrict_models = channel.restrict_models || false
  form.billing_model_source = channel.billing_model_source || 'channel_mapped'
  form.apply_pricing_to_account_stats = channel.apply_pricing_to_account_stats || false
  // Must load groups first so apiToForm can map groupID → platform
  await loadDialogChoices()
  if (editingChannel.value?.id !== channel.id || choicesState.value !== 'ready') return
  form.platforms = apiToForm(channel)
  activePlatform.value = activePlatforms.value[0] ?? null

  // Distribute channel-level rules into per-platform sections
  distributeRulesToPlatforms(channel.account_stats_pricing_rules || [])

  // Populate ruleAccountNameCache for existing rule accounts
  await populateRuleAccountNameCache()

  if (editingChannel.value?.id === channel.id) showDialog.value = true
}

/** Distribute flat channel-level rules into the matching platform section based on group_ids */
function distributeRulesToPlatforms(apiRules: AccountStatsPricingRule[]) {
  distributeAccountStatsPricingRules(form.platforms, apiRules, allGroups.value)
}

/** Populate ruleAccountNameCache by fetching account details for all account_ids in rules */
async function populateRuleAccountNameCache() {
  const allAccountIds = new Set<number>()
  for (const section of form.platforms) {
    for (const rule of section.account_stats_pricing_rules) {
      for (const id of rule.account_ids) {
        allAccountIds.add(id)
      }
    }
  }
  if (allAccountIds.size === 0) return

  const ids = [...allAccountIds]
  const accounts = await adminAPI.accounts.getBatchSummaries(ids)
  for (const account of accounts) {
    ruleAccountNameCache.value[account.id] = account.name
  }
}

function closeDialog() {
  showDialog.value = false
  editingChannel.value = null
  resetForm()
}

async function handleSubmit() {
  if (submitting.value || choicesState.value !== 'ready' || pickerMode.value) return
  if (!form.name.trim()) {
    configTab.value = 'basic'
    nextTick(() => document.getElementById('channel-name')?.focus())
    appStore.showError(t('admin.channels.nameRequired', 'Please enter a channel name'))
    return
  }

  // Check for pricing entries with empty models (would be silently skipped)
  for (const section of enabledSections.value) {
    if (section.group_ids.length === 0) {
      const platformLabel = groupPlatformLabel(t, section.platform)
      appStore.showError(t('admin.channels.noGroupsSelected', { platform: platformLabel }))
      activePlatform.value = section.platform
      configTab.value = 'basic'
      return
    }
    for (const entry of section.model_pricing) {
      if (entry.models.length === 0) {
        const platformLabel = groupPlatformLabel(t, section.platform)
        appStore.showError(t('admin.channels.emptyModelsInPricing', { platform: platformLabel }))
        activePlatform.value = section.platform
        configTab.value = 'models'
        return
      }
    }
  }

  // Check model pattern conflicts per platform (duplicate / wildcard overlap)
  for (const section of enabledSections.value) {
    // Collect all pricing models for this platform
    const allModels: string[] = []
    for (const entry of section.model_pricing) {
      allModels.push(...entry.models)
    }
    const pricingConflict = findModelConflict(allModels)
    if (pricingConflict) {
      appStore.showError(
        t('admin.channels.modelConflict',
          { model1: pricingConflict[0], model2: pricingConflict[1] })
      )
      activePlatform.value = section.platform
      configTab.value = 'models'
      return
    }
    // Check model mapping source pattern conflicts
    const mappingKeys = Object.keys(section.model_mapping)
    if (mappingKeys.length > 0) {
      const mappingConflict = findModelConflict(mappingKeys)
      if (mappingConflict) {
        appStore.showError(
          t('admin.channels.mappingConflict',
            { model1: mappingConflict[0], model2: mappingConflict[1] })
        )
        activePlatform.value = section.platform
        configTab.value = 'models'
        return
      }
    }
  }

  // 校验 per_request/image 模式必须有价格 (只校验启用的平台)
  for (const section of enabledSections.value) {
    for (const entry of section.model_pricing) {
      if (entry.models.length === 0) continue
      if (entry.billing_mode !== 'token' &&
          (entry.per_request_price == null || entry.per_request_price === '') &&
          (!entry.intervals || entry.intervals.length === 0)) {
        configTab.value = 'models'
        activePlatform.value = section.platform
        appStore.showError(t('admin.channels.form.perRequestPriceRequired'))
        return
      }
    }
  }

  // 校验区间合法性（范围、重叠等）
  for (const section of enabledSections.value) {
    for (const entry of section.model_pricing) {
      if (!entry.intervals || entry.intervals.length === 0) continue
      const intervalErr = validateIntervals(entry.intervals, entry.billing_mode, t)
      if (intervalErr) {
        const platformLabel = groupPlatformLabel(t, section.platform)
        const modelLabel = entry.models.join(', ') || t('admin.channels.form.unnamed')
        appStore.showError(`${platformLabel} - ${modelLabel}: ${intervalErr}`)
        activePlatform.value = section.platform
        configTab.value = 'models'
        return
      }
    }
  }

  const { group_ids, model_pricing, model_mapping, features_config } = formToAPI()

  submitting.value = true
  try {
    if (editingChannel.value) {
      const req: UpdateChannelRequest = {
        name: form.name.trim(),
        description: form.description.trim() || undefined,
        status: form.status,
        group_ids,
        model_pricing,
        model_mapping: Object.keys(model_mapping).length > 0 ? model_mapping : {},
        billing_model_source: form.billing_model_source,
        restrict_models: form.restrict_models,
        features_config,
        apply_pricing_to_account_stats: form.apply_pricing_to_account_stats,
        account_stats_pricing_rules: accountStatsRulesToAPI()
      }
      await adminAPI.channels.update(editingChannel.value.id, req)
      appStore.showSuccess(t('admin.channels.updateSuccess', 'Channel updated'))
    } else {
      const req: CreateChannelRequest = {
        name: form.name.trim(),
        description: form.description.trim() || undefined,
        group_ids,
        model_pricing,
        model_mapping: Object.keys(model_mapping).length > 0 ? model_mapping : {},
        billing_model_source: form.billing_model_source,
        restrict_models: form.restrict_models,
        features_config,
        apply_pricing_to_account_stats: form.apply_pricing_to_account_stats,
        account_stats_pricing_rules: accountStatsRulesToAPI()
      }
      await adminAPI.channels.create(req)
      appStore.showSuccess(t('admin.channels.createSuccess', 'Channel created'))
    }
    closeDialog()
    loadChannels()
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, editingChannel.value
      ? t('admin.channels.updateError', 'Failed to update channel')
      : t('admin.channels.createError', 'Failed to create channel')))
  } finally {
    submitting.value = false
  }
}

// ── Toggle status ──
async function toggleChannelStatus(channel: Channel) {
  const newStatus = channel.status === 'active' ? 'disabled' : 'active'
  try {
    await adminAPI.channels.update(channel.id, { status: newStatus })
    if (filters.status && filters.status !== newStatus) {
      // Item no longer matches the active filter — reload list
      await loadChannels()
    } else {
      channel.status = newStatus
    }
  } catch (error) {
    appStore.showError(t('admin.channels.updateError', 'Failed to update channel'))
    console.error('Error toggling channel status:', error)
  }
}

// ── Delete ──
function handleDelete(channel: Channel) {
  deletingChannel.value = channel
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (!deletingChannel.value) return

  try {
    await adminAPI.channels.remove(deletingChannel.value.id)
    appStore.showSuccess(t('admin.channels.deleteSuccess', 'Channel deleted'))
    showDeleteDialog.value = false
    deletingChannel.value = null
    loadChannels()
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.channels.deleteError', 'Failed to delete channel')))
  }
}

// ── Lifecycle ──
onMounted(() => {
  loadChannels()
  loadWebSearchGlobalState()
  document.addEventListener('click', handleRuleAccountClickOutside)
})

onUnmounted(() => {
  clearTimeout(searchTimeout)
  abortController?.abort()
  document.removeEventListener('click', handleRuleAccountClickOutside)
  ruleAccountSearchRunner.clearAll()
  clearAllRuleAccountSearchState()
})
</script>

<style scoped>
.provider-action {
  @apply inline-flex min-w-0 items-center justify-center gap-1.5 rounded-full border border-gray-200 px-3 py-1.5 text-xs font-medium text-gray-700 transition-colors hover:border-primary-300 hover:text-primary-700 focus-visible:outline-primary-500 dark:border-dark-600 dark:text-gray-300 dark:hover:text-primary-400;
}

.provider-current {
  @apply border-primary-300 bg-primary-50 text-primary-700 dark:border-primary-600 dark:bg-primary-900/25 dark:text-primary-300;
}

.channel-tab {
  @apply flex min-w-0 flex-1 items-center justify-center gap-2 border-b-2 border-transparent px-1 py-3 text-sm font-medium text-gray-500 transition-colors hover:text-gray-900 focus-visible:outline-primary-500 dark:text-gray-400 dark:hover:text-white;
}

.channel-tab-active {
  @apply border-primary-600 text-primary-700 dark:border-primary-400 dark:text-primary-400;
}

.channel-panel {
  @apply min-w-0 rounded-xl border border-gray-200 p-4 dark:border-dark-600 sm:p-5;
}
</style>
