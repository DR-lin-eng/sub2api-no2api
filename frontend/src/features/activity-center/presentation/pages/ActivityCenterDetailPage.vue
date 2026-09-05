<template>
  <AppLayout>
    <div class="mx-auto max-w-7xl space-y-4 px-2 sm:px-4">
      <RouterLink
        to="/activity-center"
        class="inline-flex items-center gap-2 rounded-lg px-2 py-1 text-sm font-medium text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
      >
        <Icon name="arrowLeft" size="sm" />
        <span>{{ t('activityCenter.backToList') }}</span>
      </RouterLink>

      <div v-if="loading" class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
        <div class="h-32 animate-pulse rounded-2xl bg-gray-100 dark:bg-dark-800"></div>
        <div class="h-32 animate-pulse rounded-2xl bg-gray-100 dark:bg-dark-800"></div>
      </div>

      <section v-else-if="campaign" :class="['grid gap-4', campaign.type === 'checkin' ? 'lg:grid-cols-[minmax(0,1fr)_280px]' : 'lg:grid-cols-[minmax(0,1fr)_340px]']">
        <div class="space-y-4">
          <div v-if="campaign.type === 'lottery'" class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900/90">
            <div class="flex items-center justify-between gap-3">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('activityCenter.lottery.title') }}</h2>
            </div>
            <div v-if="lotteryPools.length > 0" class="mt-4 space-y-4">
              <div v-for="pool in lotteryPools" :key="pool.id" class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
                <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div>
                    <h3 class="font-semibold text-gray-900 dark:text-white">{{ activityText(pool.name, t) || t('activityCenter.lottery.pool') }}</h3>
                    <p v-if="pool.description" class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ pool.description }}</p>
                  </div>
                  <div class="flex flex-wrap gap-2 text-xs text-gray-500 dark:text-dark-400">
                    <span class="rounded-full bg-gray-100 px-2.5 py-1 dark:bg-dark-800">{{ pool.daily_limit > 0 ? t('activityCenter.lottery.dailyLimit', { count: pool.daily_limit }) : t('activityCenter.lottery.unlimited') }}</span>
                  </div>
                </div>

                <template v-if="pool.prizes.length > 0">
                  <div class="mt-4 flex justify-center">
                    <div class="relative mx-auto flex aspect-square w-full max-w-[300px] items-center justify-center">
                      <div class="absolute inset-0 rounded-full bg-gray-200 shadow-[0_8px_18px_rgba(15,23,42,0.10)] dark:bg-dark-700"></div>
                      <div class="absolute inset-[4px] rounded-full bg-white dark:bg-dark-900"></div>
                      <div class="pointer-events-none absolute -top-1 z-20 flex flex-col items-center">
                        <div class="h-2.5 w-2.5 rounded-full bg-primary-500 shadow-[0_1px_0_rgba(255,255,255,0.35)] dark:bg-primary-400"></div>
                        <div class="h-0 w-0 border-l-[9px] border-r-[9px] border-t-[14px] border-l-transparent border-r-transparent border-t-primary-500 drop-shadow-[0_1px_1px_rgba(15,23,42,0.18)] dark:border-t-primary-400"></div>
                      </div>
                      <div class="relative h-full w-full p-3">
                        <svg
                          viewBox="0 0 120 120"
                          class="h-full w-full"
                          :style="wheelStyle(pool)"
                        >
                          <circle cx="60" cy="60" r="58" fill="#ffffff" class="dark:fill-dark-900" />
                          <circle cx="60" cy="60" r="54" fill="none" stroke="#dbe2ea" stroke-width="1.2" class="dark:stroke-dark-600" />
                          <path
                            v-for="slice in prizeWeightSlices(pool)"
                            :key="slice.id"
                            :d="slice.path"
                            :fill="slice.color"
                            stroke="#dbe2ea"
                            stroke-width="0.9"
                          />
                          <text
                            v-for="slice in prizeWeightSlices(pool)"
                            :key="`${slice.id}-label`"
                            :x="slice.labelX"
                            :y="slice.labelY"
                            text-anchor="middle"
                            dominant-baseline="middle"
                            class="select-none"
                            fill="rgba(15, 23, 42, 0.82)"
                            stroke="rgba(255, 255, 255, 0.85)"
                            stroke-width="0.45"
                            paint-order="stroke"
                            :transform="`rotate(${slice.labelRotation} ${slice.labelX} ${slice.labelY})`"
                            style="font-size: 4.6px; font-weight: 700;"
                          >
                            {{ slice.label }}
                          </text>
                          <circle cx="60" cy="60" r="43" fill="none" stroke="#edf2f7" stroke-width="0.8" class="dark:stroke-dark-700" />
                          <circle cx="60" cy="60" r="18" fill="#ffffff" stroke="#dbe2ea" stroke-width="1" class="dark:fill-dark-900 dark:stroke-dark-600" />
                          <circle cx="60" cy="60" r="12" fill="currentColor" class="text-primary-500 dark:text-primary-400" />
                          <circle cx="60" cy="60" r="6.5" fill="#ffffff" class="dark:fill-dark-900" />
                        </svg>
                        <button
                          type="button"
                          class="absolute left-1/2 top-1/2 z-10 flex h-16 w-16 -translate-x-1/2 -translate-y-1/2 flex-col items-center justify-center rounded-full border-[3px] border-white bg-primary-500 text-white shadow-[0_10px_20px_rgba(15,23,42,0.14)] transition-transform hover:scale-[1.02] disabled:cursor-not-allowed disabled:opacity-90 dark:border-dark-900 dark:bg-primary-400"
                          :disabled="wheelState(pool.id).spinning || pool.prizes.length === 0 || pool.can_draw === false"
                          @click="drawPool(pool)"
                        >
                          <Icon name="sparkles" size="xs" />
                          <span class="mt-0.5 text-[10px] font-semibold leading-none">
                            {{ pool.can_draw === false ? t('activityCenter.lottery.notEligible') : (wheelState(pool.id).spinning ? t('activityCenter.lottery.drawing') : t('activityCenter.lottery.drawNow')) }}
                          </span>
                        </button>
                      </div>
                    </div>
                  </div>
                </template>
                <div v-else class="mt-4 rounded-lg border border-dashed border-gray-300 p-5 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-dark-400">
                  {{ t('activityCenter.lottery.noPrizes') }}
                </div>
              </div>
            </div>
            <div v-else class="mt-4 rounded-xl border border-dashed border-gray-300 p-6 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-dark-400">
              {{ t('activityCenter.lottery.noPools') }}
            </div>
          </div>

          <div v-else-if="campaign.type === 'inflate' || campaign.type === 'redeem'" class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900/90">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('activityCenter.inflate.title') }}</h2>
            <div class="mt-4 flex flex-col gap-3 sm:flex-row">
              <input v-model="redeemCodeInput" type="text" class="input" :placeholder="t('activityCenter.inflate.placeholder')" :disabled="redeeming" @keyup.enter="redeemCode" />
              <button type="button" class="btn btn-primary shrink-0" :disabled="redeeming || !redeemCodeInput.trim()" @click="redeemCode">
                {{ redeeming ? t('activityCenter.inflate.submitting') : t('activityCenter.inflate.submit') }}
              </button>
            </div>
            <p class="mt-2 text-xs text-gray-500 dark:text-dark-400">{{ t('activityCenter.inflate.hint') }}</p>
            <div class="mt-4 grid gap-2 text-xs sm:grid-cols-2">
              <div class="rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-800">
                <span class="text-gray-500 dark:text-dark-400">{{ t('activityCenter.inflate.valueRange') }}</span>
                <strong class="ml-2 text-gray-800 dark:text-gray-100">{{ inflateConfig.min_value }} - {{ inflateConfig.max_value }}</strong>
              </div>
              <div class="rounded-lg bg-primary-50 px-3 py-2 dark:bg-primary-900/20">
                <span class="text-primary-700 dark:text-primary-300">{{ t('activityCenter.inflate.rateRange') }}</span>
                <strong class="ml-2 text-primary-800 dark:text-primary-200">{{ inflateConfig.min_inflate_pct }}% - {{ inflateConfig.max_inflate_pct }}%</strong>
              </div>
            </div>
          </div>

          <div v-else-if="campaign.type === 'checkin'" class="flex h-full flex-col overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900 lg:min-h-[340px]">
            <div class="border-b border-gray-200 px-5 py-5 dark:border-dark-700 sm:px-6">
              <div class="flex flex-col gap-5 sm:flex-row sm:items-center sm:justify-between">
                <div class="flex min-w-0 items-center gap-4">
                  <div :class="['flex h-12 w-12 shrink-0 items-center justify-center rounded-lg', checkinStatus.checked_today ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300' : 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300']">
                    <Icon :name="checkinStatus.checked_today ? 'checkCircle' : 'calendar'" size="lg" />
                  </div>
                  <div class="min-w-0">
                    <p :class="['text-xs font-semibold uppercase', checkinStatus.checked_today ? 'text-emerald-700 dark:text-emerald-300' : 'text-amber-700 dark:text-amber-300']">
                      {{ checkinStatus.checked_today ? t('activityCenter.checkin.todayDone') : t('activityCenter.checkin.todayReady') }}
                    </p>
                    <h2 class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ t('activityCenter.checkin.title') }}</h2>
                    <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('activityCenter.checkin.streak', { count: checkinStatus.streak_days }) }}</p>
                  </div>
                </div>
                <button type="button" :class="['btn min-h-11 w-full shrink-0 sm:w-40', checkinStatus.checked_today ? 'bg-emerald-600 text-white hover:bg-emerald-600 dark:bg-emerald-700' : 'btn-primary']" :disabled="checkinLoading" @click="submitCheckin">
                  <Icon :name="checkinStatus.checked_today ? 'checkCircle' : 'gift'" size="sm" class="mr-2" />
                  {{ checkinStatus.checked_today ? t('activityCenter.checkin.checked') : (checkinLoading ? t('activityCenter.checkin.submitting') : t('activityCenter.checkin.submit')) }}
                </button>
              </div>

              <div v-if="currentCheckinReward" class="mt-5 flex items-center gap-3 rounded-lg border border-gray-200 bg-gray-50 px-4 py-3 dark:border-dark-700 dark:bg-dark-800">
                <Icon name="gift" size="md" class="shrink-0 text-primary-600 dark:text-primary-300" />
                <div class="min-w-0">
                  <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('activityCenter.checkin.todayReward') }}</p>
                  <p class="mt-0.5 truncate text-sm font-semibold text-gray-900 dark:text-white">{{ checkinRewardText(currentCheckinReward) }}</p>
                  <p v-if="currentCheckinReward.label" class="mt-0.5 truncate text-xs text-gray-500 dark:text-dark-400">{{ activityText(currentCheckinReward.label, t) }}</p>
                </div>
                <span class="ml-auto shrink-0 text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('activityCenter.checkin.dayLabel', { day: checkinTargetDay }) }}</span>
              </div>
            </div>

            <div class="px-5 py-5 sm:px-6">
              <div class="flex items-center justify-between gap-3">
                <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('activityCenter.checkin.rewardCalendar') }}</h3>
                <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('activityCenter.checkin.day', { day: checkinTargetDay }) }}</span>
              </div>
              <div class="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-4 lg:grid-cols-7">
                <div v-for="reward in checkinRewards" :key="reward.day" :class="checkinRewardClass(reward.day)">
                  <div class="flex items-center justify-between gap-2">
                    <span class="text-xs font-semibold">{{ t('activityCenter.checkin.dayLabel', { day: reward.day }) }}</span>
                    <Icon v-if="isCheckinRewardClaimed(reward.day)" name="checkCircle" size="xs" class="shrink-0 text-emerald-600 dark:text-emerald-300" />
                    <span v-else-if="reward.day === checkinTargetDay" class="h-2 w-2 shrink-0 rounded-full bg-amber-500"></span>
                  </div>
                  <p class="mt-2 line-clamp-2 min-h-8 text-xs leading-4">{{ checkinRewardText(reward) }}</p>
                  <p v-if="reward.label" class="mt-1 line-clamp-1 text-[11px] leading-4 opacity-70">{{ activityText(reward.label, t) }}</p>
                </div>
              </div>
            </div>
          </div>
        </div>

        <aside class="space-y-4 lg:sticky lg:top-4 lg:self-start">
          <div class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900/90">
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ activityText(campaign.title, t) }}</h1>
                <p v-if="campaign.subtitle" class="mt-2 text-sm leading-6 text-gray-500 dark:text-dark-400">
                  {{ activityText(campaign.subtitle, t) }}
                </p>
              </div>
              <span class="inline-flex shrink-0 items-center rounded-full bg-primary-50 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-200">
                {{ typeLabel(campaign.type) }}
              </span>
            </div>
            <div v-if="campaign.starts_at || campaign.ends_at" class="mt-4 text-xs text-gray-500 dark:text-dark-400">
              <span class="rounded-full bg-gray-100 px-2.5 py-1 dark:bg-dark-800">
                {{ activityTimeRange(campaign) }}
              </span>
            </div>
          </div>

          <div v-if="campaign.type === 'checkin'" class="flex flex-col rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900 lg:h-[340px]">
            <div class="flex items-center justify-between gap-3">
              <div class="flex items-center gap-2"><Icon name="chart" size="sm" class="text-primary-600 dark:text-primary-300" /><h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('activityCenter.checkin.leaderboard') }}</h2></div>
              <button type="button" class="rounded-md p-1.5 text-gray-500 hover:bg-gray-100 dark:hover:bg-dark-800" :title="t('activityCenter.refresh')" @click="loadCheckinLeaderboard"><Icon name="refresh" size="xs" :class="leaderboardLoading ? 'animate-spin' : ''" /></button>
            </div>
            <div v-if="leaderboardLoading && leaderboard.length === 0" class="mt-4 space-y-3"><div v-for="index in 3" :key="index" class="h-8 animate-pulse rounded-md bg-gray-100 dark:bg-dark-800"></div></div>
            <div v-else-if="leaderboard.length > 0" class="mt-4 min-h-0 space-y-2 overflow-y-auto pr-1 lg:flex-1">
              <div v-for="item in leaderboard" :key="`${item.rank}-${item.username}`" class="flex items-center gap-3 rounded-md bg-gray-50 px-3 py-2.5 dark:bg-dark-800">
                <span :class="['w-5 shrink-0 text-center text-xs font-bold', item.rank <= 3 ? 'text-amber-600 dark:text-amber-300' : 'text-gray-400 dark:text-dark-500']">{{ item.rank }}</span>
                <span class="min-w-0 flex-1 truncate text-sm font-medium text-gray-800 dark:text-gray-100">{{ item.username }}</span>
                <span class="shrink-0 text-right text-xs text-gray-500 dark:text-dark-400">{{ t('activityCenter.checkin.leaderboardStats', { streak: item.streak_days, count: item.checkin_count }) }}</span>
              </div>
            </div>
            <p v-else class="mt-4 text-sm text-gray-500 dark:text-dark-400">{{ t('activityCenter.checkin.leaderboardEmpty') }}</p>
          </div>

          <div v-if="campaign.content" class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900/90">
            <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('activityCenter.fields.content') }}</p>
            <p class="mt-2 whitespace-pre-wrap text-sm leading-7 text-gray-700 dark:text-dark-200">{{ campaign.content }}</p>
          </div>

          <div v-if="campaign.type === 'lottery' && lotteryPools.length > 0" class="space-y-4">
            <div v-for="pool in lotteryPools" :key="`${pool.id}-side`" class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900/90">
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('activityCenter.lottery.pool') }}</p>
                <h2 class="mt-1 truncate text-base font-semibold text-gray-900 dark:text-white">{{ activityText(pool.name, t) || t('activityCenter.lottery.pool') }}</h2>
                </div>
                <span class="inline-flex shrink-0 items-center rounded-full bg-primary-50 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-200">
                  {{ pool.can_draw === false ? t('activityCenter.lottery.notEligible') : t('activityCenter.lottery.drawNow') }}
                </span>
              </div>

              <div v-if="wheelState(pool.id).result" class="mt-4 rounded-lg border border-primary-100 bg-primary-50 px-3 py-2 text-sm font-medium text-primary-700 dark:border-primary-900/40 dark:bg-primary-900/15 dark:text-primary-200">
                {{ wheelState(pool.id).result }}
              </div>

              <div class="mt-4 grid gap-2">
                <div v-for="prize in pool.prizes" :key="`${pool.id}-${prize.id}-side`" class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
                  <div class="flex items-center gap-2">
                    <span class="h-2.5 w-2.5 rounded-full" :style="{ backgroundColor: prize.color || '#8b5cf6' }"></span>
                    <span class="min-w-0 flex-1 truncate text-sm font-medium text-gray-900 dark:text-white">{{ activityText(prize.label, t) || prizeTypeLabel(prize.prize_type) }}</span>
                  </div>
                  <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ prizeTypeLabel(prize.prize_type) }}</p>
                </div>
              </div>
            </div>
          </div>

        </aside>

        <div class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900/90 lg:col-span-2">
            <div class="flex items-center justify-between gap-3">
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('activityCenter.records.title') }}</h2>
              <div class="flex items-center gap-2">
                <RouterLink to="/activity-center/records" class="text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-300">{{ t('activityCenter.records.viewAll') }}</RouterLink>
                <button type="button" class="rounded-lg p-1.5 text-gray-500 hover:bg-gray-100 dark:hover:bg-dark-800" :title="t('activityCenter.refresh')" @click="loadRecords">
                  <Icon name="refresh" size="sm" :class="recordsLoading ? 'animate-spin' : ''" />
                </button>
              </div>
            </div>
            <div v-if="records.length > 0" class="mt-4 divide-y divide-gray-100 dark:divide-dark-700">
              <div v-for="record in records" :key="record.id" class="border-b border-gray-100 py-4 last:border-b-0 dark:border-dark-700">
                <div class="relative min-w-0 pr-20">
                  <div class="min-w-0">
                    <p class="truncate text-sm font-medium text-gray-900 dark:text-white">
                      {{ activityText(record.prize_label, t) || activityText(record.campaign_title, t) }}
                    </p>
                    <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
                      {{ record.pool_name || typeLabel(record.campaign_type) }} · {{ formatTime(record.created_at) }}
                    </p>
                    <p v-if="record.reward_value" class="mt-2 break-words text-sm font-medium text-primary-700 dark:text-primary-300">
                      {{ record.reward_value }}
                      <span v-if="record.inflate_pct != null" class="ml-2 text-xs font-normal">(+{{ formatInflatePct(record.inflate_pct) }}%)</span>
                    </p>
                    <div v-if="record.reward_code" class="mt-2 border-l-2 border-primary-300 pl-2.5 dark:border-primary-600">
                      <div class="mb-1 flex items-center gap-1 text-xs font-medium text-gray-500 dark:text-dark-400">
                        <Icon name="key" size="xs" />
                        <span>{{ t('activityCenter.prizeTypes.card') }}</span>
                      </div>
                      <ul class="space-y-1">
                        <li v-for="code in rewardCodes(record)" :key="code" class="flex min-w-0 items-start gap-2 text-sm font-semibold leading-6 text-gray-800 dark:text-gray-100">
                          <span class="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-primary-400 dark:bg-primary-500"></span>
                          <code class="break-all">{{ code }}</code>
                        </li>
                      </ul>
                    </div>
                  </div>
                  <span :class="['absolute right-0 top-0 rounded-full px-2.5 py-1 text-xs font-semibold', recordStatusClass(record)]">
                    {{ recordStatusLabel(record) }}
                  </span>
                </div>
              </div>
            </div>
            <p v-else class="mt-4 text-sm text-gray-500 dark:text-dark-400">{{ t('activityCenter.records.empty') }}</p>
        </div>
      </section>

      <div v-else class="rounded-2xl border border-dashed border-gray-300 bg-white px-6 py-10 text-center dark:border-dark-700 dark:bg-dark-900/80">
        <Icon name="search" size="lg" class="mx-auto text-gray-400" />
        <h1 class="mt-4 text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('activityCenter.detail.notFoundTitle') }}
        </h1>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
          {{ t('activityCenter.detail.notFoundDescription') }}
        </p>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/core/stores/appStore'
