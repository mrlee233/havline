import { describe, expect, it } from 'vitest'
import { ref } from 'vue'
import { createVisibilityPoller, type PollingHost } from './useVisibilityPolling'

function fakeHost() {
  let hidden = false
  let intervalFn: (() => void) | null = null
  let intervalId = 0
  const listeners: Array<() => void> = []
  const host: PollingHost = {
    setInterval: (fn) => {
      intervalFn = fn
      return ++intervalId
    },
    clearInterval: () => {
      intervalFn = null
    },
    isHidden: () => hidden,
    addVisibilityListener: (fn) => {
      listeners.push(fn)
    },
    removeVisibilityListener: (fn) => {
      const index = listeners.indexOf(fn)
      if (index >= 0) listeners.splice(index, 1)
    },
  }
  return {
    host,
    setHidden: (value: boolean) => {
      hidden = value
    },
    fireInterval: () => intervalFn?.(),
    fireVisibility: () => listeners.forEach((fn) => fn()),
    hasTimer: () => intervalFn !== null,
    listenerCount: () => listeners.length,
  }
}

describe('createVisibilityPoller', () => {
  it('页面隐藏时不执行', async () => {
    const h = fakeHost()
    let calls = 0
    const poller = createVisibilityPoller(() => { calls++ }, 1000, {}, h.host)
    h.setHidden(true)
    await poller.tick()
    expect(calls).toBe(0)
  })

  it('回到前台立即补一次', async () => {
    const h = fakeHost()
    let calls = 0
    const poller = createVisibilityPoller(() => { calls++ }, 1000, {}, h.host)
    poller.attach()
    h.fireVisibility()
    await Promise.resolve()
    expect(calls).toBe(1)
  })

  it('上一轮未结束时跳过下一轮', async () => {
    const h = fakeHost()
    let calls = 0
    let release: () => void = () => {}
    const pending = new Promise<void>((resolve) => { release = resolve })
    const poller = createVisibilityPoller(() => {
      calls++
      return calls === 1 ? pending : Promise.resolve()
    }, 1000, {}, h.host)
    const first = poller.tick()
    await Promise.resolve()
    await poller.tick()
    expect(calls).toBe(1)
    release()
    await first
    await poller.tick()
    expect(calls).toBe(2)
  })

  it('enabled=false 时不执行', async () => {
    const h = fakeHost()
    const enabled = ref(false)
    let calls = 0
    const poller = createVisibilityPoller(() => { calls++ }, 1000, { enabled }, h.host)
    await poller.tick()
    expect(calls).toBe(0)
    enabled.value = true
    await poller.tick()
    expect(calls).toBe(1)
  })

  it('卸载后不再保留计时器与监听', () => {
    const h = fakeHost()
    const poller = createVisibilityPoller(() => {}, 1000, {}, h.host)
    poller.attach()
    expect(h.hasTimer()).toBe(true)
    expect(h.listenerCount()).toBe(1)
    poller.detach()
    expect(h.hasTimer()).toBe(false)
    expect(h.listenerCount()).toBe(0)
  })

  it('onError 被调用且不影响后续轮询', async () => {
    const h = fakeHost()
    const errors: unknown[] = []
    let calls = 0
    const poller = createVisibilityPoller(() => {
      calls++
      if (calls === 1) throw new Error('boom')
    }, 1000, { onError: (error) => errors.push(error) }, h.host)
    await poller.tick()
    expect(errors).toHaveLength(1)
    expect(poller.lastError.value).toBeInstanceOf(Error)
    await poller.tick()
    expect(calls).toBe(2)
    expect(poller.lastError.value).toBeNull()
  })
})
