import { onBeforeUnmount, onMounted, type Ref } from 'vue'

/**
 * 页面可见时才轮询：标签页隐藏期间跳过这一轮，回到前台立即补一次。
 *
 * 各页原来的写法是「自己 setInterval + 在回调里判断 document.visibilityState」，
 * 但只有仪表盘这么做了，其余页面在后台标签页里仍在持续打接口（公网反代页每轮 7 个请求）。
 *
 * @param fn 每轮执行的回调；由调用方自行保证幂等与「无数据时直接 return」
 * @param intervalMs 轮询间隔
 * @param enabled 可选开关，为 false 时整轮跳过（例如未选中服务端）
 */
export function useVisibilityPolling(fn: () => void | Promise<void>, intervalMs: number, enabled?: Ref<boolean>) {
  let timer: number | undefined

  const tick = () => {
    if (document.visibilityState === 'hidden') return
    if (enabled && !enabled.value) return
    void fn()
  }

  const stop = () => {
    if (timer === undefined) return
    window.clearInterval(timer)
    timer = undefined
  }

  const onVisibilityChange = () => {
    if (document.visibilityState === 'hidden') return
    tick() // 回到前台立刻补一次，不用等下一个周期
  }

  onMounted(() => {
    document.addEventListener('visibilitychange', onVisibilityChange)
    timer = window.setInterval(tick, intervalMs)
  })

  onBeforeUnmount(() => {
    document.removeEventListener('visibilitychange', onVisibilityChange)
    stop()
  })

  return { stop }
}
