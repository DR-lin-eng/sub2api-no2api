<script setup lang="ts">
import { ref } from "vue";
import Icon from "@/common/widgets/icons/Icon.vue";
import type { GroupPlatform } from "@/types/group";
import { groupPricingTagClass } from "../groupsModelPricing";

const props = defineProps<{
  models: string[];
  platform: GroupPlatform;
  placeholder: string;
}>();

const emit = defineEmits<{
  "update:models": [models: string[]];
}>();

const inputValue = ref("");

const addModels = (values: string[]) => {
  const models = [
    ...new Set([
      ...props.models,
      ...values.map((value) => value.trim()).filter(Boolean),
    ]),
  ];
  if (models.length !== props.models.length) emit("update:models", models);
  inputValue.value = "";
};

const addCurrentModel = () => addModels([inputValue.value]);

const removeModel = (index: number) => {
  const models = [...props.models];
  models.splice(index, 1);
  emit("update:models", models);
};

const handleBackspace = () => {
  if (!inputValue.value && props.models.length > 0) {
    removeModel(props.models.length - 1);
  }
};

const handlePaste = (event: ClipboardEvent) => {
  const text = event.clipboardData?.getData("text") || "";
  const values = text.split(/[,\n;]+/).map((value) => value.trim()).filter(Boolean);
  if (values.length === 0) return;
  event.preventDefault();
  addModels(values);
};
</script>

<template>
  <div>
    <div
      class="flex min-h-10 flex-wrap items-center gap-1.5 rounded border border-gray-200 bg-white p-2 dark:border-dark-600 dark:bg-dark-800"
    >
      <span
        v-for="(model, index) in models"
        :key="model"
        :class="groupPricingTagClass(platform)"
        class="inline-flex max-w-full items-center gap-1 rounded px-2 py-0.5 text-sm"
      >
        <span class="break-all">{{ model }}</span>
        <button
          type="button"
          class="rounded p-0.5 hover:bg-black/10 dark:hover:bg-white/10"
          :title="$t('common.delete')"
          @click="removeModel(index)"
        >
          <Icon name="x" size="xs" />
        </button>
      </span>
      <input
        v-model="inputValue"
        type="text"
        class="min-w-32 flex-1 border-0 bg-transparent text-sm outline-none placeholder:text-gray-400 dark:text-white"
        :placeholder="models.length === 0 ? placeholder : ''"
        @keydown.enter.prevent="addCurrentModel"
        @keydown.tab.prevent="addCurrentModel"
        @keydown.delete="handleBackspace"
        @paste="handlePaste"
        @blur="addCurrentModel"
      />
    </div>
    <p class="mt-1 text-xs text-gray-400">
      {{ $t("admin.groups.modelPricing.modelInputHint") }}
    </p>
  </div>
</template>
