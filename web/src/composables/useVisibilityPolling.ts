import { onBeforeUnmount, onMounted, ref, type Ref } from 'vue'

export interface VisibilityPollingOptions {
  /** 为 false 时整轮跳过（例如未选中服务端、弹窗未打开）。 */
  enabled?: Ref<boolean>
  /** 挂载后是否立即执行一次。 */
  immediate?: boolean
  /** 轮询回调抛错时调用；不传则只记录到 lastError。 */
  onError?: (error: unknown) => void
}

/** 注入的宿主接口，便于在无 DOM 环境下测试轮询核心。 */
export interface PollingHost {
  setInterval: (fn: () => void, ms: number) => number
  clearInterval: (id: number) => void
  isHidden: () => boolean
  addVisibilityListener: (fn: () => void) => void
  removeVisibilityListener: (fn: () => void) => void
}

export interface VisibilityPoller {
  start: () => void
  stop: () => void
  tick: () => Promise<void>
  attach: () => void
  detach: () => void
  busy: Ref<boolean>
  lastError: Ref<unknown>
}

function browserPollingHost(): PollingHost {
  return {
    setInterval: (fn, ms) => window.setInterval(fn, ms),
    clearInterval: (id) => window.clearInterval(id),
    isHidden: () => document.visibilityState === 'hidden',
    addVisibilityListener: (fn) => document.addEventListener('visibilitychange', fn),
    removeVisibilityListener: (fn) => document.removeEventListener('visibilitychange', fn),
  }
}

/**
 * 可测试的轮询核心：隐藏时跳过、上一轮未结束时跳过、回到前台立即补一次。
 * 不依赖 Vue 生命周期，宿主能力全部由 host 注入。
 */
export function createVisibilityPoller(
  fn: () => void | Promise<void>,
  intervalMs: number,
  options: VisibilityPollingOptions = {},
  host: PollingHost = browserPollingHost(),
): VisibilityPoller {
  const busy = ref(false)
  const lastError = ref<unknown>(null)
  let timer: number | undefined

  const tick = async () => {
    if (host.isHidden()) return
    if (options.enabled && !options.enabled.value) return
    if (busy.value) return
    busy.value = true
    try {
      await fn()
      lastError.value = null
    } catch (error) {
      lastError.value = error
      options.onError?.(error)
    } finally {
      busy.value = false
    }
  }

  const start = () => {
    if (timer !== undefined) return
    timer = host.setInterval(() => void tick(), intervalMs)
  }

  const stop = () => {
    if (timer === undefined) return
    host.clearInterval(timer)
    timer = undefined
  }

  const onVisibilityChange = () => {
    if (host.isHidden()) return
    void tick()
  }

  const attach = () => {
    host.addVisibilityListener(onVisibilityChange)
    start()
  }

  const detach = () => {
    host.removeVisibilityListener(onVisibilityChange)
    stop()
  }

  return { start, stop, tick, attach, detach, busy, lastError }
}

/**
 * 页面可见时才轮询：隐藏期间跳过，回到前台立即补一次，上一轮未结束时跳过下一轮。
 */
export function useVisibilityPolling(
  fn: () => void | Promise<void>,
  intervalMs: number,
  options: VisibilityPollingOptions = {},
): VisibilityPoller {
  const poller = createVisibilityPoller(fn, intervalMs, options)
  onMounted(() => {
    poller.attach()
    if (options.immediate) void poller.tick()
  })
  onBeforeUnmount(() => poller.detach())
  return poller
}
