/** Local account limits are independent of upstream pool credentials. */
export const supportsAccountQuota = (type: string | undefined): boolean =>
  type === 'apikey' || type === 'bedrock' || type === 'service_account'
