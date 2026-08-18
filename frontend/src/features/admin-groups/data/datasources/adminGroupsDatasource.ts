/**
 * Transitional admin groups compatibility facade.
 * Runtime consumers should import the explicit DTO, Query, or Action owner.
 */

import {
  batchSetGroupRateMultipliers,
  batchSetGroupRPMOverrides,
  clearGroupRateMultipliers,
  clearGroupRPMOverrides,
  create,
  createCompositeRoute,
  deleteCompositeRoute,
  deleteGroup,
  duplicate,
  toggleStatus,
  update,
  updateCompositeRoute,
  updateSortOrder,
} from './adminGroupActions'
import {
  getAll,
  getAllIncludingInactive,
  getById,
  getByPlatform,
  getCapacitySummary,
  getGroupApiKeys,
  getGroupRateMultipliers,
  getGroupRPMOverrides,
  getLiveCapability,
  getModelsListCandidates,
  getStats,
  getUsageSummary,
  list,
  listCompositeRoutes,
  previewCompositeRoute,
} from './adminGroupQueries'

export * from './adminGroupActions'
export * from './adminGroupQueries'
export * from '../dtos/adminGroupDtos'

export const groupsAPI = {
  list,
  getAll,
  getByPlatform,
  getAllIncludingInactive,
  getLiveCapability,
  getById,
  getModelsListCandidates,
  create,
  duplicate,
  update,
  delete: deleteGroup,
  toggleStatus,
  getStats,
  getGroupApiKeys,
  listCompositeRoutes,
  createCompositeRoute,
  updateCompositeRoute,
  deleteCompositeRoute,
  previewCompositeRoute,
  getGroupRateMultipliers,
  clearGroupRateMultipliers,
  batchSetGroupRateMultipliers,
  getGroupRPMOverrides,
  clearGroupRPMOverrides,
  batchSetGroupRPMOverrides,
  updateSortOrder,
  getUsageSummary,
  getCapacitySummary,
}

export default groupsAPI
