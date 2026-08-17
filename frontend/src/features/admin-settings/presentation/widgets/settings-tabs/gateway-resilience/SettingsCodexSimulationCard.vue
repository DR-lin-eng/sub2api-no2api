<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <div class="flex flex-col items-start gap-3 sm:flex-row sm:justify-between sm:gap-4">
        <div>
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ t("admin.settings.codexSimulation.title") }}
          </h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.codexSimulation.description") }}
          </p>
        </div>
        <span
          class="inline-flex items-center gap-1.5 text-xs font-medium sm:shrink-0"
          :class="
            codexSimulationLoadFailed
              ? 'text-amber-600 dark:text-amber-300'
              : codexSimulationForm.identity_secret_configured
              ? 'text-green-600 dark:text-green-400'
              : 'text-gray-500 dark:text-gray-400'
          "
          data-testid="codex-simulation-secret-status"
        >
          <Icon
            :name="
              codexSimulationLoadFailed
                ? 'refresh'
                : codexSimulationForm.identity_secret_configured
                ? 'checkCircle'
                : 'key'
            "
            size="sm"
          />
          {{
            codexSimulationLoadFailed
              ? t("admin.settings.codexSimulation.secretUnknown")
              : codexSimulationForm.identity_secret_configured
              ? t("admin.settings.codexSimulation.secretConfigured")
              : t("admin.settings.codexSimulation.secretPending")
          }}
        </span>
      </div>
    </div>

    <div class="space-y-5 p-6">
      <div
        v-if="codexSimulationLoading"
        class="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400"
      >
        <Icon name="refresh" size="sm" class="animate-spin" />
        {{ t("common.loading") }}
      </div>

      <template v-else>
        <div
          v-if="codexSimulationLoadFailed"
          class="border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-800 dark:bg-amber-950/40 dark:text-amber-200"
          data-testid="codex-simulation-load-failed"
        >
          {{ t("admin.settings.codexSimulation.loadFailedHint") }}
        </div>

        <div class="flex items-center justify-between gap-4">
          <div>
            <label class="font-medium text-gray-900 dark:text-white">
              {{ t("admin.settings.codexSimulation.fullSimulation") }}
            </label>
            <p class="text-sm text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.codexSimulation.fullSimulationHint") }}
            </p>
          </div>
          <fieldset
            class="m-0 min-w-0 border-0 p-0"
            :disabled="codexSimulationLoadFailed || codexSimulationSaving"
          >
            <Toggle
              v-model="codexSimulationForm.full_simulation_enabled"
              data-testid="codex-simulation-full-toggle"
            />
          </fieldset>
        </div>

        <div class="grid gap-5 border-t border-gray-100 pt-5 dark:border-dark-700 md:grid-cols-2">
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.settings.codexSimulation.continuationMode") }}
            </label>
            <select
              v-model="codexSimulationForm.continuation_mode"
              :disabled="codexSimulationLoadFailed || codexSimulationSaving"
              class="input w-full"
              data-testid="codex-simulation-continuation-mode"
            >
              <option value="off">
                {{ t("admin.settings.codexSimulation.modeOff") }}
              </option>
              <option value="shadow">
                {{ t("admin.settings.codexSimulation.modeShadow") }}
              </option>
              <option value="enforce">
                {{ t("admin.settings.codexSimulation.modeEnforce") }}
              </option>
            </select>
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.codexSimulation.continuationModeHint") }}
            </p>
          </div>

          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.settings.codexSimulation.stateTTL") }}
            </label>
            <input
              v-model.number="codexSimulationForm.state_ttl_seconds"
              :disabled="codexSimulationLoadFailed || codexSimulationSaving"
              type="number"
              min="1"
              step="1"
              class="input w-full"
              data-testid="codex-simulation-state-ttl"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.codexSimulation.stateTTLHint") }}
            </p>
          </div>
        </div>

        <div
          class="border-t border-gray-100 pt-4 text-sm dark:border-dark-700"
          :class="
            codexSimulationLoadFailed
              ? 'text-amber-700 dark:text-amber-300'
              : codexSimulationForm.full_simulation_enabled ||
                  codexSimulationForm.continuation_mode !== 'off'
              ? 'text-amber-700 dark:text-amber-300'
              : 'text-green-700 dark:text-green-300'
          "
          data-testid="codex-simulation-effective-state"
        >
          {{
            codexSimulationLoadFailed
              ? t("admin.settings.codexSimulation.stateUnknown")
              : codexSimulationForm.full_simulation_enabled ||
                  codexSimulationForm.continuation_mode !== "off"
              ? t("admin.settings.codexSimulation.experimentalEnabled")
              : t("admin.settings.codexSimulation.originalBehaviorActive")
          }}
        </div>
        <div class="flex flex-wrap justify-end gap-2 border-t border-gray-100 pt-4 dark:border-dark-700">
          <button
            type="button"
            class="btn btn-secondary btn-sm inline-flex items-center gap-1.5"
            :disabled="codexSimulationSaving"
            data-testid="codex-simulation-restore"
            @click="restoreOriginalCodexBehavior"
          >
            <Icon name="refresh" size="sm" />
            {{ t("admin.settings.codexSimulation.restoreOriginal") }}
          </button>
          <button
            type="button"
            class="btn btn-primary btn-sm inline-flex items-center gap-1.5"
            :disabled="codexSimulationSaving || codexSimulationLoadFailed"
            data-testid="codex-simulation-save"
            @click="saveCodexSimulationSettings"
          >
            <Icon
              :name="codexSimulationSaving ? 'refresh' : 'check'"
              size="sm"
              :class="codexSimulationSaving ? 'animate-spin' : ''"
            />
            {{ codexSimulationSaving ? t("common.saving") : t("common.save") }}
          </button>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import Toggle from '@/common/widgets/forms/Toggle.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import { useSettingsPageContext } from '@/features/admin-settings/presentation/composables/settingsPageContext'

const {
  codexSimulationForm,
  codexSimulationLoadFailed,
  codexSimulationLoading,
  codexSimulationSaving,
  restoreOriginalCodexBehavior,
  saveCodexSimulationSettings,
  t,
} = useSettingsPageContext()
</script>
