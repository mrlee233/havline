import { describe, expect, it } from 'vitest'
import { shouldShowFrpHint } from './frp'

describe('shouldShowFrpHint', () => {
  it('配置了服务端且存在启用规则时显示提示', () => {
    expect(shouldShowFrpHint({ configured: true, enabled_proxies: 1 })).toBe(true)
  })

  it('未配置服务端时不显示提示', () => {
    expect(shouldShowFrpHint({ configured: false, enabled_proxies: 1 })).toBe(false)
  })

  it('配置了服务端但没有启用规则时不显示提示', () => {
    expect(shouldShowFrpHint({ configured: true, enabled_proxies: 0 })).toBe(false)
  })

  it('状态为空时不显示提示', () => {
    expect(shouldShowFrpHint(undefined)).toBe(false)
  })
})
