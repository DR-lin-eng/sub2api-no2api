export type ProfitControlFormState = {
  platform: string;
  profit_control_enabled: boolean;
  profit_min_margin_percent: number | string | null;
  profit_safety_buffer_percent: number | string | null;
};

export const isProfitControlPlatform = (platform: string): boolean =>
  ["openai", "anthropic", "gemini", "grok", "antigravity", "multimodal"].includes(
    platform,
  );

export const profitPercentToDecimal = (
  value: number | string | null | undefined,
): number => {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) return 0;
  return Math.round(parsed * 100) / 10000;
};

export const profitDecimalToPercent = (
  value: number | null | undefined,
): number => {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) return 0;
  return Math.round(parsed * 1e6) / 1e4;
};

export const validateProfitControlFormState = (
  form: ProfitControlFormState,
): string | null => {
  if (!isProfitControlPlatform(form.platform) || !form.profit_control_enabled) {
    return null;
  }
  const marginPercent = Number(form.profit_min_margin_percent || 0);
  const bufferPercent = Number(form.profit_safety_buffer_percent || 0);
  if (!Number.isFinite(marginPercent) || marginPercent < 0) {
    return "marginRangeError";
  }
  if (!Number.isFinite(bufferPercent) || bufferPercent < 0) {
    return "bufferRangeError";
  }
  const margin = profitPercentToDecimal(marginPercent);
  const buffer = profitPercentToDecimal(bufferPercent);
  if (margin >= 1) return "marginRangeError";
  if (buffer >= 1) return "bufferRangeError";
  if (margin + buffer >= 1) return "sumTooHigh";
  return null;
};
