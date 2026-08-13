<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import Select from "@/common/widgets/forms/Select.vue";
import Icon from "@/common/widgets/icons/Icon.vue";
import type { BillingMode } from "@/core/constants/channel";
import type { GroupPlatform } from "@/types";
import type { GroupPricingFormEntry } from "../groupsModelPricing";
import GroupModelTagInput from "./GroupModelTagInput.vue";

type GroupPricingPriceField =
  | "input_price"
  | "output_price"
  | "cache_write_price"
  | "cache_read_price"
  | "image_input_price"
  | "image_output_price"
  | "per_request_price";

const props = defineProps<{
  entry: GroupPricingFormEntry;
  platform: GroupPlatform;
}>();

const emit = defineEmits<{
  update: [entry: GroupPricingFormEntry];
  remove: [];
  "update:models": [models: string[]];
}>();

const { t } = useI18n();
const $t = (key: string): string => t(key);
const collapsed = ref(props.entry.models.length > 0);
const billingModeOptions = computed(() => [
  { value: "token", label: $t("admin.channels.billingMode.token") },
  { value: "per_request", label: $t("admin.channels.billingMode.perRequest") },
  { value: "image", label: $t("admin.channels.billingMode.image") },
  { value: "video", label: $t("admin.channels.billingMode.video") },
]);
const billingModeLabel = computed(
  () =>
    billingModeOptions.value.find(
      (option) => option.value === props.entry.billing_mode,
    )?.label || props.entry.billing_mode,
);
const tokenPriceFields: Array<{
  key: GroupPricingPriceField;
  label: string;
}> = [
  { key: "input_price", label: "inputPrice" },
  { key: "output_price", label: "outputPrice" },
  { key: "cache_write_price", label: "cacheWritePrice" },
  { key: "cache_read_price", label: "cacheReadPrice" },
  { key: "image_input_price", label: "imageInputPrice" },
  { key: "image_output_price", label: "imageTokenPrice" },
];

const updateField = (
  field: GroupPricingPriceField,
  value: string | BillingMode,
) => {
  emit("update", {
    ...props.entry,
    [field]: value === "" ? null : value,
  });
};

const changeBillingMode = (mode: BillingMode) => {
  emit("update", { ...props.entry, billing_mode: mode, intervals: [] });
};

const addTier = () => {
  const intervals = [...props.entry.intervals];
  const labels =
    props.entry.billing_mode === "video"
      ? ["480p", "720p", "1080p"]
      : props.entry.billing_mode === "image"
        ? ["1K", "2K", "4K", "HD"]
        : ["realtime", "tts", "stt"];
  intervals.push({
    min_tokens: 0,
    max_tokens: null,
    tier_label: labels[intervals.length] || "",
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    per_request_price: null,
    sort_order: intervals.length,
  });
  emit("update", { ...props.entry, intervals });
};

const updateTier = (
  index: number,
  field: "tier_label" | "per_request_price",
  value: string,
) => {
  const intervals = [...props.entry.intervals];
  intervals[index] = {
    ...intervals[index],
    [field]: value === "" ? null : value,
  };
  emit("update", { ...props.entry, intervals });
};

const removeTier = (index: number) => {
  const intervals = [...props.entry.intervals];
  intervals.splice(index, 1);
  emit("update", {
    ...props.entry,
    intervals: intervals.map((interval, sortOrder) => ({
      ...interval,
      sort_order: sortOrder,
    })),
  });
};
</script>

