<template>
  <AppLayout>
    <div class="space-y-5 pb-12">
      <section class="border-y border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800" aria-labelledby="egress-runtime-title">
        <div class="flex flex-wrap items-center justify-between gap-3 px-4 py-3 sm:px-5">
          <div class="flex min-w-0 items-center gap-3">
            <span
              class="h-2.5 w-2.5 flex-shrink-0 rounded-full"
              :class="runtime?.ready ? 'bg-emerald-500' : runtime?.enabled ? 'bg-amber-500' : 'bg-gray-400'"
            ></span>
            <div class="min-w-0">
              <h2 id="egress-runtime-title" class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ runtimeStatusLabel }}
              </h2>
              <p class="truncate text-xs text-gray-500 dark:text-gray-400">
                {{ runtime?.platform || '-' }} · {{ runtime?.freebind ? t('admin.egress.runtime.freebind') : t('admin.egress.runtime.assignedOnly') }}
              </p>
            </div>
          </div>
          <dl class="grid grid-cols-2 gap-x-6 gap-y-2 text-xs sm:grid-cols-4">
            <div>
              <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.egress.runtime.failClosed') }}</dt>
              <dd class="mt-0.5 font-medium text-gray-900 dark:text-white">{{ yesNo(runtime?.fail_closed) }}</dd>
            </div>
            <div>
              <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.egress.runtime.secret') }}</dt>
              <dd class="mt-0.5 font-medium text-gray-900 dark:text-white">{{ configuredLabel(runtime?.secret_configured) }}</dd>
            </div>
            <div>
              <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.egress.runtime.probe') }}</dt>
              <dd class="mt-0.5 font-medium text-gray-900 dark:text-white">{{ configuredLabel(runtime?.probe_configured) }}</dd>
            </div>
            <div>
              <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.egress.runtime.reconcile') }}</dt>
              <dd class="mt-0.5 font-medium text-gray-900 dark:text-white">{{ runtime?.reconcile_interval_seconds || '-' }}s</dd>
            </div>
          </dl>
        </div>
      </section>

      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="inline-flex rounded-md border border-gray-200 bg-gray-50 p-1 dark:border-dark-700 dark:bg-dark-900">
          <button
            v-for="tab in tabs"
            :key="tab.value"
            type="button"
            class="rounded px-3 py-1.5 text-sm font-medium transition"
            :class="activeTab === tab.value ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'"
            @click="activeTab = tab.value"
          >
            {{ tab.label }}
          </button>
        </div>

        <div class="flex flex-wrap items-center justify-end gap-2">
          <button type="button" class="btn btn-secondary" :title="t('common.refresh')" :disabled="loading" @click="refreshAll">
            <Icon name="refresh" size="md" :class="{ 'animate-spin': loading }" />
          </button>
          <button
            v-if="activeTab === 'accounts'"
            type="button"
            class="btn btn-secondary"
            :disabled="!runtime?.ready || reconciling"
            @click="reconcile"
          >
            <Icon name="sync" size="md" class="mr-2" :class="{ 'animate-spin': reconciling }" />
            {{ t('admin.egress.actions.reconcile') }}
          </button>
          <button
            v-if="activeTab === 'accounts'"
            type="button"
            class="btn btn-primary"
            :disabled="selectedAccountIDs.size === 0"
            @click="openBatchRouteDialog"
          >
            <Icon name="cog" size="md" class="mr-2" />
            {{ t('admin.egress.actions.setSelected', { count: selectedAccountIDs.size }) }}
          </button>
          <button v-else-if="activeTab === 'pools'" type="button" class="btn btn-secondary" :disabled="detectingPrefix" @click="openCreatePoolWithDetectedPrefix">
            <Icon name="search" size="sm" class="mr-2" :class="{ 'animate-pulse': detectingPrefix }" />
            {{ t('admin.egress.actions.detectPrefix') }}
          </button>
          <button v-if="activeTab === 'pools'" type="button" class="btn btn-primary" @click="openCreatePoolDialog">
            <Icon name="plus" size="md" class="mr-2" />
            {{ t('admin.egress.actions.createPool') }}
          </button>
        </div>
      </div>

      <div v-if="errorMessage" class="border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300">
        {{ errorMessage }}
      </div>

      <section v-if="activeTab === 'pools'" aria-labelledby="egress-pools-title">
        <div class="mb-3 flex items-center justify-between">
          <h2 id="egress-pools-title" class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.egress.pools.title') }}</h2>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ pools.length }}</span>
        </div>
        <div class="overflow-x-auto border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
          <table class="w-full min-w-[1240px] table-fixed text-left text-sm">
            <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-900/60 dark:text-gray-400">
              <tr>
                <th class="w-52 px-4 py-3 font-medium">{{ t('admin.egress.pools.columns.name') }}</th>
                <th class="w-64 px-4 py-3 font-medium">{{ t('admin.egress.pools.columns.cidr') }}</th>
                <th class="w-40 px-4 py-3 font-medium">{{ t('admin.egress.pools.columns.node') }}</th>
                <th class="w-28 px-4 py-3 font-medium">{{ t('admin.egress.pools.columns.allocated') }}</th>
                <th class="w-48 px-4 py-3 font-medium">{{ t('admin.egress.pools.columns.capacity') }}</th>
                <th class="w-32 px-4 py-3 font-medium">{{ t('admin.egress.pools.columns.health') }}</th>
                <th class="w-28 px-4 py-3 font-medium">{{ t('admin.egress.pools.columns.version') }}</th>
                <th class="w-32 px-4 py-3 font-medium">{{ t('admin.egress.pools.columns.status') }}</th>
                <th class="w-36 px-4 py-3 text-right font-medium">{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="pool in pools" :key="pool.id" class="align-middle">
                <td class="px-4 py-3">
                  <div class="flex min-w-0 items-center gap-2">
                    <span class="truncate font-medium text-gray-900 dark:text-white" :title="pool.name">{{ pool.name }}</span>
                    <span v-if="pool.is_default" class="rounded bg-primary-50 px-1.5 py-0.5 text-[11px] font-medium text-primary-700 dark:bg-primary-950/40 dark:text-primary-300">
                      {{ t('admin.egress.pools.default') }}
                    </span>
                  </div>
                </td>
                <td class="px-4 py-3 font-mono text-xs text-gray-700 dark:text-gray-300">{{ pool.cidr }}</td>
                <td class="px-4 py-3 text-gray-600 dark:text-gray-300">{{ pool.node_id || '-' }}</td>
                <td class="px-4 py-3 tabular-nums text-gray-700 dark:text-gray-300">{{ pool.allocated_count }}</td>
                <td class="truncate px-4 py-3 font-mono text-xs text-gray-700 dark:text-gray-300" :title="pool.capacity">{{ pool.capacity }}</td>
                <td class="px-4 py-3">
                  <span class="rounded px-2 py-1 text-xs font-medium" :class="poolHealthBadgeClass(pool)" :title="pool.probe_error || pool.last_probe_at || ''">
                    {{ poolHealthLabel(pool) }}
                  </span>
                </td>
                <td class="px-4 py-3 tabular-nums text-gray-700 dark:text-gray-300">{{ pool.allocation_version }}</td>
                <td class="px-4 py-3">
                  <span class="rounded px-2 py-1 text-xs font-medium" :class="pool.status === 'active' ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300' : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'">
                    {{ poolStatusLabel(pool.status) }}
                  </span>
                </td>
                <td class="px-4 py-3">
                  <div class="flex items-center justify-end gap-1">
                    <button type="button" class="rounded p-2 text-gray-500 hover:bg-gray-100 hover:text-gray-800 dark:hover:bg-dark-700 dark:hover:text-white" :title="t('common.edit')" @click="openEditPoolDialog(pool)">
                      <Icon name="edit" size="sm" />
                    </button>
                    <button type="button" class="rounded p-2 text-red-500 hover:bg-red-50 dark:hover:bg-red-950/30" :title="t('common.delete')" :disabled="pool.allocated_count > 0" @click="requestDeletePool(pool)">
                      <Icon name="trash" size="sm" />
                    </button>
                  </div>
                </td>
              </tr>
              <tr v-if="!loading && pools.length === 0">
                <td colspan="9" class="px-4 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('admin.egress.pools.empty') }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section v-else-if="activeTab === 'accounts'" aria-labelledby="egress-accounts-title">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
          <h2 id="egress-accounts-title" class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.egress.accounts.title') }}</h2>
          <div class="relative w-full sm:w-72">
            <Icon name="search" size="sm" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
            <input v-model="searchQuery" type="search" class="input pl-9" :placeholder="t('admin.egress.accounts.search')" @input="handleSearch" />
          </div>
        </div>
        <div class="overflow-x-auto border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
          <table class="w-full min-w-[1120px] table-fixed text-left text-sm">
            <thead class="bg-gray-50 text-xs text-gray-500 dark:bg-dark-900/60 dark:text-gray-400">
              <tr>
                <th class="w-12 px-4 py-3">
                  <input type="checkbox" :checked="allVisibleSelected" :aria-label="t('common.selectAll')" @change="toggleVisibleSelection" />
                </th>
                <th class="w-56 px-4 py-3 font-medium">{{ t('admin.egress.accounts.columns.account') }}</th>
                <th class="w-32 px-4 py-3 font-medium">{{ t('admin.egress.accounts.columns.platform') }}</th>
                <th class="w-40 px-4 py-3 font-medium">{{ t('admin.egress.accounts.columns.mode') }}</th>
                <th class="w-64 px-4 py-3 font-medium">{{ t('admin.egress.accounts.columns.source') }}</th>
                <th class="w-44 px-4 py-3 font-medium">{{ t('admin.egress.accounts.columns.pool') }}</th>
                <th class="w-28 px-4 py-3 font-medium">{{ t('admin.egress.accounts.columns.version') }}</th>
                <th class="w-40 px-4 py-3 text-right font-medium">{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="account in accounts" :key="account.id" class="align-middle">
                <td class="px-4 py-3">
                  <input type="checkbox" :checked="selectedAccountIDs.has(account.id)" :aria-label="account.name" @change="toggleAccountSelection(account.id)" />
                </td>
                <td class="px-4 py-3">
                  <p class="truncate font-medium text-gray-900 dark:text-white" :title="account.name">{{ account.name }}</p>
                  <p class="mt-0.5 text-xs text-gray-400">#{{ account.id }}</p>
                </td>
                <td class="px-4 py-3 text-gray-600 dark:text-gray-300">{{ account.platform }}</td>
                <td class="px-4 py-3">
                  <span class="rounded px-2 py-1 text-xs font-medium" :class="modeBadgeClass(account)">{{ effectiveModeLabel(account) }}</span>
                </td>
                <td class="px-4 py-3">
                  <button
                    v-if="account.egress_binding?.source_ipv6"
                    type="button"
                    class="max-w-full truncate font-mono text-xs text-gray-700 hover:text-primary-600 dark:text-gray-300 dark:hover:text-primary-400"
                    :title="t('common.copy')"
                    @click="copySource(account.egress_binding.source_ipv6)"
                  >
                    {{ account.egress_binding.source_ipv6 }}
                  </button>
                  <span v-else class="text-gray-400">-</span>
                </td>
                <td class="px-4 py-3 text-gray-600 dark:text-gray-300">{{ account.egress_binding?.pool_name || '-' }}</td>
                <td class="px-4 py-3 tabular-nums text-gray-600 dark:text-gray-300">{{ account.egress_binding?.version || '-' }}</td>
                <td class="px-4 py-3">
                  <div class="flex items-center justify-end gap-1">
                    <button type="button" class="rounded p-2 text-gray-500 hover:bg-gray-100 hover:text-gray-800 dark:hover:bg-dark-700 dark:hover:text-white" :title="t('admin.egress.actions.setRoute')" @click="openAccountRouteDialog(account)">
                      <Icon name="cog" size="sm" />
                    </button>
                    <button type="button" class="rounded p-2 text-gray-500 hover:bg-gray-100 hover:text-gray-800 disabled:opacity-40 dark:hover:bg-dark-700 dark:hover:text-white" :title="t('admin.egress.actions.probe')" :disabled="!canProbe(account) || accountBusy.has(account.id)" @click="probe(account)">
                      <Icon name="beaker" size="sm" :class="{ 'animate-pulse': accountBusy.has(account.id) }" />
                    </button>
                    <button type="button" class="rounded p-2 text-gray-500 hover:bg-gray-100 hover:text-gray-800 disabled:opacity-40 dark:hover:bg-dark-700 dark:hover:text-white" :title="t('admin.egress.actions.rotate')" :disabled="!account.egress_binding || accountBusy.has(account.id)" @click="rotate(account)">
                      <Icon name="refresh" size="sm" />
                    </button>
                  </div>
                </td>
              </tr>
              <tr v-if="!loading && accounts.length === 0">
                <td colspan="8" class="px-4 py-12 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('admin.egress.accounts.empty') }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="mt-3 flex justify-end">
          <Pagination
            v-if="accountPagination.total > 0"
            :page="accountPagination.page"
            :total="accountPagination.total"
            :page-size="accountPagination.page_size"
            @update:page="changeAccountPage"
            @update:pageSize="changeAccountPageSize"
          />
        </div>
      </section>

      <section v-else aria-labelledby="egress-he-title" class="space-y-4">
        <div class="flex flex-wrap items-center justify-between gap-3 border-y border-gray-200 bg-white px-4 py-3 dark:border-dark-700 dark:bg-dark-800 sm:px-5">
          <div class="flex min-w-0 items-center gap-3">
            <span class="h-2.5 w-2.5 flex-shrink-0 rounded-full" :class="heAgentBadgeClass"></span>
            <div class="min-w-0">
              <h2 id="egress-he-title" class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('admin.egress.he.title') }}
              </h2>
              <p class="truncate text-xs text-gray-500 dark:text-gray-400">{{ heAgentStatusLabel }}</p>
            </div>
          </div>
          <div class="text-right text-xs text-gray-500 dark:text-gray-400">
            <p>{{ heControl?.agent.action ? egressHEActionLabel(t, heControl.agent.action) : '-' }}</p>
            <p v-if="heControl?.agent.updated_at" class="mt-0.5">{{ formatHEUpdatedAt(heControl.agent.updated_at) }}</p>
          </div>
        </div>

        <div v-if="heControl && !heControl.available" class="border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-300">
          {{ t('admin.egress.he.controlUnavailable') }}
        </div>
        <div v-else-if="heControl?.agent.message" class="border px-4 py-3 text-sm" :class="heControl.agent.state === 'failed' ? 'border-red-200 bg-red-50 text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300' : 'border-gray-200 bg-gray-50 text-gray-700 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-300'">
          {{ heControl.agent.message }}
        </div>

        <form class="space-y-5" @submit.prevent="saveHEConfig">
          <div class="border-y border-gray-200 bg-white px-4 py-4 dark:border-dark-700 dark:bg-dark-800 sm:px-5">
            <div class="mb-4 flex items-center justify-between gap-4">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.egress.he.connection') }}</h3>
              <Toggle v-model="heForm.enabled" :disabled="!heControl?.available || heBusy" />
            </div>
            <div class="grid gap-4 md:grid-cols-2">
              <div>
                <label class="form-label" for="he-server-ipv4">{{ t('admin.egress.he.fields.serverIPv4') }}</label>
                <input id="he-server-ipv4" v-model="heForm.server_ipv4" class="input font-mono" placeholder="216.66.80.30" :disabled="!heControl?.available" />
              </div>
              <div>
                <label class="form-label" for="he-local-ipv4">{{ t('admin.egress.he.fields.localIPv4') }}</label>
                <input id="he-local-ipv4" v-model="heForm.local_ipv4" class="input font-mono" :placeholder="t('admin.egress.he.fields.autoDetect')" :disabled="!heControl?.available" />
              </div>
              <div>
                <label class="form-label" for="he-client-ipv6">{{ t('admin.egress.he.fields.clientIPv6') }}</label>
                <input id="he-client-ipv6" v-model="heForm.client_ipv6" class="input font-mono" placeholder="2001:470:1::2/64" :disabled="!heControl?.available" />
              </div>
              <div>
                <label class="form-label" for="he-server-ipv6">{{ t('admin.egress.he.fields.serverIPv6') }}</label>
                <input id="he-server-ipv6" v-model="heForm.server_ipv6" class="input font-mono" placeholder="2001:470:1::1" :disabled="!heControl?.available" />
              </div>
              <div class="md:col-span-2">
                <label class="form-label" for="he-pool-cidr">{{ t('admin.egress.he.fields.routedPool') }}</label>
                <div class="flex gap-2">
                  <input id="he-pool-cidr" v-model="heForm.pool_cidr" class="input min-w-0 flex-1 font-mono" placeholder="2001:470:2::/64" :disabled="!heControl?.available" />
                  <button type="button" class="btn btn-secondary shrink-0" :disabled="!heControl?.available || detectingPrefix || heBusy" @click="detectHEPoolPrefix">
                    <Icon name="search" size="sm" :class="{ 'animate-pulse': detectingPrefix }" />
                    <span class="sr-only">{{ t('admin.egress.he.fields.detectPool') }}</span>
                  </button>
                </div>
              </div>
            </div>
          </div>

          <div class="border-y border-gray-200 bg-white px-4 py-4 dark:border-dark-700 dark:bg-dark-800 sm:px-5">
            <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.egress.he.network') }}</h3>
            <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
              <div>
                <label class="form-label" for="he-mtu">{{ t('admin.egress.he.fields.mtu') }}</label>
                <input id="he-mtu" v-model.number="heForm.mtu" type="number" min="1280" max="1480" class="input" :disabled="!heControl?.available" />
              </div>
              <div>
                <label class="form-label" for="he-route-metric">{{ t('admin.egress.he.fields.routeMetric') }}</label>
                <input id="he-route-metric" v-model.number="heForm.route_metric" type="number" min="1" max="65535" class="input" :disabled="!heControl?.available" />
              </div>
              <div>
                <label class="form-label" for="he-probe-ipv6">{{ t('admin.egress.he.fields.probeIPv6') }}</label>
                <input id="he-probe-ipv6" v-model="heForm.probe_ipv6" class="input font-mono" :disabled="!heControl?.available" />
              </div>
              <div>
                <label class="form-label" for="he-probe-timeout">{{ t('admin.egress.he.fields.probeTimeout') }}</label>
                <input id="he-probe-timeout" v-model.number="heForm.probe_timeout_seconds" type="number" min="1" max="30" class="input" :disabled="!heControl?.available" />
              </div>
            </div>
            <label class="mt-4 flex items-center justify-between gap-4 border-t border-gray-100 pt-4 text-sm dark:border-dark-700">
              <span class="text-gray-700 dark:text-gray-300">{{ t('admin.egress.he.fields.allowContainerNAT') }}</span>
              <Toggle v-model="heForm.allow_private_ipv4" :disabled="!heControl?.available" />
            </label>
          </div>

          <div class="border-y border-gray-200 bg-white px-4 py-4 dark:border-dark-700 dark:bg-dark-800 sm:px-5">
            <div class="mb-4 flex items-center justify-between gap-4">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.egress.he.dynamicUpdate') }}</h3>
              <Toggle v-model="heForm.update_enabled" :disabled="!heControl?.available" />
            </div>
            <div v-if="heForm.update_enabled" class="grid gap-4 md:grid-cols-3">
              <div>
                <label class="form-label" for="he-tunnel-id">{{ t('admin.egress.he.fields.tunnelID') }}</label>
                <input id="he-tunnel-id" v-model="heForm.tunnel_id" inputmode="numeric" class="input" :disabled="!heControl?.available" />
              </div>
              <div>
                <label class="form-label" for="he-username">{{ t('admin.egress.he.fields.username') }}</label>
                <input id="he-username" v-model="heForm.username" autocomplete="username" class="input" :disabled="!heControl?.available" />
              </div>
              <div>
                <label class="form-label" for="he-update-key">{{ t('admin.egress.he.fields.updateKey') }}</label>
                <input id="he-update-key" v-model="heUpdateKey" type="password" autocomplete="new-password" class="input" :placeholder="heControl?.config.update_key_configured ? t('admin.egress.he.fields.keepExistingKey') : ''" :disabled="!heControl?.available || clearHEUpdateKey" />
              </div>
            </div>
            <label v-if="heForm.update_enabled && heControl?.config.update_key_configured" class="mt-4 flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300">
              <input v-model="clearHEUpdateKey" type="checkbox" :disabled="!heControl?.available" />
              {{ t('admin.egress.he.fields.clearUpdateKey') }}
            </label>
          </div>

          <div class="flex flex-wrap items-center justify-between gap-3 px-1">
            <button type="button" class="btn btn-secondary" :disabled="!heControl?.available || !heForm.pool_cidr || hePoolExists || heBusy" @click="createHEPool">
              <Icon name="plus" size="sm" class="mr-2" />
              {{ hePoolExists ? t('admin.egress.he.poolExists') : t('admin.egress.he.createPool') }}
            </button>
            <div class="flex flex-wrap items-center justify-end gap-2">
              <button type="button" class="btn btn-secondary" :disabled="!heControl?.available || heBusy" @click="runHEAction('check')">
                <Icon name="beaker" size="sm" class="mr-2" />
                {{ t('admin.egress.he.actions.check') }}
              </button>
              <button type="button" class="btn btn-secondary text-red-600" :disabled="!heControl?.available || heBusy" @click="runHEAction('remove')">
                <Icon name="trash" size="sm" class="mr-2" />
                {{ t('admin.egress.he.actions.remove') }}
              </button>
              <button type="submit" class="btn btn-secondary" :disabled="!heControl?.available || heBusy">
                <Icon v-if="heSaving" name="refresh" size="sm" class="mr-2 animate-spin" />
                {{ t('common.save') }}
              </button>
              <button type="button" class="btn btn-primary" :disabled="!heControl?.available || !heForm.enabled || heBusy" @click="runHEAction('apply')">
                <Icon name="play" size="sm" class="mr-2" />
                {{ t('admin.egress.he.actions.apply') }}
              </button>
            </div>
          </div>
        </form>
      </section>
    </div>

    <BaseDialog :show="showPoolDialog" :title="editingPool ? t('admin.egress.pools.editTitle') : t('admin.egress.pools.createTitle')" @close="closePoolDialog">
      <form class="space-y-4" @submit.prevent="savePool">
        <div>
          <label class="form-label" for="egress-pool-name">{{ t('admin.egress.pools.fields.name') }}</label>
          <input id="egress-pool-name" v-model="poolForm.name" class="input" maxlength="100" required />
        </div>
        <div>
          <label class="form-label" for="egress-pool-cidr">{{ t('admin.egress.pools.fields.cidr') }}</label>
          <input id="egress-pool-cidr" v-model="poolForm.cidr" class="input font-mono" placeholder="2001:db8:100::/64" :disabled="Boolean(editingPool)" required />
        </div>
        <div>
          <label class="form-label" for="egress-pool-node">{{ t('admin.egress.pools.fields.node') }}</label>
          <input id="egress-pool-node" v-model="poolForm.node_id" class="input" maxlength="128" />
        </div>
        <div v-if="editingPool">
          <label class="form-label">{{ t('admin.egress.pools.fields.status') }}</label>
          <Select v-model="poolForm.status" :options="poolStatusOptions" />
        </div>
        <label v-if="editingPool" class="flex items-center justify-between gap-4 border-t border-gray-100 pt-4 text-sm dark:border-dark-700">
          <span class="text-gray-700 dark:text-gray-300">{{ t('admin.egress.pools.fields.default') }}</span>
          <Toggle
            v-model="poolForm.is_default"
            :disabled="!runtime?.ready || (!editingPool?.is_default && editingPool?.route_healthy !== true)"
          />
        </label>
      </form>
      <template #footer>
        <button type="button" class="btn btn-secondary" @click="closePoolDialog">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-primary" :disabled="savingPool" @click="savePool">
          <Icon v-if="savingPool" name="refresh" size="sm" class="mr-2 animate-spin" />
          {{ t('common.save') }}
        </button>
      </template>
    </BaseDialog>

    <BaseDialog :show="showRouteDialog" :title="t('admin.egress.routeDialog.title')" @close="closeRouteDialog">
      <div class="space-y-4">
        <p class="text-sm text-gray-600 dark:text-gray-400">{{ routeDialogSummary }}</p>
        <div>
          <label class="form-label">{{ t('admin.egress.routeDialog.mode') }}</label>
          <Select v-model="routeForm.mode" :options="routeModeOptions" />
        </div>
        <div v-if="routeForm.mode === 'ipv6_pool'">
          <label class="form-label">{{ t('admin.egress.routeDialog.pool') }}</label>
          <Select v-model="routeForm.pool_id" :options="poolOptions" :placeholder="t('admin.egress.routeDialog.defaultPool')" clearable />
        </div>
        <div v-if="routeTargets.some((account) => account.proxy_id)" class="border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-300">
          {{ t('admin.egress.routeDialog.proxyOverride') }}
        </div>
      </div>
      <template #footer>
        <button type="button" class="btn btn-secondary" @click="closeRouteDialog">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-primary" :disabled="savingRoute || (routeForm.mode === 'ipv6_pool' && !runtime?.ready)" @click="saveRoutes">
          <Icon v-if="savingRoute" name="refresh" size="sm" class="mr-2 animate-spin" />
          {{ t('common.apply') }}
        </button>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="Boolean(deletingPool)"
      :title="t('admin.egress.pools.deleteTitle')"
      :message="t('admin.egress.pools.deleteConfirm', { name: deletingPool?.name || '' })"
      danger
      @confirm="deleteSelectedPool"
      @cancel="deletingPool = null"
    />
    <TotpStepUpDialog :controller="sensitiveStepUp" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import Select from '@/common/widgets/forms/Select.vue'