import { extractI18nErrorMessage } from '@/core/utils/apiError'
import { formatDateTime } from '@/core/utils/format'
import activityCenterAPI from '@/features/activity-center/data/datasources/activityCenterDatasource'
import type { ActivityCampaignConfig, ActivityCheckinConfig, ActivityCheckinLeaderboardEntry, ActivityCheckinReward, ActivityCheckinStatus, ActivityInflateConfig, ActivityLotteryPool, ActivityLotteryPrize, ActivityParticipationRecord, UserActivityCampaign } from '@/types'

import AppLayout from '@/common/widgets/layout/AppLayout.vue'
import Icon from '@/common/widgets/icons/Icon.vue'
import { activityText } from '@/features/activity-center/presentation/activityCenterText'

const route = useRoute()
const { t } = useI18n()
const appStore = useAppStore()

const campaign = ref<UserActivityCampaign | null>(null)
const records = ref<ActivityParticipationRecord[]>([])
const loading = ref(false)
const recordsLoading = ref(false)
const redeemCodeInput = ref('')
const redeeming = ref(false)
const checkinLoading = ref(false)
const checkinStatus = ref<ActivityCheckinStatus>({ checked_today: false, streak_days: 0, cycle_day: 0 })
const leaderboardLoading = ref(false)
const leaderboard = ref<ActivityCheckinLeaderboardEntry[]>([])
const DRAW_SPIN_DURATION = 4200

