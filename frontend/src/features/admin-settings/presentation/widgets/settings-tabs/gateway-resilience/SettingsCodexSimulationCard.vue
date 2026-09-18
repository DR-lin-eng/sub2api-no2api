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

        <div class="flex items-center justify-between gap-4 border-t border-gray-100 pt-5 dark:border-dark-700">
          <div>
            <label class="font-medium text-gray-900 dark:text-white">
              {{ t("admin.settings.codexSimulation.experimentalTransport") }}
            </label>
            <p class="text-sm text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.codexSimulation.experimentalTransportHint") }}
            </p>
          </div>
          <fieldset
            class="m-0 min-w-0 border-0 p-0"
            :disabled="codexSimulationLoadFailed || codexSimulationSaving || !codexSimulationForm.c_level_simulation_enabled"
          >
            <Toggle
              :model-value="codexSimulationForm.experimental_transport_enabled === true"
              @update:model-value="codexSimulationForm.experimental_transport_enabled = $event"
              data-testid="codex-simulation-experimental-transport-toggle"
            />
          </fieldset>
        </div>

        <div class="space-y-4 border-t border-gray-100 pt-5 dark:border-dark-700">
          <div class="flex items-center justify-between gap-4">
            <div>
              <label class="font-medium text-gray-900 dark:text-white">
                {{ t("admin.settings.codexSimulation.turnStateAutoReplay") }}
              </label>
              <p class="text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.codexSimulation.turnStateAutoReplayHint") }}
              </p>
            </div>
            <fieldset
              class="m-0 min-w-0 border-0 p-0"
              :disabled="codexSimulationLoadFailed || codexSimulationSaving || codexSimulationSyncing"
            >
              <Toggle
                v-model="codexSimulationForm.turn_state_auto_replay_enabled"
                data-testid="codex-turn-state-auto-replay-toggle"
              />
            </fieldset>
          </div>
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300" for="codex-turn-state-target-length">
              {{ t("admin.settings.codexSimulation.turnStateTargetLength") }}
            </label>
            <input
              id="codex-turn-state-target-length"
              v-model.number="codexSimulationForm.turn_state_target_length"
              type="number"
              min="1"
              max="8192"
              step="1"
              class="input w-full sm:max-w-48"
              :disabled="codexSimulationLoadFailed || codexSimulationSaving || codexSimulationSyncing"
              data-testid="codex-turn-state-target-length"
            />
          </div>
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300" for="codex-turn-state-watch-models">
              {{ t("admin.settings.codexSimulation.turnStateWatchModels") }}
            </label>
            <textarea
              id="codex-turn-state-watch-models"
              v-model="codexTurnStateWatchModelsText"
              class="input min-h-24 w-full font-mono text-xs"
              :disabled="codexSimulationLoadFailed || codexSimulationSaving || codexSimulationSyncing"
              :placeholder="t('admin.settings.codexSimulation.turnStateWatchModelsPlaceholder')"
              data-testid="codex-turn-state-watch-models"
              spellcheck="false"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.codexSimulation.turnStateWatchModelsHint", { count: codexSimulationForm.turn_state_watch_models.length }) }}
            </p>
          </div>
          <div class="flex items-center justify-between gap-4 border-t border-gray-100 pt-4 dark:border-dark-700">
            <div>
              <label class="font-medium text-gray-900 dark:text-white">
                {{ t("admin.settings.codexSimulation.turnStateReplay") }}
              </label>
              <p class="text-sm text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.codexSimulation.turnStateReplayHint") }}
              </p>
            </div>
            <fieldset
              class="m-0 min-w-0 border-0 p-0"
              :disabled="codexSimulationLoadFailed || codexSimulationSaving || codexSimulationSyncing"
            >
              <Toggle
                v-model="codexSimulationForm.turn_state_replay_enabled"
                data-testid="codex-turn-state-replay-toggle"
              />
            </fieldset>
          </div>
          <div>
            <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
              <label class="text-sm font-medium text-gray-700 dark:text-gray-300" for="codex-turn-states">
                {{ t("admin.settings.codexSimulation.turnStates") }}
              </label>
              <button
                type="button"
                class="btn btn-secondary btn-sm inline-flex items-center gap-1.5"
                :disabled="codexSimulationLoadFailed || codexSimulationSaving || codexSimulationSyncing || codexTurnStateDraftDirty"
                data-testid="codex-turn-state-sync"
                @click="syncCodexTurnStatesFromQuality"
              >
                <Icon name="refresh" size="sm" :class="codexSimulationSyncing ? 'animate-spin' : ''" />
                {{ t("admin.settings.codexSimulation.turnStateSync") }}
              </button>
            </div>
            <textarea
              id="codex-turn-states"
              v-model="codexTurnStatesText"
              class="input min-h-28 w-full font-mono text-xs"
              :disabled="codexSimulationLoadFailed || codexSimulationSaving || codexSimulationSyncing"
              :placeholder="t('admin.settings.codexSimulation.turnStatesPlaceholder')"
              data-testid="codex-turn-states"
              spellcheck="false"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.codexSimulation.turnStatesHint", { count: codexSimulationForm.turn_states.length }) }}
            </p>
            <p v-if="codexTurnStateDraftDirty" class="mt-1.5 text-xs text-amber-700 dark:text-amber-300" data-testid="codex-turn-state-unsaved">
              {{ t("admin.settings.codexSimulation.turnStatesSaveBeforeSync") }}
            </p>
          </div>
          <div
            v-if="codexSimulationForm.turn_state_observability"
            class="space-y-4 border-t border-gray-100 pt-4 dark:border-dark-700"
            data-testid="codex-turn-state-observability"
          >
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div>
                <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t("admin.settings.codexSimulation.observabilityTitle") }}
                </h3>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.codexSimulation.observabilityHint") }}
                </p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.codexSimulation.observabilityCurrentNode") }} · {{ formatTimestamp(observability?.generated_at) }}
                </p>
              </div>
              <button
                type="button"
                class="btn btn-secondary btn-sm inline-flex items-center gap-1.5"
                :disabled="codexObservabilityRefreshing"
                data-testid="codex-turn-state-observability-refresh"
                @click="refreshCodexTurnStateObservability"
              >
                <Icon name="refresh" size="sm" :class="codexObservabilityRefreshing ? 'animate-spin' : ''" />
                {{ t("admin.settings.codexSimulation.observabilityRefresh") }}
              </button>
            </div>
            <div class="grid grid-cols-2 gap-3 md:grid-cols-4">
              <div class="border border-gray-200 px-3 py-2 dark:border-dark-600">
                <div class="text-xs text-gray-500 dark:text-gray-400">{{ t("admin.settings.codexSimulation.observabilityTracked") }}</div>
                <div class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ codexSimulationForm.turn_state_observability.items.length }}</div>
              </div>
              <div class="border border-gray-200 px-3 py-2 dark:border-dark-600">
                <div class="text-xs text-gray-500 dark:text-gray-400">{{ t("admin.settings.codexSimulation.observabilityValid") }}</div>
                <div class="mt-1 text-lg font-semibold text-green-600 dark:text-green-400">{{ validTurnStateCount }}</div>
              </div>
              <div class="border border-gray-200 px-3 py-2 dark:border-dark-600">
                <div class="text-xs text-gray-500 dark:text-gray-400">{{ t("admin.settings.codexSimulation.observabilityExpired") }}</div>
                <div class="mt-1 text-lg font-semibold text-amber-600 dark:text-amber-400">{{ expiredTurnStateCount }}</div>
              </div>
              <div class="border border-gray-200 px-3 py-2 dark:border-dark-600">
                <div class="text-xs text-gray-500 dark:text-gray-400">{{ t("admin.settings.codexSimulation.observabilityRotations") }}</div>
                <div class="mt-1 text-lg font-semibold text-red-600 dark:text-red-400">{{ rotationErrorCount }}</div>
              </div>
            </div>
            <div class="overflow-x-auto border border-gray-200 dark:border-dark-600">
              <table class="min-w-full text-left text-xs">
                <thead class="bg-gray-50 text-gray-500 dark:bg-dark-800 dark:text-gray-400">
                  <tr>
                    <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t("admin.settings.codexSimulation.observabilityAccountModel") }}</th>
                    <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t("admin.settings.codexSimulation.observabilityState") }}</th>
                    <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t("admin.settings.codexSimulation.observabilityCipher") }}</th>
                    <th class="whitespace-nowrap px-3 py-2 font-medium">{{ t("admin.settings.codexSimulation.observabilityProbe") }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                  <tr v-for="item in codexSimulationForm.turn_state_observability.items" :key="`${item.account_id}:${item.model}`">
                    <td class="whitespace-nowrap px-3 py-2 align-top text-gray-900 dark:text-white">
                      <div class="font-medium">#{{ item.account_id }} · {{ item.model }}</div>
                      <div class="mt-1 text-gray-500 dark:text-gray-400">{{ item.source }}<span v-if="item.proxy_enabled"> · {{ t("admin.settings.codexSimulation.observabilityProxy") }}</span></div>
                    </td>
                    <td class="whitespace-nowrap px-3 py-2 align-top text-gray-700 dark:text-gray-300">
                      <div>{{ stateLabel(item) }}</div>
                      <div class="mt-1 text-gray-500 dark:text-gray-400">{{ t("admin.settings.codexSimulation.observabilityIssuedAt") }} {{ formatTimestamp(item.state.issued_at) }}</div>
                      <div class="mt-1 text-gray-500 dark:text-gray-400">{{ t("admin.settings.codexSimulation.observabilityEstimatedExpiry") }} {{ formatTimestamp(item.state.estimated_expires_at) }}</div>
                      <div class="mt-1 text-gray-500 dark:text-gray-400">{{ item.state.token_characters }} {{ t("admin.settings.codexSimulation.observabilityCharacters") }} · {{ item.state.token_bytes_known ? `${item.state.token_bytes} B` : t("admin.settings.codexSimulation.observabilityUnknown") }} · {{ item.state.version_hex || t("admin.settings.codexSimulation.observabilityUnknown") }}</div>
                      <div v-if="item.last_response_at && !item.last_response_had_state" class="mt-1 text-amber-600 dark:text-amber-400">{{ t("admin.settings.codexSimulation.observabilityLastResponseMissing") }}</div>
                      <div v-if="item.state_digest" class="mt-1 font-mono text-gray-500 dark:text-gray-400">{{ item.state_digest }}</div>
                    </td>
                    <td class="whitespace-nowrap px-3 py-2 align-top text-gray-700 dark:text-gray-300">
                      <div>{{ cipherLabel(item.encrypted_content.classification) }}</div>
                      <div class="mt-1 text-gray-500 dark:text-gray-400">{{ item.encrypted_content.last_bytes_known ? `${item.encrypted_content.last_bytes} B / ${signedDelta(item.encrypted_content.delta_bytes)}` : t("admin.settings.codexSimulation.observabilityUnknown") }}</div>
                      <div class="mt-1 text-gray-500 dark:text-gray-400">{{ t("admin.settings.codexSimulation.observabilityRotationsShort", { count: item.rotation.count }) }}</div>
                    </td>
                    <td class="whitespace-nowrap px-3 py-2 align-top text-gray-700 dark:text-gray-300">
                      <div>{{ item.probe.in_flight ? t("admin.settings.codexSimulation.observabilityProbeRunning") : t("admin.settings.codexSimulation.observabilityProbeIdle") }}</div>
                      <div class="mt-1 text-gray-500 dark:text-gray-400">{{ formatTimestamp(item.probe.next_probe_at) }}</div>
                    </td>
                  </tr>
                  <tr v-if="codexSimulationForm.turn_state_observability.items.length === 0">
                    <td colspan="4" class="px-3 py-5 text-center text-gray-500 dark:text-gray-400">{{ t("admin.settings.codexSimulation.observabilityEmpty") }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>

        <div class="flex items-center justify-between gap-4 border-t border-gray-100 pt-5 dark:border-dark-700">
          <div>
            <label class="font-medium text-gray-900 dark:text-white">
              {{ t("admin.settings.codexSimulation.cLevelSimulation") }}
            </label>
            <p class="text-sm text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.codexSimulation.cLevelSimulationHint") }}
            </p>
          </div>
          <fieldset
            class="m-0 min-w-0 border-0 p-0"
            :disabled="codexSimulationLoadFailed || codexSimulationSaving"
          >
			<Toggle
				:model-value="codexSimulationForm.c_level_simulation_enabled === true"
				@update:model-value="codexSimulationForm.c_level_simulation_enabled = $event"
              data-testid="codex-simulation-c-level-toggle"
            />
          </fieldset>
        </div>

        <div class="flex items-center justify-between gap-4 border-t border-gray-100 pt-5 dark:border-dark-700">
          <div>
            <label class="font-medium text-gray-900 dark:text-white">
              {{ t("admin.settings.codexSimulation.forceAccountPrewarm") }}
            </label>
            <p class="text-sm text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.codexSimulation.forceAccountPrewarmHint") }}
            </p>
          </div>
          <fieldset
            class="m-0 min-w-0 border-0 p-0"
            :disabled="codexSimulationLoadFailed || codexSimulationSaving"
          >
            <Toggle
              :model-value="codexSimulationForm.codex_prewarm_continuation_force_enabled === true"
              @update:model-value="codexSimulationForm.codex_prewarm_continuation_force_enabled = $event"
              data-testid="codex-prewarm-force-toggle"
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
                  codexSimulationForm.c_level_simulation_enabled ||
                  codexSimulationForm.experimental_transport_enabled ||
                  codexSimulationForm.codex_prewarm_continuation_force_enabled ||
                  codexSimulationForm.turn_state_auto_replay_enabled ||
                  codexSimulationForm.turn_state_replay_enabled ||
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
                  codexSimulationForm.c_level_simulation_enabled ||
                  codexSimulationForm.experimental_transport_enabled ||
                  codexSimulationForm.codex_prewarm_continuation_force_enabled ||
                  codexSimulationForm.turn_state_auto_replay_enabled ||
                  codexSimulationForm.turn_state_replay_enabled ||
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
import { computed } from 'vue'
import Toggle from '@/common/widgets/forms/Toggle.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import { useSettingsPageContext } from '@/features/admin-settings/presentation/composables/settingsPageContext'