import Toggle from '@/common/widgets/forms/Toggle.vue'
import Pagination from '@/common/widgets/data/Pagination.vue'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import ConfirmDialog from '@/common/widgets/feedback/ConfirmDialog.vue'
import { TotpStepUpDialog } from '@/features/auth'
import { useStepUp, isStepUpCancelled } from '@/common/composables/useStepUp'
import { useClipboard } from '@/common/composables/useClipboard'
import { useAppStore } from '@/core/stores/appStore'
import { extractApiErrorMessage } from '@/core/utils/apiError'
import accountsAPI from '@/features/admin-accounts/data/datasources/adminAccountsDatasource'
import type { Account } from '@/types'
import egressAPI, {
  type EgressMode,
  type HETunnelControlSnapshot,
  type HETunnelAgentStatus,
  type IPv6EgressPool,
  type IPv6EgressPoolStatus,
  type IPv6EgressRuntime,
} from '@/features/admin-egress/data/datasources/adminEgressDatasource'
import {
  egressHEActionLabel,
  egressHEErrorMessage,
  egressHEStateLabel,
  egressHESuccessMessage,
  egressModeLabel,
  egressPoolStatusLabel,
} from '@/features/admin-egress/presentation/egressLocale'

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()
const sensitiveStepUp = useStepUp()

