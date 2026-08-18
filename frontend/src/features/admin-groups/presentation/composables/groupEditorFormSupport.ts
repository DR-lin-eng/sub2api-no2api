import type { GroupPlatform, SubscriptionType } from "@/types/group";
import { createDefaultMessagesDispatchFormState } from "../groupsMessagesDispatchResolver";
import type { MessagesDispatchMappingRow } from "../groupsMessagesDispatchResolver";
import type { ModelsListState } from "../groupsModelsListResolver";
import { createModelsListState } from "../groupsModelsListResolver";
import type { ReasoningEffortMappingRow } from "../groupsReasoningEffort";
import type { GroupEditorRoutingRule } from "../groupEditorContext";
import type { GroupPricingFormEntry } from "../groupsModelPricing";
import {
  getDefaultImagePreviewPrice,
  getDefaultVideoPreviewPrice,
} from "../groupsImagePricingResolver";

const baseGroupFormState = () => {
  const messagesDefaults = createDefaultMessagesDispatchFormState();
  return {
    name: "",
    description: "",
    platform: "anthropic" as GroupPlatform,
    rate_multiplier: 1.0,
    is_exclusive: false,
    subscription_type: "standard" as SubscriptionType,
    daily_limit_usd: null as number | null,
    weekly_limit_usd: null as number | null,
    monthly_limit_usd: null as number | null,
    long_context_pricing_enabled: true,
    model_pricing: [] as GroupPricingFormEntry[],
    allow_image_generation: false,
    openai_force_image_tool: false,
    allow_batch_image_generation: false,
    image_rate_independent: false,
    image_rate_multiplier: 1,
    batch_image_discount_multiplier: 0.5,
    batch_image_hold_multiplier: 0.6,
    image_price_1k: null as number | null,
    image_price_2k: null as number | null,
    image_price_4k: null as number | null,
    video_rate_independent: false,
    video_rate_multiplier: 1,
    video_price_480p: null as number | null,
    video_price_720p: null as number | null,
    video_price_1080p: null as number | null,
    web_search_price_per_call: null as number | null,
    peak_rate_enabled: false,
    peak_start: "",
    peak_end: "",
    peak_rate_multiplier: 1.0,
    profit_control_enabled: false,
    profit_min_margin_percent: 0,
    profit_safety_buffer_percent: 0,
    claude_code_only: false,
    fallback_group_id: null as number | null,
    fallback_group_id_on_invalid_request: null as number | null,
    allow_messages_dispatch: false,
    allow_live: false,
    opus_mapped_model: messagesDefaults.opus_mapped_model,
    sonnet_mapped_model: messagesDefaults.sonnet_mapped_model,
    haiku_mapped_model: messagesDefaults.haiku_mapped_model,
    exact_model_mappings: [] as MessagesDispatchMappingRow[],
    require_oauth_only: false,
    require_privacy_set: false,
    model_routing_enabled: false,
    supported_model_scopes: [
      "claude",
      "gemini_text",
      "gemini_image",
    ] as string[],
    mcp_xml_inject: true,
    copy_accounts_from_group_ids: [] as number[],
    rpm_limit: 0,
    max_reasoning_effort: "",
    reasoning_effort_mappings: [] as ReasoningEffortMappingRow[],
  };
};

export const createCreateGroupFormState = () => baseGroupFormState();

export const createEditGroupFormState = () => ({
  ...baseGroupFormState(),
  status: "active" as "active" | "inactive",
  default_mapped_model: "",
});

export const resetModelsListState = (
  state: ModelsListState,
  config?: Parameters<typeof createModelsListState>[0],
) => {
  const fresh = createModelsListState(config);
  state.enabled = fresh.enabled;
  state.savedModels = fresh.savedModels;
  state.items = fresh.items;
};

export const convertRoutingRulesToApiFormat = (
  rules: GroupEditorRoutingRule[],
): Record<string, number[]> | null => {
  const result: Record<string, number[]> = {};
  let hasValidRules = false;
  for (const rule of rules) {
    const pattern = rule.pattern.trim();
    if (!pattern) continue;
    const accountIDs = rule.accounts.map((account) => account.id).filter((id) => id > 0);
    if (accountIDs.length > 0) {
      result[pattern] = accountIDs;
      hasValidRules = true;
    }
  }
  return hasValidRules ? result : null;
};

export const normalizeOptionalLimit = (
  value: number | string | null | undefined,
): number | null => {
  if (value === null || value === undefined) return null;
  if (typeof value === "string") {
    const trimmed = value.trim();
    if (!trimmed) return null;
    const parsed = Number(trimmed);
    return Number.isFinite(parsed) && parsed > 0 ? parsed : null;
  }
  return Number.isFinite(value) && value > 0 ? value : null;
};