const {
  codexSimulationForm,
  codexSimulationLoadFailed,
  codexSimulationLoading,
  codexSimulationSaving,
  codexSimulationSyncing,
  codexObservabilityRefreshing,
  codexTurnStateDraftDirty,
  codexTurnStateWatchModelsText,
  codexTurnStatesText,
  refreshCodexTurnStateObservability,
  restoreOriginalCodexBehavior,
  saveCodexSimulationSettings,
  syncCodexTurnStatesFromQuality,
  t,
} = useSettingsPageContext()

const observability = computed(() => codexSimulationForm.turn_state_observability)
const validTurnStateCount = computed(() => observability.value?.items.filter((item) => item.state.valid && !item.state.expired && item.length_match).length ?? 0)
const expiredTurnStateCount = computed(() => observability.value?.items.filter((item) => item.state.expired).length ?? 0)
const rotationErrorCount = computed(() => observability.value?.items.reduce((sum, item) => sum + item.rotation.count, 0) ?? 0)
const formatTimestamp = (value?: string) => value ? new Date(value).toLocaleString() : '-'
const signedDelta = (value: number) => value > 0 ? `+${value} B` : `${value} B`
const stateLabel = (item: NonNullable<typeof observability.value>['items'][number]) => {
  if (item.state.expired) return t('admin.settings.codexSimulation.observabilityStateExpired')
  if (!item.state.valid) return t('admin.settings.codexSimulation.observabilityStateInvalid')
  if (!item.length_match) return t('admin.settings.codexSimulation.observabilityLengthMismatch')
  return t('admin.settings.codexSimulation.observabilityStateValid')
}
const cipherLabel = (classification: string) => {
  switch (classification) {
    case 'baseline': return t('admin.settings.codexSimulation.observabilityBaseline')
    case 'plus_16_hint': return t('admin.settings.codexSimulation.observabilityPlus16Hint')
    case 'other': return t('admin.settings.codexSimulation.observabilityOtherDelta')
    default: return t('admin.settings.codexSimulation.observabilityUnknown')
  }
}
</script>
