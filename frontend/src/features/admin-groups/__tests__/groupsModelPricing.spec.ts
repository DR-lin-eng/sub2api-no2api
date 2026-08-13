import { describe, expect, it, vi } from "vitest";
import {
  createGroupPricingEntry,
  groupPricingFromAPI,
  groupPricingToAPI,
  updateGroupPricingModels,
} from "../presentation/groupsModelPricing";

describe("group model pricing codec", () => {
  it("keeps token cards on the base tier and converts MTok prices", () => {
    const entry = createGroupPricingEntry();
    entry.models = [" grok-4.6 "];
    entry.input_price = 3;
    entry.output_price = "15";
    entry.intervals = [
      {
        min_tokens: 200_000,
        max_tokens: null,
        tier_label: "long",
        input_price: 6,
        output_price: 30,
        cache_write_price: null,
        cache_read_price: null,
        per_request_price: null,
        sort_order: 0,
      },
    ];

    expect(groupPricingToAPI([entry], "grok")).toEqual([
      expect.objectContaining({
        platform: "grok",
        models: ["grok-4.6"],
        billing_mode: "token",
        input_price: 0.000003,
        output_price: 0.000015,
        intervals: [],
      }),
    ]);
  });

  it("round-trips per-request tiers without applying token conversion to call prices", () => {
    const form = groupPricingFromAPI([
      {
        platform: "grok",
        models: ["grok-voice"],
        billing_mode: "per_request",
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        image_input_price: null,
        image_output_price: null,
        per_request_price: 0.02,
        intervals: [
          {
            min_tokens: 0,
            max_tokens: null,
            tier_label: "tts",
            input_price: null,
            output_price: null,
            cache_write_price: null,
            cache_read_price: null,
            per_request_price: 0.03,
            sort_order: 0,
          },
        ],
      },
    ]);

    expect(groupPricingToAPI(form, "grok")[0]).toMatchObject({
      per_request_price: 0.02,
      intervals: [{ tier_label: "tts", per_request_price: 0.03 }],
    });
  });

  it("fills an empty token card from the default pricing endpoint", async () => {
    const pricing = [createGroupPricingEntry()];
    const loadDefaultPricing = vi.fn().mockResolvedValue({
      found: true,
      input_price: 3e-6,
      output_price: 15e-6,
      cache_read_price: 0.75e-6,
    });

    await updateGroupPricingModels(
      pricing,
      0,
      ["grok-4.6"],
      loadDefaultPricing,
    );

    expect(loadDefaultPricing).toHaveBeenCalledWith("grok-4.6");
    expect(pricing[0]).toMatchObject({
      models: ["grok-4.6"],
      input_price: 3,
      output_price: 15,
      cache_read_price: 0.75,
    });
  });

  it("does not overwrite a manually configured price", async () => {
    const entry = createGroupPricingEntry();
    entry.input_price = 1.25;
    const pricing = [entry];
    const loadDefaultPricing = vi.fn();

    await updateGroupPricingModels(
      pricing,
      0,
      ["custom-model"],
      loadDefaultPricing,
    );

    expect(loadDefaultPricing).not.toHaveBeenCalled();
    expect(pricing[0].input_price).toBe(1.25);
  });
});