interface WheelState {
  rotation: number
  spinning: boolean
  result: string
  resultPrizeId: string
}

const wheelStates = reactive<Record<string, WheelState>>({})
const wheelPalette = ['#ffffff', '#f8fafc', '#eef2f7', '#f3f4f6']

const campaignId = computed(() => {
  const raw = Array.isArray(route.params.id) ? route.params.id[0] : route.params.id
  const id = Number(raw)
  return Number.isFinite(id) && id > 0 ? id : null
})

function typeLabel(type: UserActivityCampaign['type']) {
  return t(`activityCenter.types.${type}`)
}

const parsedConfig = computed<ActivityCampaignConfig>(() => {
  if (!campaign.value?.config_json) return {}
  try {
    return JSON.parse(campaign.value.config_json) as ActivityCampaignConfig
  } catch {
    return {}
  }
})

const lotteryPools = computed(() => parsedConfig.value.lottery?.pools?.filter((pool) => pool.enabled !== false) || [])
const inflateConfig = computed<ActivityInflateConfig>(() => parsedConfig.value.inflate || parsedConfig.value.redeem || {
  min_value: '0',
  max_value: '0',
  min_inflate_pct: '0',
  max_inflate_pct: '0',
  required_group_ids: [],
  priority: 0
})
const checkinConfig = computed<ActivityCheckinConfig>(() => parsedConfig.value.checkin || { timezone: 'Asia/Shanghai', cycle_type: 'weekly', required_group_ids: [], daily_rewards: [], streak_mode: 'reset_on_miss' })
const checkinRewards = computed(() => checkinConfig.value.daily_rewards || [])
const checkinTargetDay = computed(() => {
  if (checkinStatus.value.checked_today) return Math.max(1, checkinStatus.value.cycle_day)
  const nextDay = Math.max(1, checkinStatus.value.cycle_day + 1)
  const maxDay = checkinRewards.value.reduce((max, reward) => Math.max(max, reward.day), 1)
  return nextDay > maxDay ? 1 : nextDay
})
const currentCheckinReward = computed(() => checkinRewards.value.find((reward) => reward.day === checkinTargetDay.value))

