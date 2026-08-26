import { createI18n } from 'vue-i18n'
import { safeLocalStorage } from '@/core/utils/safeStorage'
import { canonicalizeRoutePath } from '@/core/utils/routePath'

type LocaleCode = 'en' | 'zh'

type LocaleMessages = Record<string, any>
export type LocaleScope = 'base' | 'user' | 'batchImage' | 'mediaStudio' | 'supportChat' | 'admin'

const LOCALE_KEY = 'sub2api_locale'
const DEFAULT_LOCALE: LocaleCode = 'en'

const localeLoaders: Record<LocaleCode, Record<LocaleScope, () => Promise<LocaleMessages>>> = {
  en: {
    base: async () => {
      const [landing, common] = await Promise.all([
        import('./locales/en/landing'),
        import('./locales/en/common'),
      ])
      return { ...landing.default, ...common.default }
    },
    user: async () => {
      const [dashboard, misc] = await Promise.all([
        import('./locales/en/dashboard'),
        import('./locales/en/misc'),
      ])
      return { ...dashboard.default, ...misc.default }
    },
    batchImage: async () => (await import('./locales/en/batchImage')).default,
    mediaStudio: async () => (await import('./locales/en/mediaStudio')).default,
    supportChat: async () => (await import('./locales/en/supportChat')).default,
    admin: async () => ({ admin: (await import('./locales/en/admin')).default }),
  },
  zh: {
    base: async () => {
      const [landing, common] = await Promise.all([
        import('./locales/zh/landing'),
        import('./locales/zh/common'),
      ])
      return { ...landing.default, ...common.default }
    },
    user: async () => {
      const [dashboard, misc] = await Promise.all([
        import('./locales/zh/dashboard'),
        import('./locales/zh/misc'),
      ])
      return { ...dashboard.default, ...misc.default }
    },
    batchImage: async () => (await import('./locales/zh/batchImage')).default,
    mediaStudio: async () => (await import('./locales/zh/mediaStudio')).default,
    supportChat: async () => (await import('./locales/zh/supportChat')).default,
    admin: async () => ({ admin: (await import('./locales/zh/admin')).default }),
  },
}

const USER_ROUTE_PREFIXES = [
  '/dashboard',
  '/keys',
  '/batch-image',
  '/media-studio',
  '/usage',
  '/redeem',
  '/affiliate',
  '/available-channels',
  '/profile',
  '/subscriptions',
  '/support',
  '/purchase',
  '/orders',
  '/payment',
  '/custom',
  '/model-plaza',
  '/monitor',
]

function matchesRoutePrefix(path: string, prefix: string): boolean {
  return path === prefix || path.startsWith(`${prefix}/`)
}

export function getLocaleScopesForRoute(pathname: string): LocaleScope[] {
  const path = canonicalizeRoutePath(pathname.split(/[?#]/, 1)[0] || '/')
  const scopes: LocaleScope[] = ['base']

  if (matchesRoutePrefix(path, '/admin')) {
    scopes.push('user', 'admin')
    if (matchesRoutePrefix(path, '/admin/support')) {
      scopes.push('supportChat')
    }
    return scopes
  }

  if (
    USER_ROUTE_PREFIXES.some((prefix) => matchesRoutePrefix(path, prefix)) ||
    path === '/auth/wechat/payment/callback'
  ) {
    scopes.push('user')
  }
  if (matchesRoutePrefix(path, '/batch-image')) {
    scopes.push('batchImage')
  }
  if (matchesRoutePrefix(path, '/media-studio')) {
    scopes.push('mediaStudio')
  }
  if (matchesRoutePrefix(path, '/support')) {
    scopes.push('supportChat')
  }

  return scopes
}

function isLocaleCode(value: string): value is LocaleCode {
  return value === 'en' || value === 'zh'
}

function getDefaultLocale(): LocaleCode {
  const saved = safeLocalStorage.getItem(LOCALE_KEY)
  if (saved && isLocaleCode(saved)) {
    return saved
  }

  const browserLang = navigator.language.toLowerCase()
  if (browserLang.startsWith('zh')) {
    return 'zh'
  }

  return DEFAULT_LOCALE
}

export const i18n = createI18n({
  legacy: false,
  locale: getDefaultLocale(),
  fallbackLocale: DEFAULT_LOCALE,
  messages: {},
  // 禁用 HTML 消息警告 - 引导步骤使用富文本内容（driver.js 支持 HTML）
  // 这些内容是内部定义的，不存在 XSS 风险
  warnHtmlMessage: false
})

const loadedScopes = new Map<LocaleCode, Set<LocaleScope>>()
const scopeLoadPromises = new Map<string, Promise<void>>()
const activatedScopes = new Set<LocaleScope>(['base'])

export async function loadLocaleMessages(
  locale: LocaleCode,
  scopes: readonly LocaleScope[] = [...activatedScopes],
): Promise<void> {
  scopes.forEach((scope) => activatedScopes.add(scope))
  let localeScopes = loadedScopes.get(locale)
  if (!localeScopes) {
    localeScopes = new Set<LocaleScope>()
    loadedScopes.set(locale, localeScopes)
  }

  await Promise.all(scopes.map(async (scope) => {
    if (localeScopes.has(scope)) return

    const loadKey = `${locale}:${scope}`
    let pending = scopeLoadPromises.get(loadKey)
    if (!pending) {
      pending = localeLoaders[locale][scope]().then((messages) => {
        i18n.global.mergeLocaleMessage(locale, messages)
        localeScopes.add(scope)
      }).finally(() => {
        scopeLoadPromises.delete(loadKey)
      })
      scopeLoadPromises.set(loadKey, pending)
    }
    await pending
  }))
}

export async function loadRouteLocaleMessages(pathname: string): Promise<void> {
  await loadLocaleMessages(getLocale(), getLocaleScopesForRoute(pathname))
}

export async function initI18n(): Promise<void> {
  const current = getLocale()
  await loadLocaleMessages(current, ['base'])
  document.documentElement.setAttribute('lang', current)
}

export async function setLocale(locale: string): Promise<void> {
  if (!isLocaleCode(locale)) {
    return
  }

  await loadLocaleMessages(locale)
  i18n.global.locale.value = locale
  safeLocalStorage.setItem(LOCALE_KEY, locale)
  document.documentElement.setAttribute('lang', locale)

  // 同步更新浏览器页签标题，使其跟随语言切换
  const { resolveRouteDocumentTitle } = await import('@/core/routes/title')
  const { default: router } = await import('@/core/routes')
  const { useAppStore } = await import('@/core/stores/appStore')
  const { useAuthStore } = await import('@/features/auth')
  const { useAdminSettingsStore } = await import('@/features/admin-settings/presentation/stores/adminSettingsStore')
  const route = router.currentRoute.value
  const appStore = useAppStore()
  const authStore = useAuthStore()
  const adminSettingsStore = useAdminSettingsStore()
  const customMenuItems = [
    ...(appStore.cachedPublicSettings?.custom_menu_items ?? []),
    ...(authStore.isAdmin ? adminSettingsStore.customMenuItems : []),
  ]
  document.title = resolveRouteDocumentTitle(route, appStore.siteName, customMenuItems)
}

export function getLocale(): LocaleCode {
  const current = i18n.global.locale.value
  return isLocaleCode(current) ? current : DEFAULT_LOCALE
}

export const availableLocales = [
  { code: 'en', name: 'English', flag: '🇺🇸' },
  { code: 'zh', name: '中文', flag: '🇨🇳' }
] as const

export default i18n
