import { describe, expect, it } from 'vitest'
import { mobileMore, mobilePrimary, navGroups } from './navigation'

describe('navigation model', () => {
  it('keeps mobile navigation to four primary entries', () => {
    expect(mobilePrimary).toHaveLength(4)
    expect(mobilePrimary.map(item => item.key)).toEqual(['overview', 'resources', 'catalog', 'reviews'])
  })
  it('puts every secondary page in the more menu exactly once', () => {
    const all = navGroups.flatMap(group => group.items).map(item => item.key)
    expect(new Set(all).size).toBe(all.length)
    expect(mobileMore.map(item => item.key)).not.toContain('overview')
    expect(mobileMore.map(item => item.key)).not.toContain('reviews')
    expect(mobileMore.length).toBe(all.length - mobilePrimary.length)
  })
})
