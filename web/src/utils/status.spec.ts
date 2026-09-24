import { describe, expect, it } from 'vitest'
import { httpStatusKind, statusKind, statusLabel } from './status'

describe('statusKind', () => {
  it('把后端状态值映射成 StatusKind', () => {
    expect(statusKind('ok')).toBe('success')
    expect(statusKind('running')).toBe('success')
    expect(statusKind('warning')).toBe('warning')
    expect(statusKind('error')).toBe('error')
    expect(statusKind('stopped')).toBe('disabled')
    expect(statusKind('none')).toBe('disabled')
    expect(statusKind(undefined)).toBe('unknown')
    expect(statusKind('')).toBe('unknown')
  })

  it('回归：n-tag 的 type 值不是状态值，必须落到 unknown', () => {
    // 曾把 serverStartupType() 返回的 'success'/'default' 传给 StatusBadge :value，
    // 导致服务端详情弹窗「运行中」也显示灰点。
    expect(statusKind('success')).toBe('unknown')
    expect(statusKind('default')).toBe('unknown')
  })
})

describe('statusLabel', () => {
  it('已知值给中文，未知值原样返回', () => {
    expect(statusLabel('ok')).toBe('正常')
    expect(statusLabel('stopped')).toBe('已停止')
    expect(statusLabel('weird')).toBe('weird')
    expect(statusLabel()).toBe('未知')
  })
})

describe('httpStatusKind', () => {
  it('按状态码分段', () => {
    expect(httpStatusKind(200)).toBe('success')
    expect(httpStatusKind(301)).toBe('unknown')
    expect(httpStatusKind(404)).toBe('warning')
    expect(httpStatusKind(503)).toBe('error')
  })
})
