import type { FrpStatus } from '../api/types'

// 只有配置了 FRP 服务端且存在启用规则时，DDNS 页才提示域名应指向 VPS。
export function shouldShowFrpHint(status: Pick<FrpStatus, 'configured' | 'enabled_proxies'> | null | undefined): boolean {
  return Boolean(status?.configured && status.enabled_proxies > 0)
}