function checkinRewardText(reward: ActivityCheckinReward) {
  const value = String(reward.value || '').trim()
  return `${prizeTypeLabel(reward.reward_type)} ${value}`.trim()
}

function isCheckinRewardClaimed(day: number) {
  return checkinStatus.value.checked_today ? day <= checkinStatus.value.cycle_day : day < checkinTargetDay.value
}

function checkinRewardClass(day: number) {
  const base = 'min-w-0 rounded-lg border p-3 transition-colors'
  if (isCheckinRewardClaimed(day)) return `${base} border-emerald-200 bg-emerald-50 text-emerald-800 dark:border-emerald-900/50 dark:bg-emerald-900/20 dark:text-emerald-200`
  if (day === checkinTargetDay.value) return `${base} border-amber-300 bg-amber-50 text-amber-900 ring-1 ring-amber-200 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-100 dark:ring-amber-900`
  return `${base} border-gray-200 bg-white text-gray-500 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-400`
}

async function loadCheckinStatus() {
  if (campaign.value?.type !== 'checkin') return
  try { checkinStatus.value = await activityCenterAPI.getCheckinStatus(campaign.value.id) } catch { /* detail load owns the visible error */ }
}

async function loadCheckinLeaderboard() {
  if (campaign.value?.type !== 'checkin') return
  leaderboardLoading.value = true
  try { leaderboard.value = await activityCenterAPI.getCheckinLeaderboard(campaign.value.id) } catch { leaderboard.value = [] } finally { leaderboardLoading.value = false }
}