const activeTab = ref<'pools' | 'accounts' | 'he'>('pools')
const runtime = ref<IPv6EgressRuntime | null>(null)
const pools = ref<IPv6EgressPool[]>([])
const accounts = ref<Account[]>([])
const loading = ref(false)
const errorMessage = ref('')
const detectingPrefix = ref(false)
const reconciling = ref(false)
const searchQuery = ref('')
const selectedAccountIDs = ref(new Set<number>())
const accountBusy = ref(new Set<number>())
const accountPagination = reactive({ page: 1, page_size: 25, total: 0, pages: 0 })
let searchTimer: ReturnType<typeof setTimeout> | null = null
let accountAbort: AbortController | null = null
let heStatusTimer: ReturnType<typeof setInterval> | null = null

const showPoolDialog = ref(false)
const editingPool = ref<IPv6EgressPool | null>(null)
const savingPool = ref(false)
const deletingPool = ref<IPv6EgressPool | null>(null)
const poolForm = reactive({
  name: '',
  cidr: '',
  node_id: '',
  status: 'active' as IPv6EgressPoolStatus,
  is_default: false,
})

const showRouteDialog = ref(false)
const routeTargets = ref<Account[]>([])
const savingRoute = ref(false)
const routeForm = reactive<{ mode: Exclude<EgressMode, 'external_proxy'>; pool_id: number | null }>({
  mode: 'inherit',
  pool_id: null,
})

