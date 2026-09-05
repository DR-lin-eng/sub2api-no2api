import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

import { describe, expect, it } from "vitest";

const currentDir = dirname(fileURLToPath(import.meta.url));
const coreFieldsSource = readFileSync(
  resolve(currentDir, "../presentation/widgets/GroupEditorCoreFields.vue"),
  "utf8",
);
const dialogSource = readFileSync(
  resolve(currentDir, "../presentation/widgets/GroupEditorDialog.vue"),
  "utf8",
);
const pricingSource = readFileSync(
  resolve(currentDir, "../presentation/widgets/GroupEditorModelPricingFields.vue"),
  "utf8",
);

describe("groups models list layout", () => {
  it("keeps the toolbar outside of the scrolling list content", () => {
    expect(coreFieldsSource).toContain("overflow-hidden rounded-lg border");
    expect(coreFieldsSource).toContain("max-h-64 space-y-2 overflow-y-auto p-2");
    expect(coreFieldsSource).not.toContain("sticky top-0");
  });

  it("keeps the modular pricing editor usable as fields grow", () => {
    expect(dialogSource).toContain('width="wide"');
    expect(pricingSource).toContain("flex flex-wrap items-start justify-between gap-3");
    expect(pricingSource).toContain("shrink-0 whitespace-nowrap");
  });
});


it('uses Gemini native endpoint copy', () => {
  expect(coreFieldsSource).toContain('form.platform === "gemini" ? "/v1beta/models" : "/v1/models"')
})
