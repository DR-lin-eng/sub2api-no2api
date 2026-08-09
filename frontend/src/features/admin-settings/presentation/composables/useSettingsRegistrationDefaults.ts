import { computed, reactive, ref } from "vue";
import {
  buildAuthSourceDefaultsState,
  type AuthSourceDefaultsState,
  type AuthSourceType,
  type DefaultSubscriptionSetting,
} from "@/features/admin-settings/data/dtos/systemSettingsDtos";
import { getAll as getAllAdminGroups } from "@/features/admin-groups/data/datasources/adminGroupsDatasource";
import {
  isRegistrationEmailSuffixDomainValid,
  normalizeRegistrationEmailSuffixDomain,
  parseRegistrationEmailSuffixWhitelistInput,
} from "@/core/utils/registrationEmailPolicy";
import type { AdminGroup } from "@/types";
import type { SettingsForm } from "./settingsForm";

type Translate = (key: string) => string;
type LocalText = (zh: string, en: string) => string;

interface DefaultSubscriptionGroupOption {
  value: number;
  label: string;
  description: string | null;
  platform: AdminGroup["platform"];
  subscriptionType: AdminGroup["subscription_type"];
  rate: number;
  [key: string]: unknown;
}

export function useSettingsRegistrationDefaults(
  form: SettingsForm,
  t: Translate,
  localText: LocalText,
) {
  const subscriptionGroups = ref<AdminGroup[]>([]);
  const registrationEmailSuffixWhitelistTags = ref<string[]>([]);
  const registrationEmailSuffixWhitelistDraft = ref("");
  const authSourceDefaults = reactive<AuthSourceDefaultsState>(
    buildAuthSourceDefaultsState({}),
  );

  const authSourceDefaultsMeta = computed(() => [
    {
      source: "email" as AuthSourceType,
      title: t("admin.settings.authSourceDefaults.sources.email.title"),
      description: t(
        "admin.settings.authSourceDefaults.sources.email.description",
      ),
    },
    {
      source: "linuxdo" as AuthSourceType,
      title: t("admin.settings.authSourceDefaults.sources.linuxdo.title"),
      description: t(
        "admin.settings.authSourceDefaults.sources.linuxdo.description",
      ),
    },
    {
      source: "oidc" as AuthSourceType,
      title: t("admin.settings.authSourceDefaults.sources.oidc.title"),
      description: t(
        "admin.settings.authSourceDefaults.sources.oidc.description",
      ),
    },
    {
      source: "wechat" as AuthSourceType,
      title: t("admin.settings.authSourceDefaults.sources.wechat.title"),
      description: t(
        "admin.settings.authSourceDefaults.sources.wechat.description",
      ),
    },
    {
      source: "github" as AuthSourceType,
      title: "GitHub",
      description: localText(
        "通过 GitHub 已验证邮箱首次注册或首次绑定时应用。",
        "Applied on first signup or first bind through a verified GitHub email.",
      ),
    },
    {
      source: "google" as AuthSourceType,
      title: "Google",
      description: localText(
        "通过 Google 已验证邮箱首次注册或首次绑定时应用。",
        "Applied on first signup or first bind through a verified Google email.",
      ),
    },
    {
      source: "dingtalk" as AuthSourceType,
      title: t("auth.dingtalkProviderName"),
      description: localText(
        "通过钉钉首次注册或首次绑定时应用。",
        "Applied on first signup or first bind through DingTalk.",
      ),
    },
  ]);

  const defaultSubscriptionGroupOptions = computed<
    DefaultSubscriptionGroupOption[]
  >(() =>
    subscriptionGroups.value.map((group) => ({
      value: group.id,
      label: group.name,
      description: group.description,
      platform: group.platform,
      subscriptionType: group.subscription_type,
      rate: group.rate_multiplier,
    })),
  );

  const registrationEmailSuffixWhitelistSeparatorKeys = new Set([
    " ",
    ",",
    "，",
    "Enter",
    "Tab",
  ]);

  function removeRegistrationEmailSuffixWhitelistTag(suffix: string) {
    registrationEmailSuffixWhitelistTags.value =
      registrationEmailSuffixWhitelistTags.value.filter(
        (item) => item !== suffix,
      );
  }

  function addRegistrationEmailSuffixWhitelistTag(raw: string) {
    const suffix = normalizeRegistrationEmailSuffixDomain(raw);
    if (
      !isRegistrationEmailSuffixDomainValid(suffix) ||
      registrationEmailSuffixWhitelistTags.value.includes(suffix)
    ) {
      return;
    }
    registrationEmailSuffixWhitelistTags.value = [
      ...registrationEmailSuffixWhitelistTags.value,
      suffix,
    ];
  }

  function commitRegistrationEmailSuffixWhitelistDraft() {
    if (!registrationEmailSuffixWhitelistDraft.value) return;
    addRegistrationEmailSuffixWhitelistTag(
      registrationEmailSuffixWhitelistDraft.value,
    );
    registrationEmailSuffixWhitelistDraft.value = "";
  }

  function handleRegistrationEmailSuffixWhitelistDraftInput() {
    registrationEmailSuffixWhitelistDraft.value =
      normalizeRegistrationEmailSuffixDomain(
        registrationEmailSuffixWhitelistDraft.value,
      );
  }

  function handleRegistrationEmailSuffixWhitelistDraftKeydown(
    event: KeyboardEvent,
  ) {
    if (event.isComposing) return;

    if (registrationEmailSuffixWhitelistSeparatorKeys.has(event.key)) {
      event.preventDefault();
      commitRegistrationEmailSuffixWhitelistDraft();
      return;
    }

    if (
      event.key === "Backspace" &&
      !registrationEmailSuffixWhitelistDraft.value &&
      registrationEmailSuffixWhitelistTags.value.length > 0
    ) {
      registrationEmailSuffixWhitelistTags.value.pop();
    }
  }

  function handleRegistrationEmailSuffixWhitelistPaste(event: ClipboardEvent) {
    const text = event.clipboardData?.getData("text") || "";
    if (!text.trim()) return;
    event.preventDefault();
    for (const token of parseRegistrationEmailSuffixWhitelistInput(text)) {
      addRegistrationEmailSuffixWhitelistTag(token);
    }
  }

  function addQuotaNotifyEmail() {
    if (!form.account_quota_notify_emails) {
      form.account_quota_notify_emails = [];
    }
    form.account_quota_notify_emails.push({
      email: "",
      disabled: false,
      verified: true,
    });
  }

  async function loadSubscriptionGroups() {
    try {
      const groups = await getAllAdminGroups();
      subscriptionGroups.value = groups.filter(
        (group) =>
          group.subscription_type === "subscription" &&
          group.status === "active",
      );
    } catch {
      subscriptionGroups.value = [];
    }
  }

  function findNextAvailableSubscriptionGroup(
    existingGroupIDs: number[],
  ): AdminGroup | undefined {
    const existing = new Set(existingGroupIDs);
    return subscriptionGroups.value.find((group) => !existing.has(group.id));
  }

  function addDefaultSubscription() {
    if (subscriptionGroups.value.length === 0) return;
    const candidate = findNextAvailableSubscriptionGroup(
      form.default_subscriptions.map((item) => item.group_id),
    );
    if (!candidate) return;
    form.default_subscriptions.push({
      group_id: candidate.id,
      validity_days: 30,
    });
  }

  function removeDefaultSubscription(index: number) {
    form.default_subscriptions.splice(index, 1);
  }

  function addAuthSourceDefaultSubscription(source: AuthSourceType) {
    if (subscriptionGroups.value.length === 0) return;
    const candidate = findNextAvailableSubscriptionGroup(
      authSourceDefaults[source].subscriptions.map((item) => item.group_id),
    );
    if (!candidate) return;
    authSourceDefaults[source].subscriptions.push({
      group_id: candidate.id,
      validity_days: 30,
    });
  }

  function removeAuthSourceDefaultSubscription(
    source: AuthSourceType,
    index: number,
  ) {
    authSourceDefaults[source].subscriptions.splice(index, 1);
  }

  function findDuplicateDefaultSubscription(
    subscriptions: DefaultSubscriptionSetting[],
  ): DefaultSubscriptionSetting | undefined {
    const seenGroupIDs = new Set<number>();
    return subscriptions.find((item) => {
      if (seenGroupIDs.has(item.group_id)) return true;
      seenGroupIDs.add(item.group_id);
      return false;
    });
  }

  return {
    addAuthSourceDefaultSubscription,
    addDefaultSubscription,
    addQuotaNotifyEmail,
    authSourceDefaults,
    authSourceDefaultsMeta,
    commitRegistrationEmailSuffixWhitelistDraft,
    defaultSubscriptionGroupOptions,
    findDuplicateDefaultSubscription,
    handleRegistrationEmailSuffixWhitelistDraftInput,
    handleRegistrationEmailSuffixWhitelistDraftKeydown,
    handleRegistrationEmailSuffixWhitelistPaste,
    loadSubscriptionGroups,
    registrationEmailSuffixWhitelistDraft,
    registrationEmailSuffixWhitelistTags,
    removeAuthSourceDefaultSubscription,
    removeDefaultSubscription,
    removeRegistrationEmailSuffixWhitelistTag,
    subscriptionGroups,
  };
}
