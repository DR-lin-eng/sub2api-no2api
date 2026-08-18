import type { ComputedRef, Ref } from "vue";
import type { AdminGroup } from "@/features/admin-groups/data/dtos/adminGroupDtos";
import type { GroupPlatform, SubscriptionType } from "@/types/group";
import type { ModelsListState } from "./groupsModelsListResolver";
import type { MessagesDispatchMappingRow } from "./groupsMessagesDispatchResolver";
import type { ReasoningEffortMappingRow } from "./groupsReasoningEffort";
import type { GroupPricingFormEntry } from "./groupsModelPricing";

export interface GroupEditorOption {
  value: string | number | boolean | null;
  label: string;
  [key: string]: unknown;
}

export interface GroupEditorIDOption {
  value: number;
  label: string;
  [key: string]: unknown;
}

export interface GroupEditorPreviewItem {
  label: string;
  value: string;
}

export interface GroupEditorSimpleAccount {
  id: number;
  name: string;
}

export interface GroupEditorRoutingRule {
  pattern: string;
  accounts: GroupEditorSimpleAccount[];
}

export interface GroupReasoningEffortFieldsExpose {
  validate: () => boolean;
  resetValidation: () => void;
}

export interface GroupEditorFormState {
  name: string;
  description: string;
  platform: GroupPlatform;
  rate_multiplier: number | string;
  is_exclusive: boolean;
  status?: "active" | "inactive";
  subscription_type: SubscriptionType;
  daily_limit_usd: number | string | null;
  weekly_limit_usd: number | string | null;
  monthly_limit_usd: number | string | null;
  long_context_pricing_enabled: boolean;
  model_pricing: GroupPricingFormEntry[];
  allow_image_generation: boolean;
  openai_force_image_tool: boolean;
  allow_batch_image_generation: boolean;
  image_rate_independent: boolean;
  image_rate_multiplier: number | string;
  batch_image_discount_multiplier: number | string;
  batch_image_hold_multiplier: number | string;
  image_price_1k: number | string | null;
  image_price_2k: number | string | null;
  image_price_4k: number | string | null;
  video_rate_independent: boolean;
  video_rate_multiplier: number | string;
  video_price_480p: number | string | null;
  video_price_720p: number | string | null;
  video_price_1080p: number | string | null;
  web_search_price_per_call: number | string | null;
  peak_rate_enabled: boolean;
  peak_start: string;
  peak_end: string;
  peak_rate_multiplier: number | string;
  profit_control_enabled: boolean;
  profit_min_margin_percent: number | string;
  profit_safety_buffer_percent: number | string;
  claude_code_only: boolean;
  fallback_group_id: number | null;
  fallback_group_id_on_invalid_request: number | null;
  allow_messages_dispatch: boolean;
  allow_live: boolean;
  default_mapped_model?: string;
  opus_mapped_model: string;
  sonnet_mapped_model: string;
  haiku_mapped_model: string;
  exact_model_mappings: MessagesDispatchMappingRow[];
  require_oauth_only: boolean;
  require_privacy_set: boolean;
  model_routing_enabled: boolean;
  supported_model_scopes: string[];
  mcp_xml_inject: boolean;
  copy_accounts_from_group_ids: number[];
  rpm_limit: number | string;
  max_reasoning_effort: string;
  reasoning_effort_mappings: ReasoningEffortMappingRow[];
}

export interface GroupEditorDialogContext {
  show: Ref<boolean>;
  form: GroupEditorFormState;
  close: () => void;
  submit: () => void | Promise<void>;
  submitting: Ref<boolean>;
  platformOptions: ComputedRef<GroupEditorOption[]>;
  subscriptionTypeOptions: ComputedRef<GroupEditorOption[]>;
  copyAccountsOptions: ComputedRef<GroupEditorIDOption[]>;
  fallbackOptions: ComputedRef<GroupEditorOption[]>;
  invalidRequestFallbackOptions: ComputedRef<GroupEditorOption[]>;
  imageFinalPricePreview: ComputedRef<GroupEditorPreviewItem[]>;
  videoFinalPricePreview: ComputedRef<GroupEditorPreviewItem[]>;
  webSearchFinalPricePreview: ComputedRef<string>;
  modelsListState: ModelsListState;
  modelsListLoading: Ref<boolean>;
  modelsListSelectedCount: ComputedRef<number>;
  moveModelsListItem: (fromIndex: number, toIndex: number) => void;
  modelRoutingRules: Ref<GroupEditorRoutingRule[]>;
  addRoutingRule: () => void;
  removeRoutingRule: (rule: GroupEditorRoutingRule) => void;
  getRuleRenderKey: (rule: GroupEditorRoutingRule) => string;
  getRuleSearchKey: (rule: GroupEditorRoutingRule) => string;
  accountSearchKeyword: Ref<Record<string, string>>;
  accountSearchResults: Ref<Record<string, GroupEditorSimpleAccount[]>>;
  showAccountDropdown: Ref<Record<string, boolean>>;
  searchAccountsByRule: (rule: GroupEditorRoutingRule) => void;
  selectAccount: (
    rule: GroupEditorRoutingRule,
    account: GroupEditorSimpleAccount,
  ) => void;
  removeSelectedAccount: (
    rule: GroupEditorRoutingRule,
    accountId: number,
  ) => void;
  onAccountSearchFocus: (rule: GroupEditorRoutingRule) => void;
  toggleScope: (scope: string) => void;
  toggleLive: () => void | Promise<void>;
  addMessagesDispatchMapping: () => void;
  removeMessagesDispatchMapping: (row: MessagesDispatchMappingRow) => void;
  getMessagesDispatchRowKey: (row: MessagesDispatchMappingRow) => string;
  reasoningEffortPolicyRef: Ref<GroupReasoningEffortFieldsExpose | null>;
  addModelPricing: () => void;
  removeModelPricing: (index: number) => void;
  updateModelPricing: (index: number, entry: GroupPricingFormEntry) => void;
  updateModelPricingModels: (index: number, models: string[]) => void | Promise<void>;
}

export interface EditGroupDialogContext extends GroupEditorDialogContext {
  editingGroup: Ref<AdminGroup | null>;
  statusOptions: ComputedRef<GroupEditorOption[]>;
}
