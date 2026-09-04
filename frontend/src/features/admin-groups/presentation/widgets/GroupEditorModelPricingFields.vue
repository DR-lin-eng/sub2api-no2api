<script setup lang="ts">
import { useI18n } from "vue-i18n";
import Icon from "@/common/widgets/icons/Icon.vue";
import type { GroupEditorDialogContext } from "../groupEditorContext";
import GroupModelPricingEntry from "./GroupModelPricingEntry.vue";

const { context } = defineProps<{ context: GroupEditorDialogContext }>();
const { t } = useI18n();
const {
  addModelPricing,
  form,
  removeModelPricing,
  updateModelPricing,
  updateModelPricingModels,
} = context;
</script>

<template>
  <section class="border-t border-gray-200 pt-4 dark:border-dark-400">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div class="min-w-0 flex-1">
        <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t("admin.groups.modelPricing.title") }}
        </h4>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t("admin.groups.modelPricing.description") }}
        </p>
      </div>
      <button type="button" class="btn btn-secondary shrink-0 whitespace-nowrap" @click="addModelPricing">
        <Icon name="plus" size="sm" class="mr-1" />
        {{ t("admin.groups.modelPricing.add") }}
      </button>
    </div>

    <label class="mt-3 flex items-start gap-2">
      <input
        v-model="form.long_context_pricing_enabled"
        type="checkbox"
        class="mt-0.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
      />
      <span>
        <span class="block text-sm text-gray-700 dark:text-gray-300">
          {{ t("admin.groups.modelPricing.longContext") }}
        </span>
        <span class="block text-xs text-gray-500 dark:text-gray-400">
          {{ t("admin.groups.modelPricing.longContextHint") }}
        </span>
      </span>
    </label>

    <div v-if="form.model_pricing.length" class="mt-3 space-y-2">
      <GroupModelPricingEntry
        v-for="(entry, index) in form.model_pricing"
        :key="index"
        :entry="entry"
        :platform="form.platform"
        @update="updateModelPricing(index, $event)"
        @update:models="updateModelPricingModels(index, $event)"
        @remove="removeModelPricing(index)"
      />
    </div>
  </section>
</template>
