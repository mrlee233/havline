import { describe, expect, it } from 'vitest'
import { formatBytes, formatLogLine, formatLogTime, formatRate, formatRelativeTime, formatUptime } from './format'

describe('formatBytes', () => {
  it('按 1024 进制换算并按量级保留有效位', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(1024)).toBe('1.00 KB')
    expect(formatBytes(1536)).toBe('1.50 KB')
    expect(formatBytes(20 * 1024)).toBe('20.0 KB')
    expect(formatBytes(200 * 1024)).toBe('200 KB')
    expect(formatBytes(3 * 1024 ** 3)).toBe('3.00 GB')
  })

  it('非法输入回落 0 B，不产生 NaN', () => {
    expect(formatBytes(Number.NaN)).toBe('0 B')
    expect(formatBytes(Number.POSITIVE_INFINITY)).toBe('0 B')
    expect(formatBytes(-1)).toBe('0 B')
  })
})

describe('formatRate', () => {
  it('复用字节格式化并加 /s', () => {
    expect(formatRate(0)).toBe('0 B/s')
    expect(formatRate(2048)).toBe('2.00 KB/s')
  })
})

describe('formatUptime', () => {
  it('小时为 0 时只显示分钟', () => {
    expect(formatUptime(90)).toBe('1 分钟')
    expect(formatUptime(3661)).toBe('1 小时 1 分钟')
  })
})

describe('formatRelativeTime', () => {
  it('按分钟 / 小时 / 天分档', () => {
    const ago = (minutes: number) => new Date(Date.now() - minutes * 60_000).toISOString()
    expect(formatRelativeTime(undefined)).toBe('-')
    expect(formatRelativeTime(ago(0))).toBe('刚刚')
    expect(formatRelativeTime(ago(5))).toBe('5 分钟前')
    expect(formatRelativeTime(ago(3 * 60))).toBe('3 小时前')
    expect(formatRelativeTime(ago(2 * 24 * 60))).toBe('2 天前')
  })

  it('无法解析的字符串原样返回，不显示 NaN', () => {
    expect(formatRelativeTime('not-a-date')).toBe('not-a-date')
  })
})

describe('formatLogTime', () => {
  it('三种日志时间格式统一成 YYYY-MM-DD HH:mm:ss', () => {
    expect(formatLogTime('2026-09-24 18:30:05')).toBe('2026-09-24 18:30:05')
    expect(formatLogTime('2026-09-24T18:30:05Z')).toBe('2026-09-24 18:30:05')
    expect(formatLogTime('2026/09/24 18:30:05')).toBe('2026-09-24 18:30:05')
    expect(formatLogTime('')).toBe('-')
  })
})

describe('formatLogLine', () => {
  it('只改写行首时间戳，保留其余内容', () => {
    expect(formatLogLine('2026/09/24 18:30:05 [error] boom')).toBe('2026-09-24 18:30:05 [error] boom')
    expect(formatLogLine('2026-09-24T18:30:05Z upstream timeout')).toBe('2026-09-24 18:30:05 upstream timeout')
  })

  it('JSON 日志只替换 time 字段', () => {
    expect(formatLogLine('{"time":"2026-09-24T18:30:05Z","level":"info"}')).toBe('{"time":"2026-09-24 18:30:05","level":"info"}')
  })
})
