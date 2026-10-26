<script setup lang="ts">
import { useI18n } from "vue-i18n";
import {
  getImagePricePlaceholder,
  getVideoPricePlaceholder,
  imagePricingI18nKey,
  supportsImagePricingPlatform,
  supportsVideoPricingPlatform,
  videoPricingI18nKey,
} from "../groupsImagePricingResolver";
import type { GroupEditorDialogContext } from "../groupEditorContext";

type GroupEditorMediaPricingContext = Pick<
  GroupEditorDialogContext,
  "form" | "imageFinalPricePreview" | "videoFinalPricePreview"
>;

const props = defineProps<{
  context: GroupEditorMediaPricingContext;
}>();

const { t } = useI18n();
const { form, imageFinalPricePreview, videoFinalPricePreview } = props.context;
</script>

<template>
  <div
    v-if="supportsImagePricingPlatform(form.platform)"
    class="border-t pt-4"
  >
    <label class="block mb-2 font-medium text-gray-700 dark:text-gray-300">
      {{ t(imagePricingI18nKey(form.platform, "title")) }}
    </label>
    <p class="text-xs text-gray-500 dark:text-gray-400 mb-3">
      {{ t(imagePricingI18nKey(form.platform, "description")) }}
    </p>
    <div class="mb-4 grid grid-cols-1 gap-3 md:grid-cols-2">
      <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
        <input
          v-model="form.allow_image_generation"
          type="checkbox"
          class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
        />
        {{ t(imagePricingI18nKey(form.platform, "allowImageGeneration")) }}
      </label>
      <label
        v-if="['openai', 'composite'].includes(form.platform)"
        class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300"
        :title="t('admin.groups.imagePricing.forceImageToolHint')"
      >
        <input
          v-model="form.openai_force_image_tool"
          type="checkbox"
          class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
        />
        {{ t("admin.groups.imagePricing.forceImageTool") }}
      </label>
      <label
        v-if="form.allow_image_generation && ['openai', 'composite'].includes(form.platform)"
        class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300"
        :title="t('admin.groups.imagePricing.perImageBillingHint')"
      >
        <input v-model="form.image_count_billing" type="checkbox" class="rounded border-gray-300 text-blue-600 focus:ring-blue-500" />
        {{ t("admin.groups.imagePricing.perImageBilling") }}
      </label>
      <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
        <input
          v-model="form.image_rate_independent"
          type="checkbox"
          class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
        />
        {{ t(imagePricingI18nKey(form.platform, "independentMultiplier")) }}
      </label>
    </div>
    <div v-if="form.image_rate_independent" class="mb-4">
      <label class="input-label">{{
        t(imagePricingI18nKey(form.platform, "imageMultiplier"))
      }}</label>
      <input
        v-model.number="form.image_rate_multiplier"
        type="number"
        step="0.0001"
        min="0"
        class="input"
        placeholder="1"
      />
    </div>
    <div class="grid grid-cols-3 gap-3">
      <div>
        <label class="input-label">1K ($)</label>
        <input
          v-model.number="form.image_price_1k"
          type="number"
          step="0.001"
          min="0"
          class="input"
          :placeholder="getImagePricePlaceholder(form.platform, 'image_price_1k')"
        />
      </div>
      <div>
        <label class="input-label">2K ($)</label>
        <input
          v-model.number="form.image_price_2k"
          type="number"
          step="0.001"
          min="0"
          class="input"
          :placeholder="getImagePricePlaceholder(form.platform, 'image_price_2k')"
        />
      </div>
      <div>
        <label class="input-label">4K ($)</label>
        <input
          v-model.number="form.image_price_4k"
          type="number"
          step="0.001"
          min="0"
          class="input"
          :placeholder="getImagePricePlaceholder(form.platform, 'image_price_4k')"
        />
      </div>
    </div>
    <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">
      {{ t(imagePricingI18nKey(form.platform, "modeHint")) }}
    </p>
    <div class="mt-2 rounded-lg bg-gray-50 p-3 text-xs text-gray-700 dark:bg-gray-800 dark:text-gray-300">
      <div class="mb-1 font-medium">
        {{ t(imagePricingI18nKey(form.platform, "finalPricePreview")) }}
      </div>
      <div class="grid grid-cols-3 gap-2">
        <div v-for="item in imageFinalPricePreview" :key="item.label">
          {{ item.label }}: {{ item.value }}
        </div>
      </div>
    </div>
    <div
      v-if="form.platform === 'gemini' && form.allow_image_generation"
      class="mt-4 border-t border-dashed border-gray-200 pt-4 dark:border-dark-700"
    >
      <label class="flex items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-300">
        <input
          v-model="form.allow_batch_image_generation"
          type="checkbox"
          class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
        />
        {{ t("admin.groups.imagePricing.allowBatchImageGeneration") }}
      </label>
      <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
        {{ t("admin.groups.imagePricing.batchSectionHint") }}
      </p>
      <div
        v-if="form.allow_batch_image_generation"
        class="mt-3 grid grid-cols-1 gap-3 md:grid-cols-2"
      >
        <div>
          <label class="input-label">{{
            t("admin.groups.imagePricing.batchDiscountMultiplier")
          }}</label>
          <input
            v-model.number="form.batch_image_discount_multiplier"
            type="number"
            step="0.0001"
            min="0"
            class="input"
            placeholder="0.5"
          />
        </div>
        <div>
          <label class="input-label">{{
            t("admin.groups.imagePricing.batchHoldMultiplier")
          }}</label>
          <input
            v-model.number="form.batch_image_hold_multiplier"
            type="number"
            step="0.0001"
            min="0"
            class="input"
            placeholder="0.6"
          />
        </div>
      </div>
    </div>
    <p
      v-else-if="form.platform !== 'gemini'"
      class="mt-4 border-t border-dashed border-gray-200 pt-4 text-xs text-gray-500 dark:border-dark-700 dark:text-gray-400"
    >
      {{ t("admin.groups.imagePricing.batchGeminiOnlyHint") }}
    </p>
  </div>

  <div
    v-if="supportsVideoPricingPlatform(form.platform)"
    class="border-t pt-4"
  >
    <label class="block mb-2 font-medium text-gray-700 dark:text-gray-300">
      {{ t(videoPricingI18nKey("title")) }}
    </label>
    <p class="text-xs text-gray-500 dark:text-gray-400 mb-3">
      {{ t(videoPricingI18nKey("description")) }}
    </p>
    <div class="mb-4">
      <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
        <input
          v-model="form.video_rate_independent"
          type="checkbox"
          class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
        />
        {{ t(videoPricingI18nKey("independentMultiplier")) }}
      </label>
    </div>
    <div v-if="form.video_rate_independent" class="mb-4">
      <label class="input-label">{{
        t(videoPricingI18nKey("videoMultiplier"))
      }}</label>
      <input
        v-model.number="form.video_rate_multiplier"
        type="number"
        step="0.0001"
        min="0"
        class="input"
        placeholder="1"
      />
    </div>
    <div class="grid grid-cols-3 gap-3">
      <div>
        <label class="input-label">480p ($/s)</label>
        <input
          v-model.number="form.video_price_480p"
          type="number"
          step="0.001"
          min="0"
          class="input"
          :placeholder="getVideoPricePlaceholder(form.platform, 'video_price_480p')"
        />
      </div>
      <div>
        <label class="input-label">720p ($/s)</label>
        <input
          v-model.number="form.video_price_720p"
          type="number"
          step="0.001"
          min="0"
          class="input"
          :placeholder="getVideoPricePlaceholder(form.platform, 'video_price_720p')"
        />
      </div>
      <div>
        <label class="input-label">1080p ($/s)</label>
        <input
          v-model.number="form.video_price_1080p"
          type="number"
          step="0.001"
          min="0"
          class="input"
          :placeholder="getVideoPricePlaceholder(form.platform, 'video_price_1080p')"
        />
      </div>
    </div>
    <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">
      {{ t(videoPricingI18nKey("modeHint")) }}
    </p>
    <div class="mt-2 rounded-lg bg-gray-50 p-3 text-xs text-gray-700 dark:bg-gray-800 dark:text-gray-300">
      <div class="mb-1 font-medium">
        {{ t(videoPricingI18nKey("finalPricePreview")) }}
      </div>
      <div class="grid grid-cols-3 gap-2">
        <div v-for="item in videoFinalPricePreview" :key="item.label">
          {{ item.label }}: {{ item.value }}
        </div>
      </div>
    </div>
  </div>
</template>
