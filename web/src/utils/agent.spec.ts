import { describe, expect, it } from 'vitest'
import { agentVersionText } from './agent'

describe('agentVersionText', () => {
  it('有版本号时原样展示', () => {
    expect(agentVersionText('0.14.1')).toBe('0.14.1')
  })

  it('旧 Agent 缺少版本字段时降级为占位符', () => {
    expect(agentVersionText()).toBe('—')
    expect(agentVersionText('')).toBe('—')
    expect(agentVersionText('  ')).toBe('—')
  })
})
