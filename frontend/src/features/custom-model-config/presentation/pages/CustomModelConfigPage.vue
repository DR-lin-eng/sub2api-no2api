<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
          <!-- Left: Search -->
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <RouterLink
              :to="{ path: '/admin/settings', query: { tab: 'features' } }"
              class="btn btn-secondary"
              :title="t('admin.customModelConfig.actions.backToSettings')"
              :aria-label="t('admin.customModelConfig.actions.backToSettings')"
            >
              <Icon name="arrowLeft" size="md" />
            </RouterLink>
            <div class="relative w-full sm:w-64">
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
              />
              <input
                v-model="searchQuery"
                type="text"
                :placeholder="t('common.search')"
                class="input pl-10"
                @input="handleSearch"
              />
            </div>
          </div>

          <!-- Right: Actions -->
          <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
            <button
              @click="loadConfigs"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh', 'Refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button @click="openCreateDialog" class="btn btn-primary">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.customModelConfig.actions.create') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="filteredConfigs"
          :loading="loading"
          :server-side-sort="false"
          default-sort-key="created_at"
          default-sort-order="desc"
        >
          <template #cell-model_name="{ value }">
            <span class="font-mono text-sm font-medium text-gray-900 dark:text-white">{{ value }}</span>
          </template>

          <template #cell-capabilities="{ value }">
            <div class="flex flex-wrap gap-1">
              <span
                v-for="cap in value"
                :key="cap"
                class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
                :class="getCapabilityClass(cap)"
              >
                {{ cap }}
              </span>
            </div>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-2">
              <button
                @click="openEditDialog(row)"
                class="text-gray-600 hover:text-primary-600 dark:text-gray-400 dark:hover:text-primary-400"
                :title="t('common.edit', 'Edit')"
              >
                <Icon name="edit" size="md" />
              </button>
              <button
                @click="handleDelete(row)"
                class="text-gray-600 hover:text-red-600 dark:text-gray-400 dark:hover:text-red-400"
                :title="t('common.delete', 'Delete')"
              >
                <Icon name="trash" size="md" />
              </button>
            </div>
          </template>
        </DataTable>
      </template>
    </TablePageLayout>

    <!-- Create/Edit Dialog -->
    <CustomModelConfigDialog
      v-if="dialogVisible"
      :visible="dialogVisible"
      :config="editingConfig"
      @close="closeDialog"
      @saved="handleSaved"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import AppLayout from '@/common/widgets/layout/AppLayout.vue';
import TablePageLayout from '@/common/widgets/layout/TablePageLayout.vue';
import DataTable from '@/common/widgets/data/DataTable.vue';
import Icon from '@/common/widgets/icons/Icon.vue';
import CustomModelConfigDialog from '../widgets/CustomModelConfigDialog.vue';
import { customModelConfigDatasource } from '../../data/datasources/customModelConfigDatasource';
import { initializeModelCapabilities } from '../../domain/services/modelCapabilityService';
import type { CustomModelConfig, ModelCapability } from '../../domain/entities/customModelConfig';

const { t } = useI18n();

const loading = ref(false);
const configs = ref<CustomModelConfig[]>([]);
const searchQuery = ref('');
const dialogVisible = ref(false);
const editingConfig = ref<CustomModelConfig | null>(null);

const columns = computed(() => [
  { key: 'model_name', label: t('admin.customModelConfig.table.modelName'), sortable: true },
  { key: 'capabilities', label: t('admin.customModelConfig.table.capabilities'), sortable: false },
  { key: 'actions', label: t('admin.customModelConfig.table.actions'), sortable: false, width: '100px' },
]);

const filteredConfigs = computed(() => {
  if (!searchQuery.value.trim()) return configs.value;
  const query = searchQuery.value.toLowerCase();
  return configs.value.filter((c) => c.model_name.toLowerCase().includes(query));
});

function getCapabilityClass(cap: ModelCapability): string {
  const classes = {
    image: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300',
    video: 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300',
    audio: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300',
  };
  return classes[cap] || 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-300';
}

async function loadConfigs() {
  loading.value = true;
  try {
    configs.value = await customModelConfigDatasource.getAll();
    // Refresh cache
    initializeModelCapabilities(configs.value);
  } catch (error) {
    console.error('Failed to load custom model configs:', error);
  } finally {
    loading.value = false;
  }
}

function handleSearch() {
  // Computed property handles filtering
}

function openCreateDialog() {
  editingConfig.value = null;
  dialogVisible.value = true;
}

function openEditDialog(config: CustomModelConfig) {
  editingConfig.value = config;
  dialogVisible.value = true;
}

function closeDialog() {
  dialogVisible.value = false;
  editingConfig.value = null;
}

async function handleSaved() {
  closeDialog();
  await loadConfigs();
}

async function handleDelete(config: CustomModelConfig) {
  if (!confirm(t('admin.customModelConfig.actions.deleteConfirm'))) {
    return;
  }

  try {
    await customModelConfigDatasource.delete(config.id);
    await loadConfigs();
  } catch (error) {
    console.error('Failed to delete model config:', error);
    alert(t('admin.customModelConfig.actions.deleteFailed', 'Failed to delete model configuration'));
  }
}

onMounted(() => {
  loadConfigs();
});
</script>
