import {
  cancelOrder,
  createChannel,
  createPlan,
  createProvider,
  deleteChannel,
  deletePlan,
  deleteProvider,
  queryRefund,
  refundOrder,
  retryRecharge,
  updateChannel,
  updateConfig,
  updatePlan,
  updateProvider,
} from './adminPaymentActions'
import {
  getChannels,
  getConfig,
  getDashboard,
  getOrder,
  getOrders,
  getPlans,
  getProviders,
} from './adminPaymentQueries'

export type {
  AdminPaymentConfig,
  AdminPaymentOrderQuery,
  AdminRefundRequest,
  RefundResult,
  UpdatePaymentConfigRequest,
} from '../dtos/adminPaymentDtos'

export const adminPaymentAPI = {
  getConfig,
  updateConfig,
  getDashboard,
  getOrders,
  getOrder,
  cancelOrder,
  retryRecharge,
  refundOrder,
  queryRefund,
  getChannels,
  createChannel,
  updateChannel,
  deleteChannel,
  getPlans,
  createPlan,
  updatePlan,
  deletePlan,
  getProviders,
  createProvider,
  updateProvider,
  deleteProvider,
}

export default adminPaymentAPI