const heControl = ref<HETunnelControlSnapshot | null>(null)
const heSaving = ref(false)
const heAction = ref<HETunnelAgentStatus['action'] | ''>('')
const heUpdateKey = ref('')
const clearHEUpdateKey = ref(false)
const heForm = reactive({
  enabled: false,
  server_ipv4: '',
  local_ipv4: '',
  client_ipv6: '',
  server_ipv6: '',
  pool_cidr: '',
  mtu: 1480,
  route_metric: 2048,
  probe_ipv6: '2606:4700:4700::1111',
  probe_timeout_seconds: 5,
  allow_private_ipv4: true,
  update_enabled: false,
  tunnel_id: '',
  username: '',
})

const tabs = computed(() => [
  { value: 'pools' as const, label: t('admin.egress.tabs.pools') },
  { value: 'accounts' as const, label: t('admin.egress.tabs.accounts') },
  { value: 'he' as const, label: t('admin.egress.tabs.he') },
])
const poolStatusOptions = computed(() => [
  { value: 'active', label: t('admin.egress.status.active') },
  { value: 'disabled', label: t('admin.egress.status.disabled') },
])
const routeModeOptions = computed(() => [
  { value: 'inherit', label: t('admin.egress.modes.inherit') },
  { value: 'direct', label: t('admin.egress.modes.direct') },
  { value: 'ipv6_pool', label: t('admin.egress.modes.ipv6_pool') },
])
const poolOptions = computed(() => pools.value
  .filter((pool) => pool.status === 'active')
  .map((pool) => ({ value: pool.id, label: pool.is_default ? `${pool.name} (${t('admin.egress.pools.default')})` : pool.name })))
