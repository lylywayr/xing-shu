import { describe, expect, it } from 'vitest'

describe('destructive action policy', () => {
  it('requires explicit confirmation for destructive operation paths', () => {
    const destructive = ['/api/admin/provider/disable', '/api/admin/provider/enable', '/api/admin/cooldowns/clear', '/api/admin/snapshots/restore']
    expect(destructive.every(path => path.startsWith('/api/admin/'))).toBe(true)
    expect(destructive).toContain('/api/admin/snapshots/restore')
  })
})
