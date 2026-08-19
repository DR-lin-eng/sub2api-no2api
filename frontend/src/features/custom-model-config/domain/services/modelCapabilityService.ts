/** 模型能力判断服务 */

import type { ModelCapability, CustomModelConfig } from "@/features/custom-model-config/domain/entities/customModelConfig";

/**
 * 模型能力缓存
 */
let modelCapabilitiesCache: CustomModelConfig[] | null = null;

/**
 * 初始化模型能力缓存
 */
export function initializeModelCapabilities(configs: CustomModelConfig[]): void {
  modelCapabilitiesCache = configs.map((config) => ({
    ...config,
    model_name: config.model_name.trim(),
    capabilities: [...config.capabilities],
  }));
}

/**
 * 清空模型能力缓存
 */
export function clearModelCapabilities(): void {
  modelCapabilitiesCache = null;
}

function resolveConfiguredCapabilities(model: string): Set<ModelCapability> | null {
  if (!modelCapabilitiesCache) {
    return null;
  }

  const normalized = model.trim().toLowerCase();
  const exact = modelCapabilitiesCache.find(
    (config) => !config.prefix_match && config.model_name.toLowerCase() === normalized
  );
  if (exact) {
    return new Set(exact.capabilities);
  }

  let bestPrefix: CustomModelConfig | null = null;
  for (const config of modelCapabilitiesCache) {
    const prefix = config.model_name.toLowerCase();
    if (!config.prefix_match || !prefix || !normalized.startsWith(prefix)) {
      continue;
    }
    if (!bestPrefix || prefix.length > bestPrefix.model_name.length) {
      bestPrefix = config;
    }
  }

  return bestPrefix ? new Set(bestPrefix.capabilities) : null;
}

/**
 * 判断模型是否支持 Image 能力
 */
export function isMediaStudioImageModel(model: string): boolean {
  const normalized = model.trim().toLowerCase();

  // 优先从自定义配置中查找
  const configuredCapabilities = resolveConfiguredCapabilities(normalized);
  if (configuredCapabilities) {
    return configuredCapabilities.has("image");
  }

  // Fallback 到硬编码规则
  return normalized.startsWith('gpt-image-') || (
    normalized.startsWith('grok-imagine') && !normalized.startsWith('grok-imagine-video')
  );
}

/**
 * 判断模型是否支持 Video 能力
 */
export function isMediaStudioVideoModel(model: string): boolean {
  const normalized = model.trim().toLowerCase();

  // 优先从自定义配置中查找
  const configuredCapabilities = resolveConfiguredCapabilities(normalized);
  if (configuredCapabilities) {
    return configuredCapabilities.has("video");
  }

  // Fallback 到硬编码规则
  return normalized.startsWith('grok-imagine-video');
}

/**
 * 判断模型是否支持 Audio 能力
 */
export function isMediaStudioAudioModel(model: string): boolean {
  const normalized = model.trim().toLowerCase();

  // 优先从自定义配置中查找
  const configuredCapabilities = resolveConfiguredCapabilities(normalized);
  if (configuredCapabilities) {
    return configuredCapabilities.has("audio");
  }

  // 暂无硬编码规则
  return false;
}