async function submitCheckin() {
  if (!campaign.value || checkinLoading.value || checkinStatus.value.checked_today) return
  checkinLoading.value = true
  try {
    const result = await activityCenterAPI.checkin(campaign.value.id)
    checkinStatus.value = result.status
    records.value = [result.record, ...records.value]
  } catch (error: any) {
    appStore.showError(extractI18nErrorMessage(error, t, 'activityCenter.errors', t('activityCenter.checkin.failed')))
  } finally { checkinLoading.value = false }
}

function activityTimeRange(item: UserActivityCampaign) {
  if (item.starts_at && item.ends_at) return `${formatTime(item.starts_at)} - ${formatTime(item.ends_at)}`
  if (item.starts_at) return t('activityCenter.fields.startsAtValue', { value: formatTime(item.starts_at) })
  return t('activityCenter.fields.endsAtValue', { value: formatTime(item.ends_at) })
}

function formatTime(raw?: string) {
  return raw ? formatDateTime(raw) : t('activityCenter.noTime')
}

function formatInflatePct(value: number) {
  return Number(value).toFixed(2).replace(/\.00$/, '').replace(/(\.\d)0$/, '$1')
}

async function redeemCode() {
  const code = redeemCodeInput.value.trim()
  if (!code || redeeming.value) return
  redeeming.value = true
  try {
    await activityCenterAPI.redeemCode(code)
    redeemCodeInput.value = ''
    appStore.showSuccess(t('activityCenter.inflate.success'))
    await loadRecords()
  } catch (error: any) {
    appStore.showError(extractI18nErrorMessage(error, t, 'activityCenter.errors', t('activityCenter.inflate.failed')))
  } finally {
    redeeming.value = false
  }
}