const allVisibleSelected = computed(() => accounts.value.length > 0 && accounts.value.every((account) => selectedAccountIDs.value.has(account.id)))
const runtimeStatusLabel = computed(() => {
  if (!runtime.value) return t('admin.egress.runtime.loading')
  if (runtime.value.ready) return t('admin.egress.runtime.ready')
  if (runtime.value.enabled) return t('admin.egress.runtime.unavailable')
  return t('admin.egress.runtime.disabled')
})
const routeDialogSummary = computed(() => routeTargets.value.length === 1
  ? t('admin.egress.routeDialog.single', { name: routeTargets.value[0]?.name || '' })
  : t('admin.egress.routeDialog.multiple', { count: routeTargets.value.length }))
const heBusy = computed(() => heSaving.value || Boolean(heAction.value))
const hePoolExists = computed(() => {
  const cidr = heForm.pool_cidr.trim().toLowerCase()
  return cidr !== '' && pools.value.some((pool) => pool.cidr.trim().toLowerCase() === cidr)
})
const heAgentStatusLabel = computed(() => {
  if (!heControl.value?.available) return t('admin.egress.he.states.unavailable')
  if (!heControl.value.agent.online) return t('admin.egress.he.states.offline')
  return egressHEStateLabel(t, heControl.value.agent.state)
})
const heAgentBadgeClass = computed(() => {
  if (!heControl.value?.available || !heControl.value.agent.online) return 'bg-gray-400'
  if (heControl.value.agent.state === 'failed') return 'bg-red-500'
  if (heControl.value.agent.state === 'pending' || heControl.value.agent.state === 'applying') return 'bg-amber-500'
  return 'bg-emerald-500'
})

