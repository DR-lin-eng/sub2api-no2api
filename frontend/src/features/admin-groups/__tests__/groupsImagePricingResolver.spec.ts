import { describe, expect, it } from "vitest";

import {
  getDefaultImagePreviewPrice,
  getDefaultVideoPreviewPrice,
  getImagePricePlaceholder,
  getVideoPricePlaceholder,
  imagePricingPlatforms,
  imagePricingI18nKey,
  supportsImagePricingPlatform,
  supportsDefaultVideoPriceFallback,
  supportsVideoPricingPlatform,
  videoPricingI18nKey,
} from "../presentation/groupsImagePricingResolver";

describe("groups image pricing platform support", () => {
  it("includes Grok image groups", () => {
    expect(supportsImagePricingPlatform("grok")).toBe(true);
    expect(imagePricingPlatforms.has("grok")).toBe(true);
  });

  it("includes composite groups for routed image generation", () => {
    expect(supportsImagePricingPlatform("composite")).toBe(true);
    expect(imagePricingPlatforms.has("composite")).toBe(true);
  });

  it("includes multimodal groups for image generation pricing", () => {
    expect(supportsImagePricingPlatform("multimodal")).toBe(true);
    expect(imagePricingPlatforms.has("multimodal")).toBe(true);
  });

  it("enables group-level video pricing controls only for Grok", () => {
    expect(supportsVideoPricingPlatform("grok")).toBe(true);
    // multimodal 是逐模型定价分组，分组级视频价不参与计费，编辑器不展示该区块。
    expect(supportsVideoPricingPlatform("multimodal")).toBe(false);
    expect(supportsVideoPricingPlatform("openai")).toBe(false);
  });

  it("keeps non-media group platforms out of the image pricing controls", () => {
    expect(supportsImagePricingPlatform("anthropic")).toBe(false);
  });

  it("keeps image and video pricing copy separate", () => {
    expect(imagePricingI18nKey("grok", "title")).toBe(
      "admin.groups.imagePricing.title",
    );
    expect(videoPricingI18nKey("title")).toBe("admin.groups.videoPricing.title");
  });

  it("keeps the built-in per-second fallback Grok-only", () => {
    // 只有 Grok 留空时后端会回落到内置每秒价。
    expect(supportsDefaultVideoPriceFallback("grok")).toBe(true);
    expect(supportsDefaultVideoPriceFallback("multimodal")).toBe(false);
  });

  it("uses Grok media defaults instead of generic image fallback placeholders", () => {
    expect(getImagePricePlaceholder("grok", "image_price_1k")).toBe("0.02");
    expect(getImagePricePlaceholder("grok", "image_price_2k")).toBe("0.02");
    // 视频 placeholder 为每秒单价：480p/720p 取 grok-imagine-video 官方每秒价，
    // 1080p 仅 video-1.5 支持、取 1.5 每秒价。
    expect(getVideoPricePlaceholder("grok", "video_price_480p")).toBe("0.05");
    expect(getVideoPricePlaceholder("grok", "video_price_720p")).toBe("0.07");
    expect(getVideoPricePlaceholder("grok", "video_price_1080p")).toBe("0.25");
  });

  it("keeps non-Grok image placeholders on the generic image card", () => {
    expect(getImagePricePlaceholder("openai", "image_price_1k")).toBe("0.134");
    expect(getDefaultImagePreviewPrice("openai", "image_price_2k")).toBe(0.201);
    expect(getDefaultVideoPreviewPrice("openai", "video_price_480p")).toBeNull();
  });

  it("shows no video price placeholder for multimodal", () => {
    // multimodal 没有内置每秒价，占位值会被误读成默认价。
    expect(getVideoPricePlaceholder("multimodal", "video_price_480p")).toBe("");
    expect(getVideoPricePlaceholder("multimodal", "video_price_720p")).toBe("");
    expect(getVideoPricePlaceholder("multimodal", "video_price_1080p")).toBe("");
    expect(getDefaultVideoPreviewPrice("multimodal", "video_price_480p")).toBeNull();
  });
});