function prizeTypeLabel(type: ActivityLotteryPrize['prize_type']) {
  return t(`activityCenter.prizeTypes.${type}`)
}

function wheelState(poolId: string): WheelState {
  if (!wheelStates[poolId]) {
    wheelStates[poolId] = {
      rotation: 0,
      spinning: false,
      result: '',
      resultPrizeId: ''
    }
  }
  return wheelStates[poolId]
}

function wheelStyle(pool: ActivityLotteryPool) {
  const state = wheelState(pool.id)
  return {
    transform: `rotate(${state.rotation}deg)`,
    transition: state.spinning ? `transform ${DRAW_SPIN_DURATION}ms cubic-bezier(0.15, 0.85, 0.15, 1)` : 'none'
  }
}

interface PrizeWeightSlice {
  id: string
  color: string
  path: string
  label: string
  labelX: number
  labelY: number
  labelRotation: number
  startAngle: number
  endAngle: number
  midAngle: number
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
  return `M ${start.x} ${start.y} A ${radius} ${radius} 0 ${largeArcFlag} 0 ${end.x} ${end.y} L 60 60 Z`
}

function wheelPrizeLabel(prize: ActivityLotteryPrize) {
  const label = prize.label || prizeTypeLabel(prize.prize_type)
  return label.length > 8 ? `${label.slice(0, 8)}...` : label
}

