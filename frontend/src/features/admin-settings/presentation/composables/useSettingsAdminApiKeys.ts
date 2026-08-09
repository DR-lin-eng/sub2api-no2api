import { reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  type AdminApiKey,
  type AdminApiKeyScope,
} from '@/features/admin-settings/data/dtos/adminSettingsDtos'
import {
  getAdminApiKey,
  listAdminApiKeys,
} from '@/features/admin-settings/data/datasources/adminSettingsQueries'
import {
  createAdminApiKey as createAdminApiKeyAction,
  deleteAdminApiKey as deleteAdminApiKeyAction,
  regenerateAdminApiKey as regenerateAdminApiKeyAction,
  revokeAdminApiKey,
  rotateAdminApiKey,
  updateAdminApiKey,
} from '@/features/admin-settings/data/datasources/adminSettingsActions'
import { extractApiErrorMessage } from '@/core/utils/apiError'
import { useAppStore } from '@/core/stores/appStore'

export function useSettingsAdminApiKeys(
  copyToClipboard: (text: string, successMessage?: string) => Promise<boolean>,
) {
  const { t } = useI18n()
  const appStore = useAppStore()

  // Admin API Key 状态
  const adminApiKeyLoading = ref(true);
  const adminApiKeyExists = ref(false);
  const adminApiKeyMasked = ref("");
  const adminApiKeyOperating = ref(false);
  const newAdminApiKey = ref("");
  const scopedAdminApiKeys = ref<AdminApiKey[]>([]);
  const adminApiKeyPanelLoading = ref(true);
  const adminApiKeyPanelOperating = ref(false);
  const adminApiKeyPanelSecret = ref("");
  const editingAdminApiKeyId = ref<string | null>(null);
  const adminApiKeyMinExpiry = new Date(Date.now() + 60_000).toISOString().slice(0, 16);
  const adminApiKeyForm = reactive<{ name: string; scopes: AdminApiKeyScope[]; expires_at: string }>({
    name: "",
    scopes: ["admin.read"],
    expires_at: "",
  });
  const adminApiKeyScopeOptions: Array<{ value: AdminApiKeyScope; label: string }> = [
    { value: "admin.read", label: "全部只读" },
    { value: "admin.write", label: "全部写入" },
    { value: "admin.users.read", label: "用户读取" },
    { value: "admin.users.write", label: "用户修改" },
    { value: "admin.accounts.read", label: "账号读取" },
    { value: "admin.accounts.write", label: "账号修改" },
    { value: "admin.settings.read", label: "设置读取" },
    { value: "admin.settings.write", label: "设置修改" },
    { value: "admin.backups.read", label: "备份读取" },
    { value: "admin.backups.write", label: "备份操作" },
    { value: "admin.system.read", label: "系统读取" },
    { value: "admin.system.write", label: "系统操作" },
    { value: "admin.audit.read", label: "审计读取" },
    { value: "admin.audit.write", label: "审计操作" },
    { value: "admin.ops.read", label: "运维读取" },
    { value: "admin.ops.write", label: "运维操作" },
  ];

  // Admin API Key 方法
  async function loadScopedAdminApiKeys() {
    adminApiKeyPanelLoading.value = true;
    try {
      scopedAdminApiKeys.value = (await listAdminApiKeys()).items;
    } catch {
      // Keep the legacy card usable when the scoped-key endpoint is unavailable.
    } finally {
      adminApiKeyPanelLoading.value = false;
    }
  }

  function formatAdminApiKeyDate(value: string): string {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
  }

  function adminApiKeyExpiryPayload(): string | null {
    if (!adminApiKeyForm.expires_at) return null;
    const date = new Date(adminApiKeyForm.expires_at);
    return Number.isNaN(date.getTime()) ? null : date.toISOString();
  }

  async function createScopedAdminApiKey() {
    adminApiKeyPanelOperating.value = true;
    try {
      const request = {
        name: adminApiKeyForm.name.trim(),
        scopes: adminApiKeyForm.scopes.length ? adminApiKeyForm.scopes : (["admin.read"] as AdminApiKeyScope[]),
        expires_at: adminApiKeyExpiryPayload(),
      };
      if (editingAdminApiKeyId.value) {
        await updateAdminApiKey(editingAdminApiKeyId.value, request);
        editingAdminApiKeyId.value = null;
      } else {
        const result = await createAdminApiKeyAction(request);
        adminApiKeyPanelSecret.value = result.key;
      }
      adminApiKeyForm.name = "";
      adminApiKeyForm.scopes = ["admin.read"];
      adminApiKeyForm.expires_at = "";
      await loadScopedAdminApiKeys();
      appStore.showSuccess(t("admin.settings.adminApiKey.keyGenerated"));
    } catch (error: unknown) {
      appStore.showError(extractApiErrorMessage(error, t("common.error")));
    } finally {
      adminApiKeyPanelOperating.value = false;
    }
  }

  function editScopedAdminApiKey(key: AdminApiKey) {
    editingAdminApiKeyId.value = key.id;
    adminApiKeyForm.name = key.name;
    adminApiKeyForm.scopes = [...key.scopes];
    adminApiKeyForm.expires_at = key.expires_at ? new Date(key.expires_at).toISOString().slice(0, 16) : "";
    adminApiKeyPanelSecret.value = "";
  }

  function cancelEditScopedAdminApiKey() {
    editingAdminApiKeyId.value = null;
    adminApiKeyForm.name = "";
    adminApiKeyForm.scopes = ["admin.read"];
    adminApiKeyForm.expires_at = "";
  }

  async function rotateScopedAdminApiKey(id: string) {
    if (!confirm(t("admin.settings.adminApiKey.regenerateConfirm"))) return;
    adminApiKeyPanelOperating.value = true;
    try {
      const result = await rotateAdminApiKey(id);
      adminApiKeyPanelSecret.value = result.key;
      await loadScopedAdminApiKeys();
    } catch (error: unknown) {
      appStore.showError(extractApiErrorMessage(error, t("common.error")));
    } finally {
      adminApiKeyPanelOperating.value = false;
    }
  }

  async function revokeScopedAdminApiKey(id: string) {
    if (!confirm(t("admin.settings.adminApiKey.deleteConfirm"))) return;
    adminApiKeyPanelOperating.value = true;
    try {
      await revokeAdminApiKey(id);
      await loadScopedAdminApiKeys();
      appStore.showSuccess(t("admin.settings.adminApiKey.keyDeleted"));
    } catch (error: unknown) {
      appStore.showError(extractApiErrorMessage(error, t("common.error")));
    } finally {
      adminApiKeyPanelOperating.value = false;
    }
  }

  function copyScopedAdminApiKey() {
    if (!adminApiKeyPanelSecret.value) return;
    copyToClipboard(adminApiKeyPanelSecret.value);
    appStore.showSuccess(t("admin.settings.adminApiKey.keyCopied"));
  }

  async function loadAdminApiKey() {
    adminApiKeyLoading.value = true;
    try {
      const status = await getAdminApiKey();
      adminApiKeyExists.value = status.exists;
      adminApiKeyMasked.value = status.masked_key;
    } catch {
      // Silent fail - admin API key status is non-critical
    } finally {
      adminApiKeyLoading.value = false;
    }
  }

  async function createAdminApiKey() {
    adminApiKeyOperating.value = true;
    try {
      const result = await regenerateAdminApiKeyAction();
      newAdminApiKey.value = result.key;
      adminApiKeyExists.value = true;
      adminApiKeyMasked.value =
        result.key.substring(0, 10) + "..." + result.key.slice(-4);
      appStore.showSuccess(t("admin.settings.adminApiKey.keyGenerated"));
    } catch (error: unknown) {
      appStore.showError(extractApiErrorMessage(error, t("common.error")));
    } finally {
      adminApiKeyOperating.value = false;
    }
  }

  async function regenerateAdminApiKey() {
    if (!confirm(t("admin.settings.adminApiKey.regenerateConfirm"))) return;
    await createAdminApiKey();
  }

  async function deleteAdminApiKey() {
    if (!confirm(t("admin.settings.adminApiKey.deleteConfirm"))) return;
    adminApiKeyOperating.value = true;
    try {
      await deleteAdminApiKeyAction();
      adminApiKeyExists.value = false;
      adminApiKeyMasked.value = "";
      newAdminApiKey.value = "";
      appStore.showSuccess(t("admin.settings.adminApiKey.keyDeleted"));
    } catch (error: unknown) {
      appStore.showError(extractApiErrorMessage(error, t("common.error")));
    } finally {
      adminApiKeyOperating.value = false;
    }
  }

  function copyNewKey() {
    navigator.clipboard
      .writeText(newAdminApiKey.value)
      .then(() => {
        appStore.showSuccess(t("admin.settings.adminApiKey.keyCopied"));
      })
      .catch(() => {
        appStore.showError(t("common.copyFailed"));
      });
  }


  return {
    adminApiKeyExists,
    adminApiKeyForm,
    adminApiKeyLoading,
    adminApiKeyMasked,
    adminApiKeyMinExpiry,
    adminApiKeyOperating,
    adminApiKeyPanelLoading,
    adminApiKeyPanelOperating,
    adminApiKeyPanelSecret,
    adminApiKeyScopeOptions,
    cancelEditScopedAdminApiKey,
    copyNewKey,
    copyScopedAdminApiKey,
    createAdminApiKey,
    createScopedAdminApiKey,
    deleteAdminApiKey,
    editScopedAdminApiKey,
    editingAdminApiKeyId,
    formatAdminApiKeyDate,
    loadAdminApiKey,
    loadScopedAdminApiKeys,
    newAdminApiKey,
    regenerateAdminApiKey,
    revokeScopedAdminApiKey,
    rotateScopedAdminApiKey,
    scopedAdminApiKeys,
  }
}
