/** 自定义模型配置 DTOs */

import type { ModelCapability } from "../../domain/entities/customModelConfig";

export interface CustomModelConfigDto {
  id: number;
  model_name: string;
  prefix_match?: boolean;
  capabilities: ModelCapability[];
  template_id?: number | null;
  template_name?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateCustomModelConfigRequest {
  model_name: string;
  prefix_match: boolean;
  capabilities: ModelCapability[];
  template_id: number | null;
}

export interface UpdateCustomModelConfigRequest {
  model_name?: string;
  prefix_match?: boolean;
  capabilities?: ModelCapability[];
  template_id?: number | null;
}

export interface CustomModelRequestTemplateDto {
  id: number;
  name: string;
  description: string;
  request_adapter: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface CreateCustomModelRequestTemplateRequest {
  name: string;
  description: string;
  request_adapter: Record<string, unknown>;
}

export interface UpdateCustomModelRequestTemplateRequest {
  name?: string;
  description?: string;
  request_adapter?: Record<string, unknown>;
}
