<template>
  <AppLayout>
    <TablePageLayout>
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

    <BaseDialog :show="showDialog" :title="editingCampaign ? t('admin.activityCenter.editCampaign') : t('admin.activityCenter.createCampaign')" width="wide" @close="closeDialog">
      <form id="activity-center-form" @submit.prevent="handleSave" class="space-y-4">
        <div>
          <label class="input-label">{{ t('admin.activityCenter.form.title') }}</label>
          <input v-model="form.title" class="input" required />
        </div>
        <div>
          <label class="input-label">{{ t('admin.activityCenter.form.subtitle') }}</label>
          <input v-model="form.subtitle" class="input" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.activityCenter.form.bannerHtml') }}</label>
          <textarea v-model="form.banner_html" rows="4" class="input" :placeholder="t('admin.activityCenter.form.bannerHtmlPlaceholder')"></textarea>
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.activityCenter.form.bannerHtmlHint') }}</p>
        </div>
        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.activityCenter.form.type') }}</label>
            <Select v-model="form.type" :options="formTypeOptions" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.activityCenter.form.status') }}</label>
            <Select v-model="form.status" :options="formStatusOptions" />
          </div>
        </div>
        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.activityCenter.form.startsAt') }}</label>
            <input v-model="form.starts_at_str" type="datetime-local" class="input" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.activityCenter.form.endsAt') }}</label>
            <input v-model="form.ends_at_str" type="datetime-local" class="input" />
          </div>
        </div>
        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.activityCenter.form.refId') }}</label>
            <input v-model="form.ref_id" class="input" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.activityCenter.form.sortOrder') }}</label>
            <input v-model.number="form.sort_order" type="number" min="0" class="input" />
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.activityCenter.form.content') }}</label>
          <textarea v-model="form.content" rows="6" class="input"></textarea>
        </div>
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
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/core/stores/appStore'
import { getPersistedPageSize } from '@/common/composables/usePersistedPageSize'
import { formatDateTime, formatDateTimeLocalInput, parseDateTimeLocalInput } from '@/core/utils/format'
import adminActivityCenterAPI from '@/features/activity-center/data/datasources/adminActivityCenterDatasource'
import type { ActivityCampaign } from '@/types'
import type { Column } from '@/common/types/uiTypes'

import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import TablePageLayout from '@/common/widgets/layout/TablePageLayout.vue'
import DataTable from '@/common/widgets/data/DataTable.vue'
import Pagination from '@/common/widgets/data/Pagination.vue'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import ConfirmDialog from '@/common/widgets/feedback/ConfirmDialog.vue'
import EmptyState from '@/common/widgets/feedback/EmptyState.vue'
import Select from '@/common/widgets/forms/Select.vue'
import Icon from '@/common/widgets/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

const campaigns = ref<ActivityCampaign[]>([])
const loading = ref(false)
const saving = ref(false)
const searchQuery = ref('')
const showDialog = ref(false)
const showDeleteDialog = ref(false)
const editingCampaign = ref<ActivityCampaign | null>(null)
const deletingCampaign = ref<ActivityCampaign | null>(null)

const filters = reactive({
  status: '',
  type: ''
})

const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0
})

const sortState = reactive({
  sort_by: 'created_at',
  sort_order: 'desc' as 'asc' | 'desc'
})

const form = reactive({
  title: '',
  subtitle: '',
  banner_html: '',
  type: 'custom' as ActivityCampaign['type'],
  ref_id: '',
  status: 'draft' as ActivityCampaign['status'],
  starts_at_str: '',
  ends_at_str: '',
  sort_order: 0,
  content: ''
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

const typeOptions = computed(() => [
  { value: '', label: t('admin.activityCenter.filters.allTypes') },
  { value: 'lottery', label: t('admin.activityCenter.types.lottery') },
  { value: 'redeem', label: t('admin.activityCenter.types.redeem') },
  { value: 'external_link', label: t('admin.activityCenter.types.external_link') },
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

function resetForm() {
  form.title = ''
  form.subtitle = ''
  form.banner_html = ''
  form.type = 'custom'
  form.ref_id = ''
  form.status = 'draft'
  form.starts_at_str = ''
  form.ends_at_str = ''
  form.sort_order = 0
  form.content = ''
}

function fillForm(item: ActivityCampaign) {
  form.title = item.title
  form.subtitle = item.subtitle
  form.banner_html = item.banner_html || ''
  form.type = item.type
  form.ref_id = item.ref_id
  form.status = item.status
  form.starts_at_str = item.starts_at ? formatDateTimeLocalInput(Math.floor(new Date(item.starts_at).getTime() / 1000)) : ''
  form.ends_at_str = item.ends_at ? formatDateTimeLocalInput(Math.floor(new Date(item.ends_at).getTime() / 1000)) : ''
  form.sort_order = item.sort_order
  form.content = item.content
}

function openCreate() {
  editingCampaign.value = null
  resetForm()
  showDialog.value = true
}

function openEdit(item: ActivityCampaign) {
  editingCampaign.value = item
  fillForm(item)
  showDialog.value = true
}

function closeDialog() {
  showDialog.value = false
  editingCampaign.value = null
}

function formatTime(value?: string) {
  return value ? formatDateTime(value) : t('admin.activityCenter.never')
}

function typeLabel(value: string) {
  return t(`admin.activityCenter.types.${value}`)
}

function statusLabel(row: ActivityCampaign) {
  if (row.status === 'draft') return t('admin.activityCenter.status.draft')
  const now = new Date()
  if (row.status === 'active' && row.starts_at && new Date(row.starts_at) > now) {
    return t('admin.activityCenter.status.scheduled')
  }
  if (row.status === 'active') return t('admin.activityCenter.status.live')
  return t('admin.activityCenter.status.archived')
}

function statusClass(row: ActivityCampaign) {
  if (row.status === 'draft') return 'badge-gray'
  if (row.status === 'active' && row.starts_at && new Date(row.starts_at) > new Date()) return 'badge-warning'
  if (row.status === 'active') return 'badge-success'
  return 'badge-gray'
}

let abortController: AbortController | null = null

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

function handlePageChange(page: number) {
  pagination.page = page
  reload()
}

function handlePageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  reload()
}

function handleSort(key: string, order: 'asc' | 'desc') {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  reload()
}

let searchTimer: ReturnType<typeof setTimeout> | null = null
function handleSearch() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    pagination.page = 1
    reload()
  }, 300)
}

async function handleSave() {
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
      ref_id: form.ref_id,
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

onMounted(() => {
  void reload()
})

onUnmounted(() => {
  if (searchTimer) clearTimeout(searchTimer)
  abortController?.abort()
})
</script>
