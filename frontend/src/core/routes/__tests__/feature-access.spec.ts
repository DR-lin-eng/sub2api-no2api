import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

type NavigationGuard = (
  to: Record<string, any>,
  from: Record<string, any>,
  next: ReturnType<typeof vi.fn>
) => Promise<void>

const routerHarness = vi.hoisted(() => ({
  guard: null as NavigationGuard | null,
}))

const authStore = vi.hoisted(() => ({
  checkAuth: vi.fn(),
  isAuthenticated: true,
  isAdmin: false,
  isSimpleMode: false,
  hasPendingAuthSession: false,
}))

const appStore = vi.hoisted(() => ({
  siteName: 'Sub2API',
  backendModeEnabled: false,
  publicSettingsLoaded: false,
  cachedPublicSettings: null as null | {
    payment_enabled?: boolean
    risk_control_enabled?: boolean
    support_chat_enabled?: boolean
    media_studio_enabled?: boolean
    ipv6_egress_ui_enabled?: boolean
    custom_menu_items?: []
  },
  fetchPublicSettings: vi.fn(),
}))

vi.mock('vue-router', () => ({
  createWebHistory: vi.fn(() => ({})),
  createRouter: vi.fn(() => ({
    beforeEach: vi.fn((guard: NavigationGuard) => {
      routerHarness.guard = guard
    }),
    afterEach: vi.fn(),
    onError: vi.fn(),
  })),
}))

vi.mock('@/features/auth/presentation/stores/authStore', () => ({
  useAuthStore: () => authStore,
}))

vi.mock('@/core/stores/appStore', () => ({
  useAppStore: () => appStore,
}))

vi.mock('@/features/admin-settings/presentation/stores/adminSettingsStore', () => ({
  useAdminSettingsStore: () => ({ customMenuItems: [] }),
}))

vi.mock('@/features/admin-settings/presentation/stores/adminComplianceStore', () => ({
  useAdminComplianceStore: () => ({
    initialized: true,
    fetchStatus: vi.fn(),
    requireAcknowledgement: vi.fn(),
  }),
}))

vi.mock('@/common/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({
    startNavigation: vi.fn(),
    endNavigation: vi.fn(),
    isLoading: { value: false },
  }),
}))

vi.mock('@/common/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({
    triggerPrefetch: vi.fn(),
    cancelPendingPrefetch: vi.fn(),
    resetPrefetchState: vi.fn(),
  }),
}))

function createDeferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

function runGuard(meta: Record<string, unknown>, path: string) {
  if (!routerHarness.guard) {
    throw new Error('router guard was not registered')
  }

  const next = vi.fn()
  const navigation = routerHarness.guard(
    {
      path,
      fullPath: path,
      name: 'FeatureRoute',
      params: {},
      meta: { requiresAuth: true, ...meta },
    },
    {},
    next
  )
  return { navigation, next }
}

