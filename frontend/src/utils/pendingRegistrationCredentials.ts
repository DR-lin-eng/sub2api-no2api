interface PendingRegistrationCredentials {
  email: string
  password: string
}

let pendingCredentials: PendingRegistrationCredentials | null = null

export function setPendingRegistrationCredentials(email: string, password: string): void {
  pendingCredentials = { email, password }
}

export function getPendingRegistrationCredentials(): PendingRegistrationCredentials | null {
  return pendingCredentials ? { ...pendingCredentials } : null
}

export function clearPendingRegistrationCredentials(): void {
  pendingCredentials = null
}
