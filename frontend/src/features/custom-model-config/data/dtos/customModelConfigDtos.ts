/** 自定义模型配置 DTOs */

import type { ModelCapability, VideoApiType } from "../../domain/entities/customModelConfig";

export interface CustomModelConfigDto {
  id: number;
  model_name: string;
  prefix_match?: boolean;
  capabilities: ModelCapability[];
  template_id?: number | null;
  template_name?: string;
  video_api_type?: VideoApiType | null;
  created_at: string;
  updated_at: string;
}

export interface CreateCustomModelConfigRequest {
  model_name: string;
  prefix_match: boolean;
  capabilities: ModelCapability[];
  template_id: number | null;
  video_api_type?: VideoApiType | null;
}

export interface UpdateCustomModelConfigRequest {
  model_name?: string;
  prefix_match?: boolean;
  capabilities?: ModelCapability[];
  template_id?: number | null;
  video_api_type?: VideoApiType | null;
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
