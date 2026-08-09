import { readFileSync, readdirSync } from "node:fs";
import { dirname, extname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const currentDir = dirname(fileURLToPath(import.meta.url));
const featureDir = resolve(currentDir, "..");
const readFeatureSource = (relativePath: string) =>
  readFileSync(resolve(featureDir, relativePath), "utf8");

function collectRuntimeSources(
  directory: string,
): Array<{ path: string; source: string }> {
  const sources: Array<{ path: string; source: string }> = [];
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    if (entry.name === "__tests__") continue;
    const absolutePath = join(directory, entry.name);
    if (entry.isDirectory()) {
      sources.push(...collectRuntimeSources(absolutePath));
      continue;
    }
    if (!new Set([".ts", ".vue"]).has(extname(entry.name))) continue;
    sources.push({
      path: relative(featureDir, absolutePath),
      source: readFileSync(absolutePath, "utf8"),
    });
  }
  return sources;
}

const facadeSource = readFeatureSource(
  "data/datasources/adminSettingsDatasource.ts",
);
const querySource = readFeatureSource(
  "data/datasources/adminSettingsQueries.ts",
);
const actionSource = readFeatureSource(
  "data/datasources/adminSettingsActions.ts",
);
const systemDtoSource = readFeatureSource("data/dtos/systemSettingsDtos.ts");
const pageSource = readFeatureSource("presentation/composables/useSettingsPage.ts");

describe("admin settings modularization", () => {
  it("owns the main settings contract and pure compatibility rules in DTOs", () => {
    expect(systemDtoSource).toContain("export interface SystemSettings");
    expect(systemDtoSource).toContain("export interface UpdateSettingsRequest");
    expect(systemDtoSource).toContain("export function buildAuthSourceDefaultsState");
    expect(systemDtoSource).toContain("export function normalizeWeChatConnectMode");
    expect(systemDtoSource).not.toContain("apiClient");
    expect(systemDtoSource).not.toContain("data/datasources");
  });

  it("keeps the compatibility datasource free of protocol implementation", () => {
    expect(facadeSource).toContain(
      'export * from "@/features/admin-settings/data/dtos/systemSettingsDtos"',
    );
    expect(facadeSource).toContain("export const settingsAPI = {");
    expect(facadeSource).not.toContain("apiClient");
    expect(facadeSource).not.toContain("export interface SystemSettings");
    expect(facadeSource).not.toContain("export function normalize");
  });

  it("routes the unified load and save through explicit owners", () => {
    expect(querySource).toContain("export async function getSettings");
    expect(querySource).toContain('apiClient.get<SystemSettings>("/admin/settings")');
    expect(actionSource).toContain("export async function updateSettings");
    expect(actionSource).toContain('"/admin/settings"');
    expect(pageSource).toContain(
      'from "@/features/admin-settings/data/datasources/adminSettingsQueries"',
    );
    expect(pageSource).toContain(
      'from "@/features/admin-settings/data/datasources/adminSettingsActions"',
    );
  });

  it("keeps presentation runtime code off the transitional facade", () => {
    const presentationSources = collectRuntimeSources(
      resolve(featureDir, "presentation"),
    );
    for (const runtime of presentationSources) {
      expect(runtime.source, runtime.path).not.toContain(
        "adminSettingsDatasource",
      );
      expect(runtime.source, runtime.path).not.toContain("settingsAPI.");
    }
  });
});