<template>
  <div class="rounded border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-800">
    <div class="flex items-center gap-2">
      <button
        type="button"
        class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-700 dark:hover:text-gray-200"
        :title="collapsed ? $t('common.expand') : $t('common.collapse')"
        @click="collapsed = !collapsed"
      >
        <Icon :name="collapsed ? 'chevronRight' : 'chevronDown'" size="sm" />
      </button>
      <div class="min-w-0 flex-1">
        <div v-if="collapsed" class="flex items-center gap-2">
          <span class="min-w-0 flex-1 truncate text-sm text-gray-700 dark:text-gray-300">
            {{ entry.models.join(", ") || $t("admin.channels.form.noModels") }}
          </span>
          <span class="rounded bg-primary-100 px-2 py-0.5 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300">
            {{ billingModeLabel }}
          </span>
        </div>
        <span v-else class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ $t("admin.channels.form.pricingEntry") }}
        </span>
      </div>
      <button
        type="button"
        class="rounded p-1 text-gray-400 hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-950/30"
        :title="$t('common.delete')"
        @click="emit('remove')"
      >
        <Icon name="trash" size="sm" />
      </button>
    </div>

    <div v-if="!collapsed" class="mt-3 space-y-3">
      <div class="grid gap-3 md:grid-cols-[minmax(0,1fr)_10rem]">
        <div>
          <label class="input-label">
            {{ $t("admin.channels.form.models") }}
          </label>
          <GroupModelTagInput
            :models="entry.models"
            :platform="platform"
            :placeholder="$t('admin.channels.form.modelsPlaceholder')"
            @update:models="emit('update:models', $event)"
          />
        </div>
        <div>
          <label class="input-label">
            {{ $t("admin.channels.form.billingMode") }}
          </label>
          <Select
            :model-value="entry.billing_mode"
            :options="billingModeOptions"
            @update:model-value="changeBillingMode($event as BillingMode)"
          />
        </div>
      </div>

      <div v-if="entry.billing_mode === 'token'">
        <div class="mb-1 flex items-center gap-2">
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ $t("admin.channels.form.defaultPrices") }}
          </span>
          <span class="text-xs text-gray-400">$/MTok</span>
        </div>
        <div class="grid grid-cols-2 gap-2 lg:grid-cols-3">
          <label v-for="field in tokenPriceFields" :key="field.key" class="text-xs text-gray-400">
            {{ $t(`admin.channels.form.${field.label}`) }}
            <input
              :value="entry[field.key]"
              type="number"
              min="0"
              step="any"
              class="input mt-1 text-sm"
              :placeholder="$t('admin.channels.form.pricePlaceholder')"
              @input="updateField(field.key, ($event.target as HTMLInputElement).value)"
            />
          </label>
        </div>
      </div>

      <div v-else>
        <label class="block max-w-64 text-xs text-gray-400">
          {{
            $t(
              entry.billing_mode === 'video'
                ? 'admin.channels.form.defaultVideoPrice'
                : entry.billing_mode === 'image'
                  ? 'admin.channels.form.defaultImagePrice'
                  : 'admin.channels.form.defaultPerRequestPrice',
            )
          }}
          <input
            :value="entry.per_request_price"
            type="number"
            min="0"
            step="any"
            class="input mt-1 text-sm"
            :placeholder="$t('admin.channels.form.pricePlaceholder')"
            @input="updateField('per_request_price', ($event.target as HTMLInputElement).value)"
          />
        </label>
        <div class="mt-3 flex items-center justify-between gap-3">
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">
            {{
              $t(
                entry.billing_mode === 'video'
                  ? 'admin.channels.form.videoTiers'
                  : entry.billing_mode === 'image'
                    ? 'admin.channels.form.imageTiers'
                    : 'admin.channels.form.requestTiers',
              )
            }}
          </span>
          <button
            type="button"
            class="inline-flex items-center gap-1 text-xs text-primary-600 hover:text-primary-700"
            @click="addTier"
          >
            <Icon name="plus" size="xs" />
            {{ $t("admin.channels.form.addTier") }}
          </button>
        </div>
        <div v-if="entry.intervals.length" class="mt-2 space-y-2">
          <div
            v-for="(interval, index) in entry.intervals"
            :key="index"
            class="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_2rem] items-end gap-2 rounded border border-gray-200 bg-white p-2 dark:border-dark-600 dark:bg-dark-700"
          >
            <label class="text-xs text-gray-400">
              {{
                $t(
                  entry.billing_mode === 'image' || entry.billing_mode === 'video'
                    ? 'admin.channels.form.resolution'
                    : 'admin.channels.form.tierLabel',
                )
              }}
              <input
                :value="interval.tier_label"
                type="text"
                class="input mt-1 text-sm"
                @input="updateTier(index, 'tier_label', ($event.target as HTMLInputElement).value)"
              />
            </label>
            <label class="text-xs text-gray-400">
              {{ $t("admin.channels.form.perRequestPrice") }} ($)
              <input
                :value="interval.per_request_price"
                type="number"
                min="0"
                step="any"
                class="input mt-1 text-sm"
                @input="updateTier(index, 'per_request_price', ($event.target as HTMLInputElement).value)"
              />
            </label>
            <button
              type="button"
              class="mb-1 rounded p-1 text-gray-400 hover:text-red-500"
              :title="$t('common.delete')"
              @click="removeTier(index)"
            >
              <Icon name="x" size="sm" />
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
