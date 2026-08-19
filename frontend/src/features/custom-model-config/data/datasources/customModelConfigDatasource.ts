/** 自定义模型配置 Datasource */

import { apiClient } from "@/core/networks/client";
import type {
  CustomModelConfigDto,
  CreateCustomModelConfigRequest,
  UpdateCustomModelConfigRequest,
} from "../dtos/customModelConfigDtos";
import type { CustomModelConfig } from "../../domain/entities/customModelConfig";

const BASE_URL = "/admin/custom-model-configs";

/**
 * 将DTO转换为domain实体
 */
function toEntity(dto: CustomModelConfigDto): CustomModelConfig {
  return {
    id: dto.id,
    model_name: dto.model_name,
    prefix_match: dto.prefix_match ?? false,
    capabilities: dto.capabilities,
    created_at: dto.created_at,
    updated_at: dto.updated_at,
  };
}

export const customModelConfigDatasource = {
  /**
   * 获取自定义模型配置列表
   */
  async getAll(): Promise<CustomModelConfig[]> {
    const response = await apiClient.get<CustomModelConfigDto[]>(BASE_URL);
    return response.data.map(toEntity);
  },

  /**
   * 获取单个自定义模型配置
   */
  async get(id: number): Promise<CustomModelConfig> {
    const response = await apiClient.get<CustomModelConfigDto>(`${BASE_URL}/${id}`);
    return toEntity(response.data);
  },

  /**
   * 创建自定义模型配置
   */
  async create(request: CreateCustomModelConfigRequest): Promise<CustomModelConfig> {
    const response = await apiClient.post<CustomModelConfigDto>(BASE_URL, request);
    return toEntity(response.data);
  },

  /**
   * 更新自定义模型配置
   */
  async update(id: number, request: UpdateCustomModelConfigRequest): Promise<CustomModelConfig> {
    const response = await apiClient.put<CustomModelConfigDto>(`${BASE_URL}/${id}`, request);
    return toEntity(response.data);
  },

  /**
   * 删除自定义模型配置
   */
  async delete(id: number): Promise<void> {
    await apiClient.delete(`${BASE_URL}/${id}`);
  },
};
