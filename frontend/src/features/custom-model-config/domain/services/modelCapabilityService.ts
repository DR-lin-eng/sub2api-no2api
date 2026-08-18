/** 模型能力判断服务 */

import type { ModelCapability, CustomModelConfig } from "@/features/custom-model-config/domain/entities/customModelConfig";

/**
 * 模型能力缓存
 */
let modelCapabilitiesCache: Map<string, Set<ModelCapability>> | null = null;

/**
 * 初始化模型能力缓存
 */
export function initializeModelCapabilities(configs: CustomModelConfig[]): void {
  modelCapabilitiesCache = new Map();
  for (const config of configs) {
    modelCapabilitiesCache.set(
      config.model_name.trim().toLowerCase(),
      new Set(config.capabilities)
    );
  }
}

/**
 * 清空模型能力缓存
 */
export function clearModelCapabilities(): void {
  modelCapabilitiesCache = null;
}

/**
 * 判断模型是否支持 Image 能力
 */
export function isMediaStudioImageModel(model: string): boolean {
  const normalized = model.trim().toLowerCase();

  // 优先从自定义配置中查找
  if (modelCapabilitiesCache) {
    const capabilities = modelCapabilitiesCache.get(normalized);
    if (capabilities) {
      return capabilities.has("image");
    }
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
  if (modelCapabilitiesCache) {
    const capabilities = modelCapabilitiesCache.get(normalized);
    if (capabilities) {
      return capabilities.has("video");
    }
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
  if (modelCapabilitiesCache) {
    const capabilities = modelCapabilitiesCache.get(normalized);
    if (capabilities) {
      return capabilities.has("audio");
    }
  }

  // 暂无硬编码规则
  return false;
}
