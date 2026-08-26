import { computed, ref } from "vue";
import {
  defaultFingerprintSignalRows,
  parseFingerprintSignalsToRows,
  serializeFingerprintRowsToJSON,
  type FingerprintSignalRow,
} from "@/features/admin-accounts/presentation/codexFingerprintSignals";
import type { LoginAgreementDocument } from "@/types";
import {
  normalizeLoginAgreementDocumentId,
} from "./settingsAgreementResolver";
import {
  tablePageSizeMax,
  tablePageSizeMin,
  type SettingsForm,
} from "./settingsForm";

interface CodexClientRow {
  originator: string;
  uaContains: string;
  skipEngineFingerprint?: boolean;
}

export function useSettingsStructuredEditors(form: SettingsForm) {
  const tablePageSizeOptionsInput = ref("10, 20, 50, 100");
  const codexBlacklistRows = ref<CodexClientRow[]>([]);
  const codexWhitelistRows = ref<CodexClientRow[]>([]);
  const codexFingerprintRows = ref<FingerprintSignalRow[]>([]);
  const codexFingerprintNoRequired = computed(
    () => !codexFingerprintRows.value.some((row) => row.required),
  );

  function addMenuItem() {
    form.custom_menu_items.push({
      id: "",
      label: "",
      icon_svg: "",
      url: "",
      visibility: "user",
      sort_order: form.custom_menu_items.length,
      forward_access_token: false,
      forward_access_token_in_url: false,
    });
  }

  function removeMenuItem(index: number) {
    form.custom_menu_items.splice(index, 1);
    form.custom_menu_items.forEach((item, itemIndex) => {
      item.sort_order = itemIndex;
    });
  }

  function moveMenuItem(index: number, direction: -1 | 1) {
    const targetIndex = index + direction;
    if (targetIndex < 0 || targetIndex >= form.custom_menu_items.length) return;
    const items = form.custom_menu_items;
    const current = items[index];
    items[index] = items[targetIndex];
    items[targetIndex] = current;
    items.forEach((item, itemIndex) => {
      item.sort_order = itemIndex;
    });
  }

  function addEndpoint() {
    form.custom_endpoints.push({ name: "", endpoint: "", description: "" });
  }

  function removeEndpoint(index: number) {
    form.custom_endpoints.splice(index, 1);
  }

  function addLoginAgreementDocument() {
    form.login_agreement_documents.push({
      id: `custom-${Date.now().toString(36)}`,
      title: "",
      content_md: "",
    });
  }

  function removeLoginAgreementDocument(index: number) {
    form.login_agreement_documents.splice(index, 1);
  }

  function normalizeLoginAgreementDocumentsForSave(): LoginAgreementDocument[] {
    return form.login_agreement_documents
      .map((document, index) => ({
        id:
          normalizeLoginAgreementDocumentId(document.id || document.title) ||
          `doc-${index + 1}`,
        title: document.title.trim(),
        content_md: document.content_md.trim(),
      }))
      .filter((document) => document.title || document.content_md);
  }

  function findDuplicateLoginAgreementDocumentId(
    documents: LoginAgreementDocument[],
  ): string | null {
    const seen = new Set<string>();
    for (const document of documents) {
      if (seen.has(document.id)) return document.id;
      seen.add(document.id);
    }
    return null;
  }

  function formatTablePageSizeOptions(options: number[]): string {
    return options.join(", ");
  }

  function parseTablePageSizeOptionsInput(raw: string): number[] | null {
    const tokens = raw
      .split(",")
      .map((token) => token.trim())
      .filter((token) => token.length > 0);
    if (tokens.length === 0) return null;

    const parsed = tokens.map((token) => Number(token));
    if (parsed.some((value) => !Number.isInteger(value))) return null;

    const deduped = Array.from(new Set(parsed)).sort((a, b) => a - b);
    if (
      deduped.some(
        (value) => value < tablePageSizeMin || value > tablePageSizeMax,
      )
    ) {
      return null;
    }
    return deduped;
  }

  function addCodexFingerprintRow(): void {
    codexFingerprintRows.value.push({
      type: "header_exact",
      match: "",
      required: false,
    });
  }

  function removeCodexFingerprintRow(index: number): void {
    codexFingerprintRows.value.splice(index, 1);
  }

  function parseCodexEntriesToRows(raw: string): CodexClientRow[] {
    if (!raw || !raw.trim()) return [];
    try {
      const entries = JSON.parse(raw);
      if (!Array.isArray(entries)) return [];
      return entries.map((entry) => ({
        originator:
          typeof entry?.originator === "string" ? entry.originator : "",
        uaContains: Array.isArray(entry?.ua_contains)
          ? entry.ua_contains
              .filter((value: unknown) => typeof value === "string")
              .join(", ")
          : "",
        skipEngineFingerprint: entry?.skip_engine_fingerprint === true,
      }));
    } catch {
      return [];
    }
  }

  function serializeCodexRowsToJSON(rows: CodexClientRow[]): string {
    const entries = rows
      .map((row) => {
        const entry: {
          originator: string;
          ua_contains: string[];
          skip_engine_fingerprint?: boolean;
        } = {
          originator: row.originator.trim(),
          ua_contains: row.uaContains
            .split(",")
            .map((value) => value.trim())
            .filter((value) => value.length > 0),
        };
        if (row.skipEngineFingerprint) {
          entry.skip_engine_fingerprint = true;
        }
        return entry;
      })
      .filter(
        (entry) => entry.originator !== "" || entry.ua_contains.length > 0,
      );
    return entries.length > 0 ? JSON.stringify(entries) : "";
  }

  function addCodexBlacklistRow(): void {
    codexBlacklistRows.value.push({ originator: "", uaContains: "" });
  }

  function removeCodexBlacklistRow(index: number): void {
    codexBlacklistRows.value.splice(index, 1);
  }

  function addCodexWhitelistRow(): void {
    codexWhitelistRows.value.push({
      originator: "",
      uaContains: "",
      skipEngineFingerprint: false,
    });
  }

  function removeCodexWhitelistRow(index: number): void {
    codexWhitelistRows.value.splice(index, 1);
  }

  return {
    addCodexBlacklistRow,
    addCodexFingerprintRow,
    addCodexWhitelistRow,
    addEndpoint,
    addLoginAgreementDocument,
    addMenuItem,
    codexBlacklistRows,
    codexFingerprintNoRequired,
    codexFingerprintRows,
    codexWhitelistRows,
    defaultFingerprintSignalRows,
    findDuplicateLoginAgreementDocumentId,
    formatTablePageSizeOptions,
    moveMenuItem,
    normalizeLoginAgreementDocumentsForSave,
    parseCodexEntriesToRows,
    parseFingerprintSignalsToRows,
    parseTablePageSizeOptionsInput,
    removeCodexBlacklistRow,
    removeCodexFingerprintRow,
    removeCodexWhitelistRow,
    removeEndpoint,
    removeLoginAgreementDocument,
    removeMenuItem,
    serializeCodexRowsToJSON,
    serializeFingerprintRowsToJSON,
    tablePageSizeOptionsInput,
  };
}
