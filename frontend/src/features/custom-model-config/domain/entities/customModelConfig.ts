/** 自定义模型配置实体 */

export type ModelCapability = "image" | "video" | "audio";

export interface CustomModelConfig {
  id: number;
  model_name: string;
  capabilities: ModelCapability[];
  created_at: string;
  updated_at: string;
}
