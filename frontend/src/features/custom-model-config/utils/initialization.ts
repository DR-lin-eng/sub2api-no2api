/** 模型能力初始化工具 */

import { customModelConfigDatasource } from "../data/datasources/customModelConfigDatasource";
import { initializeModelCapabilities } from "../domain/services/modelCapabilityService";

/**
 * 应用启动时初始化模型能力
 * 非阻塞，失败时使用fallback规则
 */
export function initializeModelCapabilitiesOnStartup(): void {
  customModelConfigDatasource
    .getAll()
    .then((configs) => {
      initializeModelCapabilities(configs);
      console.log(`[ModelCapability] Loaded ${configs.length} custom model configs`);
    })
    .catch((error) => {
      console.warn("[ModelCapability] Failed to load custom configs, using fallback rules:", error);
    });
}
