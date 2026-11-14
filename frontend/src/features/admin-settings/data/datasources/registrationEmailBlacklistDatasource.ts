import { apiClient } from '@/core/networks/client'

export interface RegistrationEmailBlacklist { emails: string[]; max: number }
export async function getRegistrationEmailBlacklist(): Promise<RegistrationEmailBlacklist> {
  const { data } = await apiClient.get<RegistrationEmailBlacklist>('/admin/settings/registration-email-blacklist')
  return data
}
export async function updateRegistrationEmailBlacklist(emails: string[]): Promise<RegistrationEmailBlacklist & { added: string[] }> {
  const { data } = await apiClient.put<RegistrationEmailBlacklist & { added: string[] }>('/admin/settings/registration-email-blacklist', { emails })
  return data
}
