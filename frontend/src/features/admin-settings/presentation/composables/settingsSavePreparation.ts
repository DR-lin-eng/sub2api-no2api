import {
  deriveWeChatConnectStoredMode,
  normalizeDefaultSubscriptionSettings,
  type AuthSourceDefaultsState,
  type AuthSourceType,
  type DefaultSubscriptionSetting,
  type WeChatConnectMode,
} from "@/features/admin-settings/data/dtos/systemSettingsDtos";
import type { LoginAgreementDocument } from "@/types";
import {
  tablePageSizeMax,
  tablePageSizeMin,
  type SettingsForm,
} from "./settingsForm";

interface AuthSourceDefaultsMetaEntry {
  source: AuthSourceType;
  title: string;
}

interface SettingsSavePreparationContext {
  form: SettingsForm;
  tablePageSizeOptionsInput: string;
  authSourceDefaults: AuthSourceDefaultsState;
  authSourceDefaultsMeta: readonly AuthSourceDefaultsMetaEntry[];
  parseTablePageSizeOptionsInput: (raw: string) => number[] | null;
  normalizeLoginAgreementDocumentsForSave: () => LoginAgreementDocument[];
  findDuplicateLoginAgreementDocumentId: (
    documents: LoginAgreementDocument[],
  ) => string | null;
  findDuplicateDefaultSubscription: (
    subscriptions: DefaultSubscriptionSetting[],
  ) => DefaultSubscriptionSetting | undefined;
  syncWeChatConnectMode: () => void;
  serializeClaudeOAuthSystemPromptBlocks: () => string;
}

export type SettingsSaveValidationError =
  | { kind: "tableDefaultPageSize" }
  | { kind: "tablePageSizeOptions" }
  | { kind: "loginAgreementDocumentRequired" }
  | { kind: "loginAgreementDocumentTitleRequired" }
  | { kind: "duplicateLoginAgreementDocumentId"; documentId: string }
  | { kind: "duplicateDefaultSubscription"; groupId: number }
  | {
      kind: "duplicateAuthSourceDefaultSubscription";
      groupId: number;
      sourceTitle: string;
    }
  | { kind: "conflictingWeChatApplications" };

export type SettingsSavePreparationResult =
  | {
      ok: true;
      normalizedDefaultSubscriptions: DefaultSubscriptionSetting[];
      wechatStoredMode: WeChatConnectMode;
      claudeOAuthSystemPromptBlocksJSON: string;
    }
  | { ok: false; error: SettingsSaveValidationError };

function isValidHttpUrl(url: string): boolean {
  if (!url) return true;
  try {
    const parsed = new URL(url);
    return parsed.protocol === "http:" || parsed.protocol === "https:";
  } catch {
    return false;
  }
}

export function prepareSettingsSave({
  form,
  tablePageSizeOptionsInput,
  authSourceDefaults,
  authSourceDefaultsMeta,
  parseTablePageSizeOptionsInput,
  normalizeLoginAgreementDocumentsForSave,
  findDuplicateLoginAgreementDocumentId,
  findDuplicateDefaultSubscription,
  syncWeChatConnectMode,
  serializeClaudeOAuthSystemPromptBlocks,
}: SettingsSavePreparationContext): SettingsSavePreparationResult {
  const normalizedTableDefaultPageSize = Math.floor(
    Number(form.table_default_page_size),
  );
  if (
    !Number.isInteger(normalizedTableDefaultPageSize) ||
    normalizedTableDefaultPageSize < tablePageSizeMin ||
    normalizedTableDefaultPageSize > tablePageSizeMax
  ) {
    return { ok: false, error: { kind: "tableDefaultPageSize" } };
  }

  const normalizedTablePageSizeOptions =
    parseTablePageSizeOptionsInput(tablePageSizeOptionsInput);
  if (!normalizedTablePageSizeOptions) {
    return { ok: false, error: { kind: "tablePageSizeOptions" } };
  }

  form.table_default_page_size = normalizedTableDefaultPageSize;
  form.table_page_size_options = normalizedTablePageSizeOptions;

  const normalizedLoginAgreementDocuments =
    normalizeLoginAgreementDocumentsForSave();
  if (
    form.login_agreement_enabled &&
    normalizedLoginAgreementDocuments.length === 0
  ) {
    return {
      ok: false,
      error: { kind: "loginAgreementDocumentRequired" },
    };
  }
  if (normalizedLoginAgreementDocuments.some((document) => !document.title)) {
    return {
      ok: false,
      error: { kind: "loginAgreementDocumentTitleRequired" },
    };
  }
  const duplicateLoginAgreementDocumentId =
    findDuplicateLoginAgreementDocumentId(normalizedLoginAgreementDocuments);
  if (duplicateLoginAgreementDocumentId) {
    return {
      ok: false,
      error: {
        kind: "duplicateLoginAgreementDocumentId",
        documentId: duplicateLoginAgreementDocumentId,
      },
    };
  }

  form.login_agreement_mode =
    form.login_agreement_mode === "checkbox" ? "checkbox" : "modal";
  form.login_agreement_documents = normalizedLoginAgreementDocuments;

  const normalizedDefaultSubscriptions = normalizeDefaultSubscriptionSettings(
    form.default_subscriptions,
  );
  const duplicateDefaultSubscription = findDuplicateDefaultSubscription(
    normalizedDefaultSubscriptions,
  );
  if (duplicateDefaultSubscription) {
    return {
      ok: false,
      error: {
        kind: "duplicateDefaultSubscription",
        groupId: duplicateDefaultSubscription.group_id,
      },
    };
  }

  for (const authSource of authSourceDefaultsMeta) {
    authSourceDefaults[authSource.source].subscriptions =
      normalizeDefaultSubscriptionSettings(
        authSourceDefaults[authSource.source].subscriptions,
      );
    const duplicate = findDuplicateDefaultSubscription(
      authSourceDefaults[authSource.source].subscriptions,
    );
    if (duplicate) {
      return {
        ok: false,
        error: {
          kind: "duplicateAuthSourceDefaultSubscription",
          groupId: duplicate.group_id,
          sourceTitle: authSource.title,
        },
      };
    }
  }

  if (form.wechat_connect_mp_enabled && form.wechat_connect_mobile_enabled) {
    return {
      ok: false,
      error: { kind: "conflictingWeChatApplications" },
    };
  }

  // These fields are optional, and the backend rejects invalid URL strings.
  if (!isValidHttpUrl(form.frontend_url)) form.frontend_url = "";
  if (!isValidHttpUrl(form.doc_url)) form.doc_url = "";

  syncWeChatConnectMode();
  const wechatStoredMode = deriveWeChatConnectStoredMode(
    form.wechat_connect_open_enabled,
    form.wechat_connect_mp_enabled,
    form.wechat_connect_mobile_enabled,
    form.wechat_connect_mode,
  );
  const claudeOAuthSystemPromptBlocksJSON =
    serializeClaudeOAuthSystemPromptBlocks();
  form.claude_oauth_system_prompt_blocks =
    claudeOAuthSystemPromptBlocksJSON;

  return {
    ok: true,
    normalizedDefaultSubscriptions,
    wechatStoredMode,
    claudeOAuthSystemPromptBlocksJSON,
  };
}