function prizeWeightSlices(pool: ActivityLotteryPool): PrizeWeightSlice[] {
  if (pool.prizes.length === 0) return []
  let currentAngle = 0
  return pool.prizes.map((prize, index) => {
    const angle = 360 / pool.prizes.length
    const startAngle = currentAngle
    const endAngle = Math.min(359.99, currentAngle + angle)
    currentAngle += angle
    const midAngle = startAngle + (endAngle - startAngle) / 2
    const labelPoint = polarToCartesian(60, 60, 35, midAngle)
    const labelRotation = midAngle > 90 && midAngle < 270 ? midAngle + 180 : midAngle
    return {
      id: prize.id,
      color: prize.color || wheelPalette[index % wheelPalette.length],
      path: describeArc(60, 60, 58, startAngle, endAngle),
      label: wheelPrizeLabel(prize),
      labelX: labelPoint.x,
      labelY: labelPoint.y,
      labelRotation,
      startAngle,
      endAngle,
      midAngle
    }
  })
}

function wait(ms: number) {
  return new Promise<void>((resolve) => {
    window.setTimeout(resolve, ms)
  })
}

async function drawPool(pool: ActivityLotteryPool) {
  if (pool.can_draw === false || pool.prizes.length === 0) return
  const state = wheelState(pool.id)
  if (state.spinning) return

  state.result = ''
  state.resultPrizeId = ''
  state.spinning = true

  let record: ActivityParticipationRecord
  try {
    record = await activityCenterAPI.participate(campaign.value?.id || 0, pool.id)
  } catch (error: any) {
    state.spinning = false
    appStore.showError(
      extractI18nErrorMessage(
        error,
        t,
        'activityCenter.errors',
        t('activityCenter.lottery.drawFailed'),
      ),
    )
    return
  }

  const slice = prizeWeightSlices(pool).find((item) => item.id === record.prize_id)
  const sliceSpan = slice ? slice.endAngle - slice.startAngle : 0
  const safeMargin = slice ? Math.min(10, Math.max(6, sliceSpan * 0.18)) : 0
  const usableSpan = slice ? Math.max(1, sliceSpan - safeMargin * 2) : 0
  const targetAngle = slice ? slice.startAngle + safeMargin + Math.random() * usableSpan : 0
  const extraTurns = 5 + Math.floor(Math.random() * 2)
  const normalizedRotation = ((state.rotation % 360) + 360) % 360
  const targetRotation = (360 - targetAngle) % 360
  const forwardRotation = (targetRotation - normalizedRotation + 360) % 360

  await wait(16)
  state.rotation += extraTurns * 360 + forwardRotation
  await wait(DRAW_SPIN_DURATION)
  state.spinning = false
  state.resultPrizeId = record.prize_id
  state.result = t('activityCenter.lottery.drawResult', {
    name: record.prize_label || prizeTypeLabel(record.prize_type || 'none')
  })
  records.value = [record]
}

