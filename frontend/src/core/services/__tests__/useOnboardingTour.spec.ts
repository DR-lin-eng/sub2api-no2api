import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, nextTick, ref } from 'vue'
import { useOnboardingTour } from '../useOnboardingTour'

const mocks = vi.hoisted(() => ({
  driverFactory: vi.fn(),
  drive: vi.fn()
}))

vi.mock('driver.js', () => ({
  driver: mocks.driverFactory
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

vi.mock('@/features/auth', () => ({
  useAuthStore: () => ({
    user: { id: 1, role: 'admin' },
    isAdmin: true,
    isSimpleMode: false
  })
}))

vi.mock('@/core/stores/onboardingStore', () => ({
  useOnboardingStore: () => ({
    getDriverInstance: () => null,
    setDriverInstance: vi.fn(),
    setControlMethods: vi.fn(),
    clearControlMethods: vi.fn(),
    isDriverActive: () => false
  })
}))

vi.mock('@/core/services/guide/steps', () => ({
  getAdminSteps: () => [{ popover: { title: 'Welcome', description: 'Welcome' } }],
  getUserSteps: () => []
}))

describe('useOnboardingTour auto start gate', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    localStorage.clear()
    mocks.drive.mockReset()
    mocks.driverFactory.mockReset()
    mocks.driverFactory.mockReturnValue({
      destroy: vi.fn(),
      drive: mocks.drive,
      isActive: () => false,
      getActiveIndex: () => 0,
      getActiveElement: () => null,
      moveNext: vi.fn(),
      movePrevious: vi.fn()
    })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('waits for a blocking gate before starting the tour', async () => {
    const ready = ref(false)
    const Harness = defineComponent({
      setup() {
        useOnboardingTour({
          storageKey: 'admin_guide',
          autoStart: true,
          autoStartReady: ready
        })
        return () => h('div')
      }
    })

    const wrapper = mount(Harness)
    await nextTick()
    await vi.advanceTimersByTimeAsync(1500)
    expect(mocks.driverFactory).not.toHaveBeenCalled()

    ready.value = true
    await nextTick()
    await vi.advanceTimersByTimeAsync(999)
    expect(mocks.driverFactory).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(1)
    expect(mocks.driverFactory).toHaveBeenCalledTimes(1)
    expect(mocks.drive).toHaveBeenCalledWith(0)

    wrapper.unmount()
  })
})
