/** 自定义模型配置实体 */

export type ModelCapability = "image" | "video" | "audio";

export interface CustomModelRequestTemplate {
  id: number;
  name: string;
  description: string;
  request_adapter: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface CustomModelConfig {
  id: number;
  model_name: string;
  prefix_match: boolean;
  capabilities: ModelCapability[];
  template_id?: number | null;
  template_name?: string;
  created_at: string;
  updated_at: string;
}
