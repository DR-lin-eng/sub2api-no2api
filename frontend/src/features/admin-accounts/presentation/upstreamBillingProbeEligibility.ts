import type { AccountPlatform, AccountType } from "@/types";

const UPSTREAM_BILLING_PROBE_PLATFORMS = new Set<AccountPlatform>([
  "openai",
  "anthropic",
  "gemini",
  "antigravity",
  "grok",
  "typesafe",
]);

export const isUpstreamBillingProbeEligible = (
  platform: AccountPlatform,
  type: AccountType,
): boolean =>
  type === "apikey" && UPSTREAM_BILLING_PROBE_PLATFORMS.has(platform);

export const areUpstreamBillingProbeTargetsEligible = (
  platforms: AccountPlatform[],
  types: AccountType[],
): boolean =>
  platforms.length > 0 &&
  types.length > 0 &&
  platforms.every((platform) => UPSTREAM_BILLING_PROBE_PLATFORMS.has(platform)) &&
  types.every((type) => type === "apikey");