function yesNo(value?: boolean): string {
  return value ? t('common.yes') : t('common.no')
}

function configuredLabel(value?: boolean): string {
  return value ? t('admin.egress.runtime.configured') : t('admin.egress.runtime.notConfigured')
}

function poolStatusLabel(status: IPv6EgressPoolStatus): string {
  return egressPoolStatusLabel(t, status)
}

function effectiveModeLabel(account: Account): string {
  if (account.proxy_id) return t('admin.egress.modes.proxyOverride')
  return egressModeLabel(t, account.egress_mode || 'inherit')
}

function modeBadgeClass(account: Account): string {
  if (account.proxy_id) return 'bg-blue-50 text-blue-700 dark:bg-blue-950/40 dark:text-blue-300'
  if (account.egress_mode === 'ipv6_pool' || (account.egress_mode !== 'direct' && account.egress_binding)) {
    return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300'
  }
  if (account.egress_mode === 'direct') return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
  return 'bg-amber-50 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300'
}

function applyHESnapshot(snapshot: HETunnelControlSnapshot, updateForm: boolean): void {
  heControl.value = snapshot
  if (!updateForm) return
  Object.assign(heForm, {
    enabled: snapshot.config.enabled,
    server_ipv4: snapshot.config.server_ipv4 || '',
    local_ipv4: snapshot.config.local_ipv4 || '',
    client_ipv6: snapshot.config.client_ipv6 || '',
    server_ipv6: snapshot.config.server_ipv6 || '',
    pool_cidr: snapshot.config.pool_cidr || '',
    mtu: snapshot.config.mtu || 1480,
    route_metric: snapshot.config.route_metric || 2048,
    probe_ipv6: snapshot.config.probe_ipv6 || '',
    probe_timeout_seconds: snapshot.config.probe_timeout_seconds || 5,
    allow_private_ipv4: snapshot.config.allow_private_ipv4,
    update_enabled: snapshot.config.update_enabled,
    tunnel_id: snapshot.config.tunnel_id || '',
    username: snapshot.config.username || '',
  })
  heUpdateKey.value = ''
  clearHEUpdateKey.value = false
}

async function loadHEControl(updateForm = false): Promise<void> {
  const snapshot = await egressAPI.getHETunnel()
  applyHESnapshot(snapshot, updateForm || heControl.value === null)
}

async function loadRuntimeAndPools(): Promise<void> {
  const [runtimeResult, poolResult] = await Promise.all([egressAPI.getRuntime(), egressAPI.listPools()])
  runtime.value = runtimeResult
  pools.value = poolResult
}

