import { describe, expect, it } from 'vitest'
import { allNavItems, navGroups } from './navigation'

describe('navigation model', () => {
  it('groups every workspace page exactly once', () => {
    const keys = allNavItems.map(item => item.key)
    expect(new Set(keys).size).toBe(keys.length)
    expect(keys).toHaveLength(14)
  })

  it('keeps the most important mobile workspaces easy to scan', () => {
    expect(navGroups.map(group => group.label)).toEqual(['工作台', '模型服务', '路由治理', '学习闭环', '系统运维'])
    expect(allNavItems.find(item => item.key === 'overview')?.description).toBe('健康与关键指标')
    expect(allNavItems.find(item => item.key === 'catalog')?.description).toBe('模型与能力')
    expect(allNavItems.find(item => item.key === 'routing')?.description).toBe('Explain 与决策')
  })
})
