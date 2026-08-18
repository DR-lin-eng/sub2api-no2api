/** 自定义模型配置 DTOs */

import type { ModelCapability } from "../../domain/entities/customModelConfig";

export interface CustomModelConfigDto {
  id: number;
  model_name: string;
  capabilities: ModelCapability[];
  created_at: string;
  updated_at: string;
}

export interface CreateCustomModelConfigRequest {
  model_name: string;
  capabilities: ModelCapability[];
}

export interface UpdateCustomModelConfigRequest {
  model_name?: string;
  capabilities?: ModelCapability[];
}
