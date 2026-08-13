import { describe, expect, it } from "vitest";
import { resolveAccountPlanType } from "../presentation/grokPlanResolver";

const now = Date.parse("2026-08-14T00:00:00Z");

const grokAccount = (overrides: Record<string, unknown> = {}) =>
  ({
    platform: "grok",
    credentials: {},
    extra: {},
    parent_plan_type: undefined,
    ...overrides,
  }) as any;

describe("resolveAccountPlanType", () => {
  it("prefers an unambiguous persisted JWT tier over lagging snapshots", () => {
    expect(
      resolveAccountPlanType(
        grokAccount({
          credentials: { subscription_tier: "FREE" },
          extra: {
            grok_billing_snapshot: { plan: "SuperGrok Heavy" },
            grok_usage_snapshot: { subscription_tier: "SuperGrok" },
          },
        }),
        now,
      ),
    ).toBe("FREE");
  });

  it("uses a fresh canonical Grok 4.5 Responses window to disambiguate Heavy", () => {
    expect(
      resolveAccountPlanType(
        grokAccount({
          credentials: { subscription_tier: "SuperGrokPro" },
          extra: {
            grok_billing_snapshot: { plan: "SuperGrok" },
            grok_usage_snapshot: {
              model: "grok-4.5",
              last_headers_seen_at: "2026-08-13T23:00:00Z",
              requests: { limit: 8_300 },
              tokens: { limit: 53_000_000 },
            },
            grok_quota_snapshot: {
              model: "grok-4.6",
              last_headers_seen_at: "2026-08-13T23:00:00Z",
              requests: { limit: 8_300 },
            },
          },
        }),
        now,
      ),
    ).toBe("SuperGrok Heavy");
  });

  it("does not treat another model or stale 4.5 signal as Heavy", () => {
    for (const snapshot of [
      {
        model: "grok-4.6",
        last_headers_seen_at: "2026-08-13T23:00:00Z",
        requests: { limit: 8_300 },
      },
      {
        model: "grok-4.5",
        last_headers_seen_at: "2026-08-10T00:00:00Z",
        requests: { limit: 8_300 },
      },
    ]) {
      expect(
        resolveAccountPlanType(
          grokAccount({
            credentials: { subscription_tier: "SuperGrokPro" },
            extra: {
              grok_billing_snapshot: { plan: "SuperGrok" },
              grok_usage_snapshot: snapshot,
            },
          }),
          now,
        ),
      ).toBe("SuperGrok");
    }
  });

  it("prefers canonical usage and skips malformed plan fields", () => {
    expect(
      resolveAccountPlanType(
        grokAccount({
          extra: {
            grok_billing_snapshot: { plan: {} },
            grok_usage_snapshot: { subscription_tier: "SuperGrok" },
            grok_quota_snapshot: { subscription_tier: "Free" },
          },
        }),
        now,
      ),
    ).toBe("SuperGrok");

    expect(
      resolveAccountPlanType(
        grokAccount({
          credentials: { subscription_tier: 0, plan_type: "SuperGrok Heavy" },
          extra: {
            grok_billing_snapshot: { plan: [] },
            grok_usage_snapshot: { subscription_tier: { name: "Heavy" } },
            grok_quota_snapshot: { subscription_tier: null },
          },
        }),
        now,
      ),
    ).toBe("SuperGrok Heavy");
  });
});
