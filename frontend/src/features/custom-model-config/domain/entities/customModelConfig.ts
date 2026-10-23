/** 自定义模型配置实体 */

export type ModelCapability = "image" | "video" | "audio";
export type VideoApiType = "grok" | "agnes";

/** 媒体工坊参考素材类型 */
export type MediaFileKind = "image" | "video" | "audio";

/** 模型级媒体限制；空对象表示不限制 */
export interface CustomModelMediaLimits {
  allow_upload?: boolean;
  file_types?: MediaFileKind[];
  max_files?: Partial<Record<MediaFileKind, number>>;
  max_file_size_mb?: Partial<Record<MediaFileKind, number>>;
  duration?: { min: number; max: number } | null;
}

export interface CustomModelRequestTemplate {
  id: number;
  name: string;
  description: string;
  request_adapter: Record<string, unknown>;
  attributes: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface CustomModelConfig {
  id: number;
  model_name: string;
  prefix_match: boolean;
  capabilities: ModelCapability[];
  video_api_type?: VideoApiType | null;
  template_id?: number | null;
  template_name?: string;
  media_limits?: CustomModelMediaLimits | null;
  created_at: string;
  updated_at: string;
}
