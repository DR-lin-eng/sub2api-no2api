<template>
  <section class="space-y-6 rounded-xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-800">
    <div class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.permissionGroups.title') }}</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.permissionGroups.description') }}</p>
      </div>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="loading || saving || !loaded" @click="addGroup">
        {{ t('admin.settings.permissionGroups.add') }}
      </button>
    </div>

    <div v-if="loading" class="py-8 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>
    <div v-if="error" role="alert" class="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300">
      {{ error }}
      <button v-if="!loaded" type="button" class="btn btn-secondary btn-sm ml-3" :disabled="loading" @click="load">{{ t('common.retry') }}</button>
    </div>
    <div v-if="loaded" class="space-y-4">
      <article v-for="group in groups" :key="group.id" class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
        <div class="mb-4 flex items-start gap-3">
          <div class="min-w-0 flex-1">
            <label :for="`permission-group-${group.id}`" class="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.permissionGroups.name') }}</label>
            <input :id="`permission-group-${group.id}`" :disabled="saving" v-model.trim="group.name" type="text" maxlength="50" class="input w-full" />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ group.built_in ? t('admin.settings.permissionGroups.builtIn') : group.id }}</p>
          </div>
          <button v-if="!group.built_in" type="button" class="btn btn-danger btn-sm mt-6" :disabled="saving" @click="removeGroup(group.id)">
            {{ t('common.delete') }}
          </button>
        </div>
        <div>
          <p class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.permissionGroups.permissions') }}</p>
          <div class="grid grid-cols-1 gap-2 md:grid-cols-2">
            <label v-for="permission in permissionDefinitions" :key="permission.key" class="flex cursor-pointer items-start gap-2 rounded-md border border-gray-100 p-2 text-sm dark:border-dark-700">
              <input :disabled="saving" v-model="group.permissions" type="checkbox" :value="permission.key" class="mt-1" />
              <span>
                <span class="block font-medium text-gray-800 dark:text-gray-200">{{ t(`admin.settings.permissionGroups.catalog.${permission.key.replace(/\./g, '_')}.name`) }}</span>
                <span class="block text-xs text-gray-500 dark:text-gray-400">{{ t(`admin.settings.permissionGroups.catalog.${permission.key.replace(/\./g, '_')}.description`) }}</span>
              </span>
            </label>
          </div>
        </div>
      </article>
    </div>

    <div class="flex justify-end border-t border-gray-100 pt-4 dark:border-dark-700">
      <button type="button" class="btn btn-primary" :disabled="loading || saving || !loaded" @click="save">
        {{ saving ? t('admin.settings.saving') : t('admin.settings.permissionGroups.save') }}
      </button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getPermissionGroups, updatePermissionGroups } from '@/features/admin-settings/data/datasources/permissionGroupsDatasource'
import type { PermissionDefinition, PermissionGroup } from '@/features/admin-settings/data/dtos/permissionGroupDtos'
import { useAppStore } from '@/core/stores/appStore'

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(true)
const loaded = ref(false)
const saving = ref(false)
const error = ref('')
const groups = ref<PermissionGroup[]>([])
const permissionDefinitions = ref<PermissionDefinition[]>([])

function newGroupID(): string {
  return `custom_${crypto.randomUUID().replace(/-/g, '').slice(0, 12)}`
}

function addGroup(): void {
  groups.value.push({ id: newGroupID(), name: t('admin.settings.permissionGroups.newGroup'), permissions: [], built_in: false })
}

function removeGroup(id: string): void {
  groups.value = groups.value.filter((group) => group.id !== id)
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const data = await getPermissionGroups()
    loaded.value = true
    groups.value = data.groups
    permissionDefinitions.value = data.permissions
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : t('admin.settings.permissionGroups.loadFailed')
  } finally {
    loading.value = false
  }
}

async function save(): Promise<void> {
  saving.value = true
  error.value = ''
  try {
    const data = await updatePermissionGroups(groups.value)
    loaded.value = true
    groups.value = data.groups
    permissionDefinitions.value = data.permissions
    appStore.showSuccess(t('admin.settings.permissionGroups.saved'))
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : t('admin.settings.permissionGroups.saveFailed')
    appStore.showError(error.value)
  } finally {
    saving.value = false
  }
}

onMounted(() => { void load() })
</script>
