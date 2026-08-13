import type { Account } from "@/types";

const GROK_QUOTA_SIGNAL_MAX_AGE_MS = 24 * 60 * 60 * 1000;
const GROK_QUOTA_SIGNAL_MAX_FUTURE_SKEW_MS = 5 * 60 * 1000;

const firstNonBlankString = (...values: unknown[]): string | undefined =>
  values.find(
    (value): value is string =>
      typeof value === "string" && value.trim().length > 0,
  );

const normalizeGrokPlanKey = (value: unknown): string =>
  typeof value === "string"
    ? value.trim().toLowerCase().replace(/[\s_-]+/g, "")
    : "";

const asRecord = (value: unknown): Record<string, unknown> | undefined =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;

const grokPersistedQuotaSnapshot = (
  extra: Record<string, unknown>,
): Record<string, unknown> | undefined =>
  asRecord(extra.grok_usage_snapshot) || asRecord(extra.grok_quota_snapshot);

const isGrokQuotaTimestampFresh = (raw: unknown, now: number): boolean => {
  if (typeof raw !== "string" || !raw.trim()) return false;
  const observedAt = Date.parse(raw);
  if (!Number.isFinite(observedAt)) return false;
  const age = now - observedAt;
  return (
    age <= GROK_QUOTA_SIGNAL_MAX_AGE_MS &&
    age >= -GROK_QUOTA_SIGNAL_MAX_FUTURE_SKEW_MS
  );
};

const isGrok45ResponsesQuotaModel = (model: unknown): boolean => {
  if (typeof model !== "string") return false;
  const normalized = model
    .trim()
    .toLowerCase()
    .replace(/^(x-ai|xai)\//, "");
  return normalized === "grok-4.5" || normalized.startsWith("grok-4.5-");
};

const grokQuotaLooksHeavy = (
  snapshot: Record<string, unknown> | undefined,
): boolean => {
  const requests = asRecord(snapshot?.requests);
  const tokens = asRecord(snapshot?.tokens);
  return (
    Number(requests?.limit ?? 0) >= 8_300 ||
    Number(tokens?.limit ?? 0) >= 53_000_000
  );
};

const grok45ResponsesPlanIsHeavy = (
  snapshot: Record<string, unknown> | undefined,
  now: number,
): boolean => {
  if (!snapshot) return false;
  const hint = normalizeGrokPlanKey(snapshot.plan_from_45_responses);
  if (
    hint === "supergrokheavy" &&
    isGrokQuotaTimestampFresh(snapshot.plan_from_45_responses_at, now)
  ) {
    return true;
  }
  return (
    isGrok45ResponsesQuotaModel(snapshot.model) &&
    isGrokQuotaTimestampFresh(
      snapshot.last_headers_seen_at || snapshot.updated_at,
      now,
    ) &&
    grokQuotaLooksHeavy(snapshot)
  );
};

export const resolveAccountPlanType = (
  row: Pick<Account, "platform" | "credentials" | "extra" | "parent_plan_type">,
  now = Date.now(),
): string | undefined => {
  if (row.platform !== "grok") {
    return firstNonBlankString(
      row.credentials?.plan_type,
      row.parent_plan_type,
    );
  }

  const extra = (row.extra || {}) as Record<string, unknown>;
  const billing = asRecord(extra.grok_billing_snapshot);
  const usage = asRecord(extra.grok_usage_snapshot);
  const legacyQuota = asRecord(extra.grok_quota_snapshot);
  const quota = grokPersistedQuotaSnapshot(extra);
  const credentialTier = firstNonBlankString(
    row.credentials?.subscription_tier,
  );
  const credentialTierKey = normalizeGrokPlanKey(credentialTier);
  if (credentialTierKey && credentialTierKey !== "supergrokpro") {
    return credentialTier;
  }

  const billingPlanKey = normalizeGrokPlanKey(billing?.plan);
  if (
    grok45ResponsesPlanIsHeavy(quota, now) &&
    (credentialTierKey === "supergrokpro" ||
      billingPlanKey === "supergrok" ||
      billingPlanKey === "supergrokpro")
  ) {
    return "SuperGrok Heavy";
  }
  if (credentialTierKey === "supergrokpro") {
    return firstNonBlankString(billing?.plan) || "SuperGrok";
  }

  return firstNonBlankString(
    billing?.plan,
    usage?.subscription_tier,
    legacyQuota?.subscription_tier,
    extra.subscription_tier,
    row.credentials?.plan_type,
    row.parent_plan_type,
  );
};