export const normalizeRateMultiplier = (
  value: number | string | null | undefined,
): number => {
  if (value === null || value === undefined || value === "") return 1;
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : 1;
};

type ImagePricingFormState = {
  platform: GroupPlatform;
  allow_image_generation: boolean;
  allow_batch_image_generation: boolean;
  rate_multiplier: number | string;
  image_rate_independent: boolean;
  image_rate_multiplier: number | string;
  batch_image_discount_multiplier: number | string;
  batch_image_hold_multiplier: number | string;
  image_price_1k: number | string | null;
  image_price_2k: number | string | null;
  image_price_4k: number | string | null;
};

type VideoPricingFormState = {
  platform: GroupPlatform;
  rate_multiplier: number | string;
  video_rate_independent: boolean;
  video_rate_multiplier: number | string;
  video_price_480p: number | string | null;
  video_price_720p: number | string | null;
  video_price_1080p: number | string | null;
};

const imagePricingTiers = [
  { key: "image_price_1k", label: "1K" },
  { key: "image_price_2k", label: "2K" },
  { key: "image_price_4k", label: "4K" },
] as const;

const videoPricingTiers = [
  { key: "video_price_480p", label: "480p" },
  { key: "video_price_720p", label: "720p" },
  { key: "video_price_1080p", label: "1080p" },
] as const;

const normalizePreviewNumber = (
  value: number | string | null | undefined,
  fallback = 0,
) => {
  if (value === null || value === undefined || value === "") return fallback;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : fallback;
};

const parsePreviewPrice = (value: number | string | null | undefined) => {
  if (value === null || value === undefined || value === "") return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : null;
};

const formatPricePreview = (
  value: number | string | null | undefined,
  notConfigured: string,
) => {
  if (value === null || value === undefined || value === "") {
    return notConfigured;
  }
  const price = Number(value);
  if (!Number.isFinite(price) || price < 0) return notConfigured;
  return `$${price.toFixed(6).replace(/0+$/, "").replace(/\.$/, "")}`;
};

export const buildImageFinalPricePreview = (
  form: ImagePricingFormState,
  notConfigured: string,
) => {
  const multiplier = form.image_rate_independent
    ? normalizePreviewNumber(form.image_rate_multiplier, 1)
    : normalizePreviewNumber(form.rate_multiplier, 1);
  return imagePricingTiers.map((tier) => {
    const basePrice =
      parsePreviewPrice(form[tier.key]) ??
      getDefaultImagePreviewPrice(form.platform, tier.key);
    return {
      label: tier.label,
      value:
        basePrice !== null
          ? formatPricePreview(basePrice * multiplier, notConfigured)
          : notConfigured,
    };
  });
};

export const buildVideoFinalPricePreview = (
  form: VideoPricingFormState,
  notConfigured: string,
) => {
  const multiplier = form.video_rate_independent
    ? normalizePreviewNumber(form.video_rate_multiplier, 1)
    : normalizePreviewNumber(form.rate_multiplier, 1);
  return videoPricingTiers.map((tier) => {
    const basePrice =
      parsePreviewPrice(form[tier.key]) ??
      getDefaultVideoPreviewPrice(form.platform, tier.key);
    return {
      label: tier.label,
      value:
        basePrice !== null
          ? formatPricePreview(basePrice * multiplier, notConfigured)
          : notConfigured,
    };
  });
};

const DEFAULT_WEB_SEARCH_PRICE_PER_CALL = 0.01;

export const buildWebSearchFinalPricePreview = (
  form: {
    web_search_price_per_call: number | string | null;
    rate_multiplier: number | string | null;
  },
  notConfigured: string,
) => {
  const basePrice =
    parsePreviewPrice(form.web_search_price_per_call) ??
    DEFAULT_WEB_SEARCH_PRICE_PER_CALL;
  const multiplier = normalizePreviewNumber(form.rate_multiplier, 1);
  return formatPricePreview(basePrice * multiplier, notConfigured);
};

export const resetDisabledBatchImagePricing = (
  form: Pick<
    ImagePricingFormState,
    | "platform"
    | "allow_image_generation"
    | "allow_batch_image_generation"
    | "batch_image_discount_multiplier"
    | "batch_image_hold_multiplier"
  >,
) => {
  if (form.platform !== "gemini" || !form.allow_image_generation) {
    form.allow_batch_image_generation = false;
  }
  if (!form.allow_batch_image_generation) {
    form.batch_image_discount_multiplier = 0.5;
    form.batch_image_hold_multiplier = 0.6;
  }
};
