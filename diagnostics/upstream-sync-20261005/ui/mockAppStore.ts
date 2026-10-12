export function useAppStore() {
  return { showError(message: string) { throw new Error(message) } }
}
