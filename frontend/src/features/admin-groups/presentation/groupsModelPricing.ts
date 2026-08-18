import type { BillingMode } from "@/core/constants/channel";
import type {
  ChannelModelPricing,
  GroupPlatform,
  PricingInterval,
} from "@/types/group";
import type { ModelDefaultPricing } from "../data/dtos/adminGroupDtos";

export interface GroupPricingIntervalFormEntry {
  min_tokens: number;
  max_tokens: number | null;
  tier_label: string;
  input_price: number | string | null;
  output_price: number | string | null;
  cache_write_price: number | string | null;
  cache_read_price: number | string | null;
  per_request_price: number | string | null;
  sort_order: number;
}

export interface GroupPricingFormEntry {
  models: string[];
  billing_mode: BillingMode;
  input_price: number | string | null;
  output_price: number | string | null;
  cache_write_price: number | string | null;
  cache_read_price: number | string | null;
  image_input_price: number | string | null;
  image_output_price: number | string | null;
  per_request_price: number | string | null;
  intervals: GroupPricingIntervalFormEntry[];
}

const MTOK = 1_000_000;

const toNullableNumber = (
  value: number | string | null | undefined,
): number | null => {
  if (value === null || value === undefined || value === "") return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
};

const mTokToPerToken = (
  value: number | string | null | undefined,
): number | null => {
  const parsed = toNullableNumber(value);
  return parsed === null ? null : Number((parsed / MTOK).toPrecision(10));
};

const perTokenToMTok = (value: number | null | undefined): number | null => {
  if (value === null || value === undefined) return null;
  return Number((value * MTOK).toPrecision(10));
};

const intervalFromAPI = (
  interval: PricingInterval,
): GroupPricingIntervalFormEntry => ({
  min_tokens: interval.min_tokens,
  max_tokens: interval.max_tokens,
  tier_label: interval.tier_label || "",
  input_price: perTokenToMTok(interval.input_price),
  output_price: perTokenToMTok(interval.output_price),
  cache_write_price: perTokenToMTok(interval.cache_write_price),
  cache_read_price: perTokenToMTok(interval.cache_read_price),
  per_request_price: interval.per_request_price,
  sort_order: interval.sort_order,
});

const intervalToAPI = (
  interval: GroupPricingIntervalFormEntry,
): PricingInterval => ({
  min_tokens: interval.min_tokens,
  max_tokens: interval.max_tokens,
  tier_label: interval.tier_label.trim(),
  input_price: mTokToPerToken(interval.input_price),
  output_price: mTokToPerToken(interval.output_price),
  cache_write_price: mTokToPerToken(interval.cache_write_price),
  cache_read_price: mTokToPerToken(interval.cache_read_price),
  per_request_price: toNullableNumber(interval.per_request_price),
  sort_order: interval.sort_order,
});

export const createGroupPricingEntry = (): GroupPricingFormEntry => ({
  models: [],
  billing_mode: "token",
  input_price: null,
  output_price: null,
  cache_write_price: null,
  cache_read_price: null,
  image_input_price: null,
  image_output_price: null,
  per_request_price: null,
  intervals: [],
});

export const groupPricingFromAPI = (
  pricing: ChannelModelPricing[] | null | undefined,
): GroupPricingFormEntry[] =>
  (pricing || []).map((entry) => ({
    models: [...(entry.models || [])],
    billing_mode: entry.billing_mode || "token",
    input_price: perTokenToMTok(entry.input_price),
    output_price: perTokenToMTok(entry.output_price),
    cache_write_price: perTokenToMTok(entry.cache_write_price),
    cache_read_price: perTokenToMTok(entry.cache_read_price),
    image_input_price: perTokenToMTok(entry.image_input_price),
    image_output_price: perTokenToMTok(entry.image_output_price),
    per_request_price: entry.per_request_price,
    intervals: (entry.intervals || []).map(intervalFromAPI),
  }));

export const groupPricingToAPI = (
  pricing: GroupPricingFormEntry[],
  platform: GroupPlatform,
): ChannelModelPricing[] =>
  pricing
    .map((entry) => ({
      entry,
      models: entry.models.map((model) => model.trim()).filter(Boolean),
    }))
    .filter(({ models }) => models.length > 0)
    .map(({ entry, models }) => ({
      platform,
      models,
      billing_mode: entry.billing_mode,
      input_price: mTokToPerToken(entry.input_price),
      output_price: mTokToPerToken(entry.output_price),
      cache_write_price: mTokToPerToken(entry.cache_write_price),
      cache_read_price: mTokToPerToken(entry.cache_read_price),
      image_input_price: mTokToPerToken(entry.image_input_price),
      image_output_price: mTokToPerToken(entry.image_output_price),
      per_request_price: toNullableNumber(entry.per_request_price),
      intervals:
        entry.billing_mode === "token"
          ? []
          : (entry.intervals || []).map(intervalToAPI),
    }));

const entryHasConfiguredPrice = (entry: GroupPricingFormEntry): boolean =>
  [
    entry.input_price,
    entry.output_price,
    entry.cache_write_price,
    entry.cache_read_price,
    entry.image_input_price,
    entry.image_output_price,
    entry.per_request_price,
  ].some((value) => value !== null && value !== undefined && value !== "");

export const updateGroupPricingModels = async (
  pricing: GroupPricingFormEntry[],
  index: number,
  models: string[],
  loadDefaultPricing: (model: string) => Promise<ModelDefaultPricing>,
): Promise<void> => {
  const current = pricing[index];
  if (!current) return;

  const normalizedModels = [...new Set(models.map((model) => model.trim()).filter(Boolean))];
  const addedModel = normalizedModels.find((model) => !current.models.includes(model));
  pricing[index] = { ...current, models: normalizedModels };
  if (!addedModel || current.billing_mode !== "token" || entryHasConfiguredPrice(current)) {
    return;
  }

  try {
    const defaults = await loadDefaultPricing(addedModel);
    const latest = pricing[index];
    if (
      !defaults.found ||
      !latest ||
      latest.billing_mode !== "token" ||
      !latest.models.includes(addedModel) ||
      entryHasConfiguredPrice(latest)
    ) {
      return;
    }
    pricing[index] = {
      ...latest,
      input_price: perTokenToMTok(defaults.input_price),
      output_price: perTokenToMTok(defaults.output_price),
      cache_write_price: perTokenToMTok(defaults.cache_write_price),
      cache_read_price: perTokenToMTok(defaults.cache_read_price),
      image_input_price: perTokenToMTok(defaults.image_input_price),
      image_output_price: perTokenToMTok(defaults.image_output_price),
    };
  } catch {
    // Default lookup is optional; manual pricing remains available.
  }
};

export const groupPricingTagClass = (platform: GroupPlatform): string => {
  switch (platform) {
    case "anthropic":
      return "bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400";
    case "openai":
      return "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400";
    case "gemini":
      return "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400";
    case "antigravity":
      return "bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-400";
    case "grok":
      return "bg-zinc-100 text-zinc-700 dark:bg-zinc-800 dark:text-zinc-300";
    default:
      return "bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300";
  }
};
