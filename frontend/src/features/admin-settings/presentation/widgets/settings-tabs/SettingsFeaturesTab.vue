<template>
        <div class="space-y-6">

        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.features.channelMonitor.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.features.channelMonitor.description') }}
            </p>
            <p class="mt-1.5 text-xs">
              <RouterLink
                to="/admin/channels/monitor"
                class="inline-flex items-center gap-1 text-primary-600 hover:underline dark:text-primary-400"
              >
                {{ t('admin.settings.features.channelMonitor.configureLink') }}
                <span aria-hidden="true">→</span>
              </RouterLink>
            </p>
          </div>
          <div class="space-y-5 p-6">
            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.features.channelMonitor.enabled') }}
                </label>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.features.channelMonitor.enabledHint') }}
                </p>
              </div>
              <Toggle v-model="form.channel_monitor_enabled" />
            </div>

            <div v-if="form.channel_monitor_enabled">
              <label class="input-label">
                {{ t('admin.settings.features.channelMonitor.defaultInterval') }}
                <span class="text-red-500">*</span>
              </label>
              <input
                v-model.number="form.channel_monitor_default_interval_seconds"
                type="number"
                min="15"
                max="3600"
                class="input"
              />
              <p class="mt-1 text-xs text-gray-400">
                {{ t('admin.settings.features.channelMonitor.defaultIntervalHint') }}
              </p>
            </div>

            <div v-if="form.channel_monitor_enabled">
              <label class="input-label">
                {{ t('admin.settings.features.channelMonitor.latencyUnit') }}
              </label>
              <select
                v-model="form.channel_monitor_latency_unit"
                class="input"
              >
                <option value="ms">
                  {{ t('admin.settings.features.channelMonitor.latencyUnitMs') }}
                </option>
                <option value="s">
                  {{ t('admin.settings.features.channelMonitor.latencyUnitSeconds') }}
                </option>
              </select>
              <p class="mt-1 text-xs text-gray-400">
                {{ t('admin.settings.features.channelMonitor.latencyUnitHint') }}
              </p>
            </div>

            <div v-if="form.channel_monitor_enabled" class="flex items-center justify-between gap-6">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.features.channelMonitor.publicShare') }}
                </label>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.features.channelMonitor.publicShareHint') }}
                </p>
              </div>
              <Toggle v-model="form.channel_monitor_public_share_enabled" />
            </div>

            <div
              v-if="form.channel_monitor_enabled && form.channel_monitor_public_share_enabled"
              class="flex items-center justify-between gap-6"
            >
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.features.channelMonitor.publicShareRequireAuth') }}
                </label>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.features.channelMonitor.publicShareRequireAuthHint') }}
                </p>
              </div>
              <Toggle v-model="form.channel_monitor_public_share_require_auth" />
            </div>
          </div>
        </div>

        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.features.supportChat.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.features.supportChat.description') }}
            </p>
          </div>
          <div class="space-y-5 p-6">
            <div class="flex items-center justify-between gap-6">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.features.supportChat.enabled') }}
                </label>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.features.supportChat.enabledHint') }}
                </p>
              </div>
              <Toggle v-model="form.support_chat_enabled" />
            </div>
          </div>
        </div>

        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.features.availableChannels.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.features.availableChannels.description') }}
            </p>
            <p class="mt-1.5 text-xs">
              <RouterLink
                to="/admin/channels/pricing"
                class="inline-flex items-center gap-1 text-primary-600 hover:underline dark:text-primary-400"
              >
                {{ t('admin.settings.features.availableChannels.configureLink') }}
                <span aria-hidden="true">→</span>
              </RouterLink>
            </p>
          </div>
          <div class="space-y-5 p-6">
            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.features.availableChannels.enabled') }}
                </label>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.features.availableChannels.enabledHint') }}
                </p>
              </div>
              <Toggle v-model="form.available_channels_enabled" />
            </div>
          </div>
        </div>

        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.features.modelPlaza.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.features.modelPlaza.description') }}
            </p>
          </div>
          <div class="space-y-5 p-6">
            <div class="flex items-center justify-between gap-6">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.features.modelPlaza.enabled') }}
                </label>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.features.modelPlaza.enabledHint') }}
                </p>
              </div>
              <Toggle v-model="form.model_plaza_enabled" />
            </div>

            <div v-if="form.model_plaza_enabled" class="flex items-center justify-between gap-6">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.features.modelPlaza.requireAuth') }}
                </label>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.features.modelPlaza.requireAuthHint') }}
                </p>
              </div>
              <Toggle v-model="form.model_plaza_require_auth" />
            </div>

            <div v-if="form.model_plaza_enabled" class="flex items-center justify-between gap-6">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.features.modelPlaza.autoPublicModels') }}
                </label>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.features.modelPlaza.autoPublicModelsHint') }}
                </p>
              </div>
              <Toggle v-model="form.model_plaza_auto_public_models" />
            </div>

            <div v-if="form.model_plaza_enabled">
              <label class="input-label">
                {{ t('admin.settings.features.modelPlaza.priceDescription') }}
              </label>
              <textarea
                v-model="form.model_plaza_description"
                rows="4"
                class="input resize-y"
                :placeholder="t('admin.settings.features.modelPlaza.priceDescriptionHint')"
              />
            </div>
          </div>
        </div>

        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.features.mediaStudio.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.features.mediaStudio.description') }}
            </p>
          </div>
          <div class="space-y-5 p-6">
            <div class="flex items-center justify-between gap-6">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.features.mediaStudio.enabled') }}
                </label>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.features.mediaStudio.enabledHint') }}
                </p>
              </div>
              <Toggle v-model="form.media_studio_enabled" />
            </div>
          </div>
        </div>

        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.features.riskControl.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.features.riskControl.description') }}
            </p>
            <p class="mt-1.5 text-xs">
              <RouterLink
                to="/admin/risk-control"
                class="inline-flex items-center gap-1 text-primary-600 hover:underline dark:text-primary-400"
              >
                {{ t('admin.settings.features.riskControl.configureLink') }}
                <span aria-hidden="true">→</span>
              </RouterLink>
            </p>
          </div>
          <div class="space-y-5 p-6">
            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.features.riskControl.enabled') }}
                </label>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.features.riskControl.enabledHint') }}
                </p>
              </div>
              <Toggle v-model="form.risk_control_enabled" />
            </div>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.features.riskControl.cyberSessionBlock') }}
                </label>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.features.riskControl.cyberSessionBlockHint') }}
                </p>
              </div>
              <Toggle v-model="form.cyber_session_block_enabled" />
            </div>

            <div v-if="form.cyber_session_block_enabled">
              <label class="input-label">
                {{ t('admin.settings.features.riskControl.cyberSessionBlockTTL') }}
                <span class="text-red-500">*</span>
              </label>
              <input
                v-model.number="form.cyber_session_block_ttl_seconds"
                type="number"
                min="1"
                class="input"
              />
            </div>
          </div>
        </div>

        <!-- Affiliate (邀请返利) feature card -->
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.features.affiliate.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.features.affiliate.description') }}
            </p>
          </div>
          <div class="space-y-5 p-6">
            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.settings.features.affiliate.enabled') }}
                </label>
                <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.features.affiliate.enabledHint') }}
                </p>
              </div>
              <Toggle v-model="form.affiliate_enabled" />
            </div>

            <div v-if="form.affiliate_enabled" class="space-y-6">
              <div class="flex items-center justify-between">
                <div>
                  <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.features.affiliate.adminRechargeRebate') }}
                  </label>
                  <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.settings.features.affiliate.adminRechargeRebateHint') }}
                  </p>
                </div>
                <Toggle v-model="form.affiliate_admin_recharge_enabled" />
              </div>

              <div>
                <label class="input-label">
                  {{ t('admin.settings.features.affiliate.rebateRate') }}
                </label>
                <div class="relative">
                  <input
                    v-model.number="form.affiliate_rebate_rate"
                    type="number"
                    step="0.01"
                    min="0"
                    max="100"
                    class="input pr-8"
                    placeholder="20"
                  />
                  <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-gray-400">%</span>
                </div>
                <p class="mt-1 text-xs text-gray-400">
                  {{ t('admin.settings.features.affiliate.rebateRateHint') }}
                </p>
              </div>

              <div>
                <label class="input-label">
                  {{ t('admin.settings.features.affiliate.freezeHours') }}
                </label>
                <input
                  v-model.number="form.affiliate_rebate_freeze_hours"
                  type="number"
                  step="1"
                  min="0"
                  max="720"
                  class="input"
                />
                <p class="mt-1 text-xs text-gray-400">
                  {{ t('admin.settings.features.affiliate.freezeHoursDesc') }}
                </p>
              </div>

              <div>
                <label class="input-label">
                  {{ t('admin.settings.features.affiliate.durationDays') }}
                </label>
                <input
                  v-model.number="form.affiliate_rebate_duration_days"
                  type="number"
                  step="1"
                  min="0"
                  max="3650"
                  class="input"
                />
                <p class="mt-1 text-xs text-gray-400">
                  {{ t('admin.settings.features.affiliate.durationDaysDesc') }}
                </p>
              </div>

              <div>
                <label class="input-label">
                  {{ t('admin.settings.features.affiliate.perInviteeCap') }}
                </label>
                <input
                  v-model.number="form.affiliate_rebate_per_invitee_cap"
                  type="number"
                  step="0.01"
                  min="0"
                  class="input"
                />
                <p class="mt-1 text-xs text-gray-400">
                  {{ t('admin.settings.features.affiliate.perInviteeCapDesc') }}
                </p>
              </div>

              <!-- 专属用户管理 -->
              <div class="border-t border-gray-100 pt-6 dark:border-dark-700">
                <div class="mb-3 flex items-center justify-between">
                  <div>
                    <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                      {{ t('admin.settings.features.affiliate.customUsers.title') }}
                    </h3>
                    <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                      {{ t('admin.settings.features.affiliate.customUsers.description') }}
                    </p>
                  </div>
                  <button
                    type="button"
                    class="btn btn-primary btn-sm"
                    @click="openAffiliateModal(null)"
                  >
                    + {{ t('admin.settings.features.affiliate.customUsers.addButton') }}
                  </button>
                </div>

                <div class="mb-3 flex items-center gap-2">
                  <input
                    v-model="affiliateState.search"
                    type="text"
                    class="input flex-1"
                    :placeholder="t('admin.settings.features.affiliate.customUsers.searchPlaceholder')"
                    @input="onAffiliateSearchInput"
                  />
                  <button
                    v-if="affiliateState.selected.length > 0"
                    type="button"
                    class="btn btn-secondary btn-sm"
                    @click="openAffiliateBatchModal"
                  >
                    {{ t('admin.settings.features.affiliate.customUsers.batchButton', { count: affiliateState.selected.length }) }}
                  </button>
                </div>

                <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-700">
                  <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
                    <thead class="bg-gray-50 dark:bg-dark-800">
                      <tr>
                        <th class="px-3 py-2 text-left">
                          <input
                            type="checkbox"
                            :checked="affiliateState.entries.length > 0 && affiliateState.selected.length === affiliateState.entries.length"
                            @change="toggleAffiliateSelectAll"
                          />
                        </th>
                        <th class="px-3 py-2 text-left text-xs font-medium uppercase text-gray-500">{{ t('admin.settings.features.affiliate.customUsers.col.email') }}</th>
                        <th class="px-3 py-2 text-left text-xs font-medium uppercase text-gray-500">{{ t('admin.settings.features.affiliate.customUsers.col.username') }}</th>
                        <th class="px-3 py-2 text-left text-xs font-medium uppercase text-gray-500">{{ t('admin.settings.features.affiliate.customUsers.col.code') }}</th>
                        <th class="px-3 py-2 text-left text-xs font-medium uppercase text-gray-500">{{ t('admin.settings.features.affiliate.customUsers.col.rate') }}</th>
                        <th class="px-3 py-2 text-left text-xs font-medium uppercase text-gray-500">{{ t('admin.settings.features.affiliate.customUsers.col.actions') }}</th>
                      </tr>
                    </thead>
                    <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-900">
                      <tr v-if="affiliateState.loading">
                        <td colspan="6" class="px-3 py-6 text-center text-sm text-gray-500">
                          {{ t('common.loading') }}
                        </td>
                      </tr>
                      <tr v-else-if="affiliateState.entries.length === 0">
                        <td colspan="6" class="px-3 py-6 text-center text-sm text-gray-500">
                          {{ t('admin.settings.features.affiliate.customUsers.empty') }}
                        </td>
                      </tr>
                      <tr v-for="entry in affiliateState.entries" :key="entry.user_id">
                        <td class="px-3 py-2">
                          <input
                            type="checkbox"
                            :checked="affiliateState.selected.includes(entry.user_id)"
                            @change="toggleAffiliateSelect(entry.user_id)"
                          />
                        </td>
                        <td class="px-3 py-2 text-sm text-gray-900 dark:text-white">{{ entry.email }}</td>
                        <td class="px-3 py-2 text-sm text-gray-600 dark:text-gray-300">{{ entry.username }}</td>
                        <td class="px-3 py-2 text-sm font-mono">
                          {{ entry.aff_code }}
                          <span
                            v-if="entry.aff_code_custom"
                            class="ml-1 inline-block rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
                          >{{ t('admin.settings.features.affiliate.customUsers.customBadge') }}</span>
                        </td>
                        <td class="px-3 py-2 text-sm">
                          <span v-if="entry.aff_rebate_rate_percent != null">{{ entry.aff_rebate_rate_percent }}%</span>
                          <span v-else class="text-gray-400">{{ t('admin.settings.features.affiliate.customUsers.useGlobal') }}</span>
                        </td>
                        <td class="px-3 py-2 text-sm">
                          <div class="flex items-center gap-2">
                            <button type="button" class="text-primary-600 hover:underline" @click="openAffiliateModal(entry)">
                              {{ t('common.edit') }}
                            </button>
                            <button
                              type="button"
                              class="text-red-600 hover:underline"
                              @click="askResetAffiliateUser(entry)"
                            >
                              {{ t('common.delete') }}
                            </button>
                          </div>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>

                <div v-if="affiliateState.total > affiliateState.pageSize" class="mt-3 flex items-center justify-between text-sm">
                  <span class="text-gray-500">
                    {{ t('admin.settings.features.affiliate.customUsers.totalLabel', { total: affiliateState.total }) }}
                  </span>
                  <div class="flex items-center gap-2">
                    <button
                      type="button"
                      class="btn btn-secondary btn-sm"
                      :disabled="affiliateState.page <= 1"
                      @click="changeAffiliatePage(affiliateState.page - 1)"
                    >
                      {{ t('pagination.previous') }}
                    </button>
                    <span class="text-gray-500">{{ affiliateState.page }} / {{ Math.max(1, Math.ceil(affiliateState.total / affiliateState.pageSize)) }}</span>
                    <button
                      type="button"
                      class="btn btn-secondary btn-sm"
                      :disabled="affiliateState.page >= Math.ceil(affiliateState.total / affiliateState.pageSize)"
                      @click="changeAffiliatePage(affiliateState.page + 1)"
                    >
                      {{ t('pagination.next') }}
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Affiliate add/edit modal -->
        <div
          v-if="affiliateModal.open"
          class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
          @click.self="closeAffiliateModal"
        >
          <div class="w-full max-w-md rounded-lg bg-white p-6 shadow-xl dark:bg-dark-900">
            <h3 class="mb-4 text-lg font-semibold">
              {{ affiliateModal.mode === 'add' ? t('admin.settings.features.affiliate.modal.addTitle') : t('admin.settings.features.affiliate.modal.editTitle') }}
            </h3>
            <div class="space-y-4">
              <div v-if="affiliateModal.mode === 'add'">
                <label class="input-label">{{ t('admin.settings.features.affiliate.modal.userLabel') }}</label>
                <!-- Chip showing the picked user; clicking it re-opens the search -->
                <div
                  v-if="affiliateModal.selectedUser"
                  class="flex items-center justify-between rounded-md border border-primary-200 bg-primary-50 px-3 py-2 dark:border-primary-700/50 dark:bg-primary-900/20"
                >
                  <div class="text-sm">
                    <span class="font-medium text-gray-900 dark:text-white">{{ affiliateModal.selectedUser.email }}</span>
                    <span class="ml-1 text-xs text-gray-500">({{ affiliateModal.selectedUser.username }})</span>
                  </div>
                  <button
                    type="button"
                    class="text-lg leading-none text-gray-400 hover:text-red-600"
                    :title="t('admin.settings.features.affiliate.modal.changeUser')"
                    @click="clearSelectedAffiliateUser"
                  >
                    ×
                  </button>
                </div>
                <!-- Search input + result dropdown — hidden once a selection is made -->
                <template v-else>
                  <input
                    v-model="affiliateModal.userQuery"
                    type="text"
                    class="input"
                    :placeholder="t('admin.settings.features.affiliate.modal.userPlaceholder')"
                    @input="onAffiliateUserSearchInput"
                  />
                  <div
                    v-if="affiliateModal.userResults.length > 0"
                    class="mt-1 max-h-40 overflow-y-auto rounded border border-gray-200 dark:border-dark-700"
                  >
                    <button
                      v-for="u in affiliateModal.userResults"
                      :key="u.id"
                      type="button"
                      class="w-full px-3 py-1.5 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-800"
                      @click="selectAffiliateUser(u)"
                    >
                      {{ u.email }} <span class="text-xs text-gray-500">({{ u.username }})</span>
                    </button>
                  </div>
                </template>
              </div>
              <div v-else>
                <label class="input-label">{{ t('admin.settings.features.affiliate.modal.userLabel') }}</label>
                <input
                  type="text"
                  class="input"
                  :value="affiliateModal.editingEntry ? affiliateModal.editingEntry.email : ''"
                  disabled
                />
              </div>

              <div>
                <label class="input-label">{{ t('admin.settings.features.affiliate.modal.codeLabel') }}</label>
                <input
                  v-model="affiliateModal.code"
                  type="text"
                  class="input font-mono"
                  :placeholder="t('admin.settings.features.affiliate.modal.codePlaceholder')"
                  maxlength="32"
                />
                <p class="mt-1 text-xs text-gray-400">
                  {{ t('admin.settings.features.affiliate.modal.codeHint') }}
                </p>
              </div>

              <div>
                <label class="input-label">{{ t('admin.settings.features.affiliate.modal.rateLabel') }}</label>
                <div class="relative">
                  <input
                    v-model="affiliateModal.rate"
                    type="number"
                    step="0.01"
                    min="0"
                    max="100"
                    class="input pr-8"
                    :placeholder="t('admin.settings.features.affiliate.modal.ratePlaceholder')"
                  />
                  <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-gray-400">%</span>
                </div>
                <p class="mt-1 text-xs text-gray-400">
                  {{ t('admin.settings.features.affiliate.modal.rateHint') }}
                </p>
              </div>
            </div>

            <div class="mt-6 flex items-center justify-between gap-3">
              <p
                v-if="!affiliateModalCanSubmit"
                class="text-xs text-gray-500 dark:text-gray-400"
              >
                {{ t('admin.settings.features.affiliate.modal.errorEmpty') }}
              </p>
              <span v-else></span>
              <div class="flex gap-2">
                <button type="button" class="btn btn-secondary" @click="closeAffiliateModal">
                  {{ t('common.cancel') }}
                </button>
                <button
                  type="button"
                  class="btn btn-primary"
                  :disabled="affiliateModal.saving || !affiliateModalCanSubmit"
                  @click="submitAffiliateModal"
                >
                  {{ affiliateModal.saving ? t('common.saving') : t('common.save') }}
                </button>
              </div>
            </div>
          </div>
        </div>

        <!-- Affiliate batch rate modal -->
        <div
          v-if="affiliateBatchModal.open"
          class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
          @click.self="affiliateBatchModal.open = false"
        >
          <div class="w-full max-w-md rounded-lg bg-white p-6 shadow-xl dark:bg-dark-900">
            <h3 class="mb-4 text-lg font-semibold">
              {{ t('admin.settings.features.affiliate.batchModal.title', { count: affiliateState.selected.length }) }}
            </h3>
            <p class="mb-4 text-sm text-gray-500">
              {{ t('admin.settings.features.affiliate.batchModal.hint') }}
            </p>
            <div class="relative">
              <input
                v-model="affiliateBatchModal.rate"
                type="number"
                step="0.01"
                min="0"
                max="100"
                class="input pr-8"
                :placeholder="t('admin.settings.features.affiliate.batchModal.placeholder')"
              />
              <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-gray-400">%</span>
            </div>
            <p class="mt-2 text-xs text-gray-400">
              {{ t('admin.settings.features.affiliate.batchModal.clearHint') }}
            </p>
            <div class="mt-6 flex justify-end gap-2">
              <button type="button" class="btn btn-secondary" @click="affiliateBatchModal.open = false">
                {{ t('common.cancel') }}
              </button>
              <button
                type="button"
                class="btn btn-primary"
                :disabled="affiliateBatchModal.saving"
                @click="submitAffiliateBatchModal"
              >
                {{ affiliateBatchModal.saving ? t('common.saving') : t('common.save') }}
              </button>
            </div>
          </div>
        </div>

        </div><!-- /Tab: Features -->
</template>

<script setup lang="ts">
import { RouterLink } from 'vue-router'
import Toggle from '@/common/widgets/forms/Toggle.vue'
import { useSettingsPageContext } from '@/features/admin-settings/presentation/composables/settingsPageContext'

const { affiliateBatchModal, affiliateModal, affiliateModalCanSubmit, affiliateState, askResetAffiliateUser, changeAffiliatePage, clearSelectedAffiliateUser, closeAffiliateModal, form, onAffiliateSearchInput, onAffiliateUserSearchInput, openAffiliateBatchModal, openAffiliateModal, selectAffiliateUser, submitAffiliateBatchModal, submitAffiliateModal, t, toggleAffiliateSelect, toggleAffiliateSelectAll } = useSettingsPageContext()
</script>