async function loadCampaign() {
  if (!campaignId.value) {
    campaign.value = null
    return
  }
  loading.value = true
  try {
		campaign.value = await activityCenterAPI.getById(campaignId.value)
		await loadCheckinStatus()
		await loadCheckinLeaderboard()
    await loadRecords()
  } catch (error: any) {
    campaign.value = null
    console.error('Failed to load activity campaign', error)
    appStore.showError(error?.message || t('activityCenter.detail.failedToLoad'))
  } finally {
    loading.value = false
  }
}

async function loadRecords() {
  recordsLoading.value = true
  try {
    const response = await activityCenterAPI.listMyRecords(1, 1, campaignId.value ?? undefined)
    records.value = response.items.slice(0, 1)
  } catch (error: any) {
    appStore.showError(error?.message || t('activityCenter.records.failedToLoad'))
  } finally {
    recordsLoading.value = false
  }
}

function recordStatusLabel(record: ActivityParticipationRecord) {
  if (record.result_status === 'none') return t('activityCenter.records.none')
  if (record.reward_status === 'pending') return t('activityCenter.records.pending')
  if (record.reward_status === 'granted') return t('activityCenter.records.granted')
  if (record.reward_status === 'failed') return t('activityCenter.records.failed')
  return t('activityCenter.records.recorded')
}

function recordStatusClass(record: ActivityParticipationRecord) {
  if (record.result_status === 'none') return 'bg-gray-100 text-gray-600 dark:bg-dark-800 dark:text-dark-300'
  if (record.reward_status === 'granted') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
  if (record.reward_status === 'failed') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  if (record.reward_status === 'pending') return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
}

function rewardCodes(record: ActivityParticipationRecord) {
  return (record.reward_code || '')
    .split(/\r?\n/)
    .map((code) => code.trim())
    .filter(Boolean)
}

watch(campaignId, () => {
  void loadCampaign()
})

onMounted(() => {
  void loadCampaign()
})
</script>