async function loadAccounts(): Promise<void> {
  accountAbort?.abort()
  accountAbort = new AbortController()
  const result = await accountsAPI.list(
    accountPagination.page,
    accountPagination.page_size,
    { search: searchQuery.value.trim(), sort_by: 'id', sort_order: 'desc' },
    { signal: accountAbort.signal },
  )
  accounts.value = result.items
  accountPagination.total = result.total
  accountPagination.pages = result.pages
  const visibleIDs = new Set(result.items.map((account) => account.id))
  selectedAccountIDs.value = new Set([...selectedAccountIDs.value].filter((id) => visibleIDs.has(id)))
}

async function refreshAll(): Promise<void> {
  if (loading.value) return
  loading.value = true
  errorMessage.value = ''
  try {
    await loadRuntimeAndPools()
    if (activeTab.value === 'accounts') await loadAccounts()
    if (activeTab.value === 'he') await loadHEControl(true)
  } catch (error) {
    if ((error as { code?: string })?.code === 'ERR_CANCELED') return
    errorMessage.value = extractApiErrorMessage(error, t('admin.egress.errors.load'))
  } finally {
    loading.value = false
  }
}

function handleSearch(): void {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    accountPagination.page = 1
    void refreshAll()
  }, 300)
}

function changeAccountPage(page: number): void {
  accountPagination.page = page
  void refreshAll()
}

function changeAccountPageSize(pageSize: number): void {
  accountPagination.page_size = pageSize
  accountPagination.page = 1
  void refreshAll()
}

