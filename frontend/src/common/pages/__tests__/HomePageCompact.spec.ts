import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'

import HomePage from '../HomePage.vue'

const { appStore, authStore } = vi.hoisted(() => ({
  appStore: {
    cachedPublicSettings: {} as Record<string, unknown>,
    siteName: 'Fallback site',
    siteLogo: '',
    docUrl: '',
    publicSettingsLoaded: true,
    fetchPublicSettings: vi.fn(),
  },
  authStore: {
    isAuthenticated: false,
    isAdmin: false,
    user: null as { email?: string } | null,
  },
}))

vi.mock('@/core/stores/appStore', () => ({
  useAppStore: () => appStore,
}))

vi.mock('@/features/auth/presentation/stores/authStore', () => ({
  useAuthStore: () => authStore,
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

function mountHome(settings: Record<string, unknown> = {}) {
  appStore.cachedPublicSettings = {
    site_name: 'Test site',
    site_subtitle: 'Test subtitle',
    ...settings,
  }

  return mount(HomePage, {
    global: {
      stubs: {
        RouterLink: RouterLinkStub,
        LocaleSwitcher: { template: '<div data-testid="locale-switcher" />' },
        Icon: { template: '<span data-testid="icon" />' },
      },
    },
  })
}

function compactDestination(wrapper: ReturnType<typeof mountHome>) {
  return wrapper.get('[data-testid="compact-home"]').findComponent(RouterLinkStub).props('to')
}

describe('HomePage compact mode', () => {
  const originalLocation = window.location

  beforeEach(() => {
    authStore.isAuthenticated = false
    authStore.isAdmin = false
    authStore.user = null
    appStore.fetchPublicSettings.mockClear()
    localStorage.clear()
    vi.spyOn(window, 'matchMedia').mockReturnValue({ matches: false } as MediaQueryList)
  })

  afterEach(() => {
    Object.defineProperty(window, 'location', {
      value: originalLocation,
      writable: true,
      configurable: true,
    })
    vi.restoreAllMocks()
  })

  it('keeps custom HTML and URL content ahead of compact mode', () => {
    const html = mountHome({
      compact_home_enabled: true,
      home_content: '<section id="custom-home">Custom home</section>',
    })
    expect(html.get('#custom-home').text()).toBe('Custom home')
    expect(html.find('[data-testid="compact-home"]').exists()).toBe(false)

    const url = mountHome({
      compact_home_enabled: true,
      home_content: ' https://example.com/home ',
    })
    const frame = url.get('[data-testid="custom-home-frame"]')
    expect(frame.attributes('src')).toBe('https://example.com/home')
    expect(frame.attributes('title')).toBe('home.customContentFrameTitle')
    expect(frame.attributes('referrerpolicy')).toBe('no-referrer')
    expect(frame.attributes('sandbox')).toBe('allow-forms allow-scripts allow-popups')
  })

  it('enables storage and popup escape only for a trusted HTTPS sibling app', () => {
    Object.defineProperty(window, 'location', {
      value: {
        origin: 'https://gptcodex.top',
        href: 'https://gptcodex.top/home',
        pathname: '/home',
      },
      writable: true,
      configurable: true,
    })

    const wrapper = mountHome({ home_content: 'https://fuck.gptcodex.top/' })
    const frame = wrapper.get('[data-testid="custom-home-frame"]')
    expect(frame.attributes('sandbox')).toBe(
      'allow-forms allow-scripts allow-popups allow-same-origin allow-popups-to-escape-sandbox',
    )
  })

  it('sanitizes executable custom HTML before rendering the public home page', () => {
    const wrapper = mountHome({
      home_content: `
        <section id="safe-home"><h1>Safe home</h1><img src="/logo.svg" onerror="alert(1)"></section>
        <script>window.pwned = true</script>
        <form><input name="password"><button>Submit</button></form>
        <iframe src="https://evil.example.com"></iframe>
      `,
    })

    const content = wrapper.get('[data-testid="sanitized-home-content"]')
    expect(content.get('#safe-home').text()).toContain('Safe home')
    expect(content.get('img').attributes('onerror')).toBeUndefined()
    expect(content.find('script, form, input, button, iframe').exists()).toBe(false)
  })

  it('does not treat active URL schemes as iframe sources', () => {
    const wrapper = mountHome({ home_content: 'javascript:alert(1)' })
    expect(wrapper.find('[data-testid="custom-home-frame"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="sanitized-home-content"]').text()).toBe('javascript:alert(1)')
  })

  it('treats whitespace-only custom content as empty', () => {
    const wrapper = mountHome({ compact_home_enabled: true, home_content: ' \n\t ' })
    expect(wrapper.get('[data-testid="compact-home"]').text()).toContain('Test site')
  })

  it.each([undefined, false])('uses the default home when compact mode is %s', (enabled) => {
    const settings = enabled === undefined ? {} : { compact_home_enabled: enabled }
    const wrapper = mountHome(settings)
    expect(wrapper.find('[data-testid="compact-home"]').exists()).toBe(false)
    expect(wrapper.find('.terminal-container').exists()).toBe(true)
  })

  it('routes visitors and authenticated users to the appropriate entry point', () => {
    expect(compactDestination(mountHome({ compact_home_enabled: true }))).toBe('/login')

    authStore.isAuthenticated = true
    expect(compactDestination(mountHome({ compact_home_enabled: true }))).toBe('/dashboard')

    authStore.isAdmin = true
    expect(compactDestination(mountHome({ compact_home_enabled: true }))).toBe('/admin/dashboard')
  })

  it('shows the model plaza entry in both built-in home layouts when enabled', () => {
    const compact = mountHome({
      compact_home_enabled: true,
      model_plaza_enabled: true,
    })
    expect(
      compact.get('[data-testid="compact-model-plaza-link"]').findComponent(RouterLinkStub).props('to'),
    ).toBe('/model-plaza')

    const defaultHome = mountHome({ model_plaza_enabled: true })
    expect(
      defaultHome.get('[data-testid="default-model-plaza-link"]').findComponent(RouterLinkStub).props('to'),
    ).toBe('/model-plaza')
  })

  it.each([undefined, false])('hides the model plaza entry when the feature flag is %s', (enabled) => {
    const settings = enabled === undefined ? {} : { model_plaza_enabled: enabled }
    const compact = mountHome({ compact_home_enabled: true, ...settings })
    const defaultHome = mountHome(settings)

    expect(compact.find('[data-testid="compact-model-plaza-link"]').exists()).toBe(false)
    expect(defaultHome.find('[data-testid="default-model-plaza-link"]').exists()).toBe(false)
  })
})
