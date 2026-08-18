import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  createProvider,
  deleteProvider,
  updateProvider,
} from '@/features/admin-orders/data/datasources/adminPaymentActions'
import { getProviders } from '@/features/admin-orders/data/datasources/adminPaymentQueries'
import type { ProviderInstance } from '@/features/billing/paymentContracts'
import type PaymentProviderDialog from '@/features/billing/paymentProviderDialog'
import { extractI18nErrorMessage } from '@/core/utils/apiError'
import { useAppStore } from '@/core/stores/appStore'
import { normalizeVisibleMethod } from '@/features/billing/paymentMethods'

interface PaymentSettingsForm {
  payment_enabled_types: string[]
}
export function useSettingsPaymentProviders(
  form: PaymentSettingsForm,
  saveSettings: () => Promise<void>,
) {
  const { t } = useI18n()
  const appStore = useAppStore()

  // ==================== Provider Management ====================

  const allPaymentTypes = computed(() => [
    { value: "easypay", label: t("payment.methods.easypay") },
    { value: "alipay", label: t("payment.methods.alipay") },
    { value: "wxpay", label: t("payment.methods.wxpay") },
    { value: "stripe", label: t("payment.methods.stripe") },
    { value: "airwallex", label: t("payment.methods.airwallex") },
  ]);

  function isPaymentTypeEnabled(type: string): boolean {
    return form.payment_enabled_types.includes(type);
  }

  const hasAnyPaymentTypeEnabled = computed(
    () => form.payment_enabled_types.length > 0,
  );

  function togglePaymentType(type: string) {
    if (form.payment_enabled_types.includes(type)) {
      form.payment_enabled_types = form.payment_enabled_types.filter(
        (t) => t !== type,
      );
      // Disable all provider instances matching this type
      disableProvidersByType(type);
    } else {
      form.payment_enabled_types = [...form.payment_enabled_types, type];
    }
  }

  async function disableProvidersByType(type: string) {
    const matching = providers.value.filter(
      (p) => p.provider_key === type && p.enabled,
    );
    for (const p of matching) {
      try {
        await updateProvider(p.id, { enabled: false });
        p.enabled = false;
      } catch (err: unknown) {
        slog("disable provider failed", p.id, err);
      }
    }
  }

  function slog(...args: unknown[]) {
    console.warn("[payment]", ...args);
  }

  const providersLoading = ref(false);
  const providerSaving = ref(false);
  const providers = ref<ProviderInstance[]>([]);
  const showProviderDialog = ref(false);
  const showDeleteProviderDialog = ref(false);
  const editingProvider = ref<ProviderInstance | null>(null);
  const deletingProviderId = ref<number | null>(null);
  const providerDialogRef = ref<InstanceType<
    typeof PaymentProviderDialog
  > | null>(null);

  const providerKeyOptions = computed(() => [
    { value: "easypay", label: t("admin.settings.payment.providerEasypay") },
    { value: "alipay", label: t("admin.settings.payment.providerAlipay") },
    { value: "wxpay", label: t("admin.settings.payment.providerWxpay") },
    { value: "stripe", label: t("admin.settings.payment.providerStripe") },
    { value: "airwallex", label: t("admin.settings.payment.providerAirwallex") },
  ]);

  const enabledProviderKeyOptions = computed(() => {
    const enabled = form.payment_enabled_types;
    return providerKeyOptions.value.filter((opt) => enabled.includes(opt.value));
  });

  const loadBalanceOptions = computed(() => [
    {
      value: "round-robin",
      label: t("admin.settings.payment.strategyRoundRobin"),
    },
    {
      value: "least-amount",
      label: t("admin.settings.payment.strategyLeastAmount"),
    },
  ]);

  const cancelRateLimitUnitOptions = computed(() => [
    {
      value: "minute",
      label: t("admin.settings.payment.cancelRateLimitUnitMinute"),
    },
    { value: "hour", label: t("admin.settings.payment.cancelRateLimitUnitHour") },
    { value: "day", label: t("admin.settings.payment.cancelRateLimitUnitDay") },
  ]);

  const cancelRateLimitModeOptions = computed(() => [
    {
      value: "rolling",
      label: t("admin.settings.payment.cancelRateLimitWindowModeRolling"),
    },
    {
      value: "fixed",
      label: t("admin.settings.payment.cancelRateLimitWindowModeFixed"),
    },
  ]);

  type ProviderEnablementCandidate = Pick<
    ProviderInstance,
    "id" | "provider_key" | "supported_types" | "enabled" | "name"
  >;

  function getProviderVisibleMethods(
    provider: ProviderEnablementCandidate,
  ): Array<"alipay" | "wxpay"> {
    if (!provider.enabled) {
      return [];
    }

    const supportedTypes = Array.isArray(provider.supported_types)
      ? provider.supported_types
      : [];
    const methods = new Set<"alipay" | "wxpay">();
    const addMethod = (type: string) => {
      const method = normalizeVisibleMethod(type);
      if (method === "alipay" || method === "wxpay") {
        methods.add(method);
      }
    };

    if (provider.provider_key === "alipay") {
      if (supportedTypes.length === 0) {
        methods.add("alipay");
      } else {
        supportedTypes.forEach((type) => {
          if (normalizeVisibleMethod(type) === "alipay") {
            methods.add("alipay");
          }
        });
      }
    } else if (provider.provider_key === "wxpay") {
      if (supportedTypes.length === 0) {
        methods.add("wxpay");
      } else {
        supportedTypes.forEach((type) => {
          if (normalizeVisibleMethod(type) === "wxpay") {
            methods.add("wxpay");
          }
        });
      }
    } else if (provider.provider_key === "easypay") {
      supportedTypes.forEach(addMethod);
    }

    return Array.from(methods);
  }

  function findProviderEnablementConflict(
    candidate: ProviderEnablementCandidate,
  ): { method: "alipay" | "wxpay"; conflicting: ProviderInstance } | null {
    const claimedMethods = getProviderVisibleMethods(candidate);
    if (claimedMethods.length === 0) {
      return null;
    }

    for (const other of providers.value) {
      if (other.id === candidate.id || !other.enabled) {
        continue;
      }

      const otherMethods = getProviderVisibleMethods(other);
      const matchedMethod = claimedMethods.find((method) =>
        otherMethods.includes(method),
      );
      if (matchedMethod) {
        return {
          method: matchedMethod,
          conflicting: other,
        };
      }
    }

    return null;
  }

  function showProviderEnablementConflict(
    conflict: { method: "alipay" | "wxpay"; conflicting: ProviderInstance },
  ) {
    appStore.showError(
      t("admin.settings.payment.enableConflict", {
        method: t(`payment.methods.${conflict.method}`),
        provider: conflict.conflicting.name,
      }),
    );
  }

  async function loadProviders() {
    providersLoading.value = true;
    try {
      const res = await getProviders();
      // Normalize supported_types: backend returns null when the list is empty
      // (Go nil slice → JSON null). Without this, ProviderCard's isSelected()
      // throws TypeError on null.includes(), causing the card to vanish.
      providers.value = (res.data || []).map((p) => ({
        ...p,
        supported_types: Array.isArray(p.supported_types)
          ? p.supported_types
          : [],
      }));
    } catch (err: unknown) {
      appStore.showError(extractI18nErrorMessage(err, t, "payment.errors", t("common.error")));
    } finally {
      providersLoading.value = false;
    }
  }

  function openCreateProvider() {
    editingProvider.value = null;
    providerDialogRef.value?.reset(
      enabledProviderKeyOptions.value[0]?.value || "easypay",
    );
    showProviderDialog.value = true;
  }

  function openEditProvider(provider: ProviderInstance) {
    editingProvider.value = provider;
    providerDialogRef.value?.loadProvider(provider);
    showProviderDialog.value = true;
  }

  async function handleSaveProvider(payload: Partial<ProviderInstance>) {
    providerSaving.value = true;
    try {
      const candidate: ProviderEnablementCandidate = {
        id: editingProvider.value?.id ?? 0,
        provider_key:
          payload.provider_key ?? editingProvider.value?.provider_key ?? "",
        supported_types:
          payload.supported_types ?? editingProvider.value?.supported_types ?? [],
        enabled: payload.enabled ?? editingProvider.value?.enabled ?? false,
        name: payload.name ?? editingProvider.value?.name ?? "",
      };
      const conflict = findProviderEnablementConflict(candidate);
      if (conflict) {
        showProviderEnablementConflict(conflict);
        return;
      }

      if (editingProvider.value) {
        await updateProvider(editingProvider.value.id, payload);
      } else {
        await createProvider(payload);
      }
      showProviderDialog.value = false;
      // Reload full list (API returns decrypted/formatted data with correct sort order)
      await loadProviders();
      // Auto-save settings so provider changes take effect immediately
      await saveSettings();
    } catch (err: unknown) {
      appStore.showError(extractI18nErrorMessage(err, t, "payment.errors", t("common.error")));
    } finally {
      providerSaving.value = false;
    }
  }

  async function handleToggleField(
    provider: ProviderInstance,
    field: "enabled" | "refund_enabled" | "allow_user_refund",
  ) {
    let newValue: boolean;
    if (field === "enabled") newValue = !provider.enabled;
    else if (field === "refund_enabled") newValue = !provider.refund_enabled;
    else newValue = !provider.allow_user_refund;

    if (field === "enabled" && newValue) {
      const conflict = findProviderEnablementConflict({
        id: provider.id,
        provider_key: provider.provider_key,
        supported_types: provider.supported_types,
        enabled: true,
        name: provider.name,
      });
      if (conflict) {
        showProviderEnablementConflict(conflict);
        return;
      }
    }

    const payload: Record<string, boolean> = { [field]: newValue };
    // Cascade: turning off refund_enabled also turns off allow_user_refund
    if (field === "refund_enabled" && !newValue) {
      payload.allow_user_refund = false;
    }
    try {
      await updateProvider(provider.id, payload);
      await loadProviders();
    } catch (err: unknown) {
      appStore.showError(extractI18nErrorMessage(err, t, "payment.errors", t("common.error")));
    }
  }

  async function handleToggleType(provider: ProviderInstance, type: string) {
    const currentTypes = Array.isArray(provider.supported_types)
      ? provider.supported_types
      : [];
    const updated = currentTypes.includes(type)
      ? currentTypes.filter((t) => t !== type)
      : [...currentTypes, type];
    const conflict = findProviderEnablementConflict({
      id: provider.id,
      provider_key: provider.provider_key,
      supported_types: updated,
      enabled: provider.enabled,
      name: provider.name,
    });
    if (conflict) {
      showProviderEnablementConflict(conflict);
      return;
    }
    try {
      await updateProvider(provider.id, {
        supported_types: updated,
      } as any);
      await loadProviders();
    } catch (err: unknown) {
      appStore.showError(extractI18nErrorMessage(err, t, "payment.errors", t("common.error")));
    }
  }

  function confirmDeleteProvider(provider: ProviderInstance) {
    deletingProviderId.value = provider.id;
    showDeleteProviderDialog.value = true;
  }

  async function handleReorderProviders(
    updates: { id: number; sort_order: number }[],
  ) {
    try {
      await Promise.all(
        updates.map((u) =>
          updateProvider(u.id, {
            sort_order: u.sort_order,
          } as Partial<ProviderInstance>),
        ),
      );
      await loadProviders();
    } catch (err: unknown) {
      appStore.showError(extractI18nErrorMessage(err, t, "payment.errors", t("common.error")));
      loadProviders();
    }
  }

  async function handleDeleteProvider() {
    if (!deletingProviderId.value) return;
    try {
      await deleteProvider(deletingProviderId.value);
      appStore.showSuccess(t("common.deleted"));
      showDeleteProviderDialog.value = false;
      loadProviders();
    } catch (err: unknown) {
      appStore.showError(extractI18nErrorMessage(err, t, "payment.errors", t("common.error")));
    }
  }


  return {
    allPaymentTypes,
    cancelRateLimitModeOptions,
    cancelRateLimitUnitOptions,
    confirmDeleteProvider,
    editingProvider,
    enabledProviderKeyOptions,
    handleDeleteProvider,
    handleReorderProviders,
    handleSaveProvider,
    handleToggleField,
    handleToggleType,
    hasAnyPaymentTypeEnabled,
    isPaymentTypeEnabled,
    loadBalanceOptions,
    loadProviders,
    openCreateProvider,
    openEditProvider,
    providerDialogRef,
    providerKeyOptions,
    providerSaving,
    providers,
    providersLoading,
    showDeleteProviderDialog,
    showProviderDialog,
    togglePaymentType,
  }
}