describe('feature route guard', () => {
  beforeAll(async () => {
    await import('@/core/routes')
  })

  beforeEach(() => {
    authStore.isAuthenticated = true
    authStore.isAdmin = false
    authStore.isSimpleMode = false
    appStore.publicSettingsLoaded = false
    appStore.cachedPublicSettings = null
    appStore.fetchPublicSettings.mockReset()
    authStore.checkAuth.mockReset().mockResolvedValue(undefined)
  })

  it('waits for cookie session restoration before evaluating the first route', async () => {
    const deferred = createDeferred<void>()
    authStore.isAuthenticated = false
    authStore.checkAuth.mockReturnValueOnce(deferred.promise)

    const { navigation, next } = runGuard({}, '/dashboard')
    await Promise.resolve()
    expect(next).not.toHaveBeenCalled()

    authStore.isAuthenticated = true
    deferred.resolve()
    await navigation
    expect(next).toHaveBeenCalledWith()
  })

  it('waits for the first public-settings request before deciding payment access', async () => {
    const deferred = createDeferred<{ payment_enabled: boolean }>()
    appStore.fetchPublicSettings.mockImplementation(async () => {
      const settings = await deferred.promise
      appStore.cachedPublicSettings = settings
      appStore.publicSettingsLoaded = true
      return settings
    })

    const { navigation, next } = runGuard({ requiresPayment: true }, '/purchase')

    await vi.waitFor(() => expect(appStore.fetchPublicSettings).toHaveBeenCalledTimes(1))
    expect(next).not.toHaveBeenCalled()

    deferred.resolve({ payment_enabled: true })
    await navigation
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith()
  })

  it.each([
    ['payment', { requiresPayment: true }, '/purchase'],
    ['risk control', { requiresRiskControl: true }, '/admin/risk-control'],
  ])('does not treat a failed %s settings load as explicitly disabled', async (_name, meta, path) => {
    authStore.isAdmin = meta.requiresRiskControl === true
    appStore.fetchPublicSettings.mockResolvedValue(null)

    const { navigation, next } = runGuard(meta, path)
    await navigation

    expect(appStore.publicSettingsLoaded).toBe(false)
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith()
  })

  it('fails closed for support chat when public settings cannot be loaded', async () => {
    appStore.fetchPublicSettings.mockResolvedValue(null)

    const { navigation, next } = runGuard({ requiresSupportChat: true }, '/support')
    await navigation

    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith('/dashboard')
  })

  it.each([
    ['payment', { requiresPayment: true }, { payment_enabled: false }, '/dashboard'],
    [
      'risk control',
      { requiresRiskControl: true },
      { risk_control_enabled: false },
      '/admin/settings',
    ],
    [
      'support chat',
      { requiresSupportChat: true },
      { support_chat_enabled: false },
      '/dashboard',
    ],
    [
      'media studio',
      { requiresMediaStudio: true },
      { media_studio_enabled: false },
      '/dashboard',
    ],
  ])('redirects when loaded settings explicitly disable %s', async (_name, meta, settings, target) => {
    authStore.isAdmin = meta.requiresRiskControl === true || meta.requiresIPv6Egress === true
    appStore.cachedPublicSettings = settings
    appStore.publicSettingsLoaded = true

    const { navigation, next } = runGuard(meta, '/feature')
    await navigation

    expect(appStore.fetchPublicSettings).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith(target)
  })

  it('redirects support chat unless settings explicitly enable it', async () => {
    appStore.cachedPublicSettings = {}
    appStore.publicSettingsLoaded = true

    const { navigation, next } = runGuard({ requiresSupportChat: true }, '/support')
    await navigation

    expect(next).toHaveBeenCalledWith('/dashboard')
  })

  it('allows support chat only when settings explicitly enable it', async () => {
    appStore.cachedPublicSettings = { support_chat_enabled: true }
    appStore.publicSettingsLoaded = true

    const { navigation, next } = runGuard({ requiresSupportChat: true }, '/support')
    await navigation

    expect(next).toHaveBeenCalledWith()
  })

  it('fails closed for media studio unless settings explicitly enable it', async () => {
    appStore.cachedPublicSettings = {}
    appStore.publicSettingsLoaded = true

    const blocked = runGuard({ requiresMediaStudio: true }, '/media-studio')
    await blocked.navigation
    expect(blocked.next).toHaveBeenCalledWith('/dashboard')

    appStore.cachedPublicSettings = { media_studio_enabled: true }
    const allowed = runGuard({ requiresMediaStudio: true }, '/media-studio')
    await allowed.navigation
    expect(allowed.next).toHaveBeenCalledWith()
  })

  it('keeps IPv6 egress management reachable so the page can own its runtime switch', async () => {
    authStore.isAdmin = true
    appStore.cachedPublicSettings = {}
    appStore.publicSettingsLoaded = true

    const navigation = runGuard({ requiresAdmin: true, requiresIPv6Egress: true }, '/admin/egress')
    await navigation.navigation
    expect(navigation.next).toHaveBeenCalledWith()
  })
})