function toggleAccountSelection(id: number): void {
  const next = new Set(selectedAccountIDs.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selectedAccountIDs.value = next
}

function toggleVisibleSelection(): void {
  const next = new Set(selectedAccountIDs.value)
  if (allVisibleSelected.value) accounts.value.forEach((account) => next.delete(account.id))
  else accounts.value.forEach((account) => next.add(account.id))
  selectedAccountIDs.value = next
}

function openCreatePoolDialog(): void {
  editingPool.value = null
  Object.assign(poolForm, { name: '', cidr: '', node_id: '', status: 'active', is_default: false })
  showPoolDialog.value = true
}

async function detectSuggestedPrefix(): Promise<string | null> {
  if (detectingPrefix.value) return null
  detectingPrefix.value = true
  try {
    const result = await egressAPI.discoverPrefixes()
    if (!result.suggested_pool_cidr) {
      appStore.showError(t('admin.egress.errors.noDetectedPrefix'))
      return null
    }
    return result.suggested_pool_cidr
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.egress.errors.detectPrefix')))
    return null
  } finally {
    detectingPrefix.value = false
  }
}

async function openCreatePoolWithDetectedPrefix(): Promise<void> {
  const prefix = await detectSuggestedPrefix()
  if (!prefix) return
  openCreatePoolDialog()
  poolForm.cidr = prefix
  poolForm.name = t('admin.egress.pools.fields.detectedName', { prefix })
}

async function detectHEPoolPrefix(): Promise<void> {
  const prefix = await detectSuggestedPrefix()
  if (prefix) heForm.pool_cidr = prefix
}

function poolHealthLabel(pool: IPv6EgressPool): string {
  if (pool.route_healthy === true) return t('admin.egress.health.healthy')
  if (pool.route_healthy === false) return t('admin.egress.health.unhealthy')
  return t('admin.egress.health.unknown')
}

function poolHealthBadgeClass(pool: IPv6EgressPool): string {
  if (pool.route_healthy === true) return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300'
  if (pool.route_healthy === false) return 'bg-red-50 text-red-700 dark:bg-red-950/40 dark:text-red-300'
  return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
}

function openEditPoolDialog(pool: IPv6EgressPool): void {
  editingPool.value = pool
  Object.assign(poolForm, {
    name: pool.name,
    cidr: pool.cidr,
    node_id: pool.node_id || '',
    status: pool.status,
    is_default: pool.is_default,
  })
  showPoolDialog.value = true
}

function closePoolDialog(): void {
  if (savingPool.value) return
  showPoolDialog.value = false
  editingPool.value = null
}

async function savePool(): Promise<void> {
  if (savingPool.value) return
  if (!poolForm.name.trim() || !poolForm.cidr.trim()) {
    appStore.showError(t('admin.egress.errors.poolFields'))
    return
  }
  savingPool.value = true
  try {
    if (editingPool.value) {
      const update: Parameters<typeof egressAPI.updatePool>[1] = {
        name: poolForm.name.trim(),
        node_id: poolForm.node_id.trim() || null,
        status: poolForm.status,
      }
      if (poolForm.is_default !== editingPool.value.is_default) update.is_default = poolForm.is_default
      await egressAPI.updatePool(editingPool.value.id, update)
    } else {
      await egressAPI.createPool({
        name: poolForm.name.trim(),
        cidr: poolForm.cidr.trim(),
        node_id: poolForm.node_id.trim() || null,
        is_default: false,
      })
    }
    appStore.showSuccess(t(editingPool.value ? 'admin.egress.success.poolUpdated' : 'admin.egress.success.poolCreated'))
    showPoolDialog.value = false
    editingPool.value = null
    await refreshAll()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.egress.errors.savePool')))
  } finally {
    savingPool.value = false
  }
}

function requestDeletePool(pool: IPv6EgressPool): void {
  if (pool.allocated_count > 0) return
  deletingPool.value = pool
}

async function deleteSelectedPool(): Promise<void> {
  const pool = deletingPool.value
  if (!pool) return
  try {
    await sensitiveStepUp.run(() => egressAPI.deletePool(pool.id))
    deletingPool.value = null
    appStore.showSuccess(t('admin.egress.success.poolDeleted'))
    await refreshAll()
  } catch (error) {
    if (!isStepUpCancelled(error)) appStore.showError(extractApiErrorMessage(error, t('admin.egress.errors.deletePool')))
  }
}

function openAccountRouteDialog(account: Account): void {
  routeTargets.value = [account]
  routeForm.mode = account.egress_mode === 'direct' || account.egress_mode === 'ipv6_pool' ? account.egress_mode : 'inherit'
  routeForm.pool_id = account.egress_binding?.pool_id || null
  showRouteDialog.value = true
}

function openBatchRouteDialog(): void {
  routeTargets.value = accounts.value.filter((account) => selectedAccountIDs.value.has(account.id))
  if (routeTargets.value.length === 0) return
  routeForm.mode = 'inherit'
  routeForm.pool_id = null
  showRouteDialog.value = true
}

function closeRouteDialog(): void {
  if (savingRoute.value) return
  showRouteDialog.value = false
  routeTargets.value = []
}

async function saveRoutes(): Promise<void> {
  if (savingRoute.value || routeTargets.value.length === 0) return
  savingRoute.value = true
  let completed = 0
  try {
    for (const account of routeTargets.value) {
      await egressAPI.setAccountRoute(account.id, routeForm.mode, routeForm.pool_id || undefined)
      completed += 1
    }
    appStore.showSuccess(t('admin.egress.success.routesUpdated', { count: completed }))
    selectedAccountIDs.value = new Set()
    showRouteDialog.value = false
    routeTargets.value = []
    await refreshAll()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.egress.errors.routesPartial', { completed })))
  } finally {
    savingRoute.value = false
  }
}

function setAccountBusy(id: number, busy: boolean): void {
  const next = new Set(accountBusy.value)
  if (busy) next.add(id)
  else next.delete(id)
  accountBusy.value = next
}

function canProbe(account: Account): boolean {
  return Boolean(runtime.value?.ready && runtime.value.probe_configured && account.egress_binding && !account.proxy_id)
}

async function probe(account: Account): Promise<void> {
  if (!canProbe(account)) return
  setAccountBusy(account.id, true)
  try {
    const result = await egressAPI.probeAccount(account.id)
    appStore.showSuccess(t('admin.egress.success.probe', { ip: result.observed_ip, latency: result.latency_ms }))
    await loadRuntimeAndPools()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.egress.errors.probe')))
  } finally {
    setAccountBusy(account.id, false)
  }
}

async function rotate(account: Account): Promise<void> {
  if (!account.egress_binding) return
  setAccountBusy(account.id, true)
  try {
    await sensitiveStepUp.run(() => egressAPI.rotateBinding(account.id))
    appStore.showSuccess(t('admin.egress.success.rotated'))
    await refreshAll()
  } catch (error) {
    if (!isStepUpCancelled(error)) appStore.showError(extractApiErrorMessage(error, t('admin.egress.errors.rotate')))
  } finally {
    setAccountBusy(account.id, false)
  }
}

async function reconcile(): Promise<void> {
  if (reconciling.value) return
  reconciling.value = true
  try {
    const result = await egressAPI.reconcileDefault()
    appStore.showSuccess(t('admin.egress.success.reconciled', { count: result.allocated }))
    await refreshAll()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.egress.errors.reconcile')))
  } finally {
    reconciling.value = false
  }
}

async function saveHEConfig(): Promise<void> {
  if (heBusy.value || !heControl.value?.available) return
  heSaving.value = true
  try {
    const snapshot = await sensitiveStepUp.run(() => egressAPI.saveHETunnel(heConfigPayload()))
    applyHESnapshot(snapshot, true)
    appStore.showSuccess(t('admin.egress.he.success.saved'))
  } catch (error) {
    if (!isStepUpCancelled(error)) appStore.showError(extractApiErrorMessage(error, t('admin.egress.he.errors.save')))
  } finally {
    heSaving.value = false
  }
}

async function runHEAction(action: 'apply' | 'check' | 'remove'): Promise<void> {
  if (heBusy.value || !heControl.value?.available) return
  heAction.value = action
  try {
    const snapshot = await sensitiveStepUp.run(async () => {
      if (action === 'apply') {
        const saved = await egressAPI.saveHETunnel(heConfigPayload())
        applyHESnapshot(saved, true)
      }
      return egressAPI.runHETunnelAction(action)
    })
    applyHESnapshot(snapshot, false)
    appStore.showSuccess(egressHESuccessMessage(t, action))
  } catch (error) {
    if (!isStepUpCancelled(error)) appStore.showError(extractApiErrorMessage(error, egressHEErrorMessage(t, action)))
  } finally {
    heAction.value = ''
  }
}

function heConfigPayload(): Parameters<typeof egressAPI.saveHETunnel>[0] {
  return {
    ...heForm,
    update_key: heUpdateKey.value.trim() || undefined,
    clear_update_key: clearHEUpdateKey.value,
  }
}

async function createHEPool(): Promise<void> {
  if (heBusy.value || hePoolExists.value || !heForm.pool_cidr.trim()) return
  heSaving.value = true
  try {
    await egressAPI.createPool({
      name: t('admin.egress.he.poolName'),
      cidr: heForm.pool_cidr.trim(),
      node_id: null,
      is_default: false,
    })
    appStore.showSuccess(t('admin.egress.success.poolCreated'))
    await loadRuntimeAndPools()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.egress.errors.savePool')))
  } finally {
    heSaving.value = false
  }
}

function formatHEUpdatedAt(value: string): string {
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString()
}

async function copySource(source: string): Promise<void> {
  await copyToClipboard(source)
}

watch(activeTab, (tab) => {
  if (tab === 'accounts' && accounts.value.length === 0) void refreshAll()
  if (tab === 'he' && heControl.value === null) void refreshAll()
})

onMounted(() => {
  void refreshAll()
  heStatusTimer = setInterval(() => {
    if (activeTab.value !== 'he' || heControl.value === null) return
    void loadHEControl(false).catch(() => undefined)
  }, 3000)
})
onBeforeUnmount(() => {
  accountAbort?.abort()
  if (searchTimer) clearTimeout(searchTimer)
  if (heStatusTimer) clearInterval(heStatusTimer)
})
</script>
