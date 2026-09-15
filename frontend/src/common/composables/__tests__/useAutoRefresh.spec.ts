import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useAutoRefresh } from '@/common/composables/useAutoRefresh'

describe('useAutoRefresh', () => {
  afterEach(() => {
    vi.useRealTimers()
    window.localStorage.clear()
  })

  it('fires on the selected interval without an extra second and resets to that interval', async () => {
    vi.useFakeTimers()
    const refresh = vi.fn()
    const wrapper = mount(defineComponent({
      setup() {
        return useAutoRefresh({
          storageKey: 'auto-refresh-test',
          intervals: [30, 60] as const,
          defaultInterval: 30,
          onRefresh: refresh,
        })
      },
      template: '<div />',
    }))

    const vm = wrapper.vm as unknown as {
      countdown: number
      setEnabled: (value: boolean) => void
      setInterval: (value: number) => void
      resetCountdown: () => void
    }
    vm.setInterval(60)
    vm.setEnabled(true)
    expect(vm.countdown).toBe(60)
    await vi.advanceTimersByTimeAsync(60_000)
    expect(refresh).toHaveBeenCalledTimes(1)
    expect(vm.countdown).toBe(60)
    wrapper.unmount()
  })
})
