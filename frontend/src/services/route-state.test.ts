import { describe, expect, it } from 'vitest'
import type { PageKey } from '../types'
import { parseRouteState, serializeRouteState, type RouteState } from './route-state'

describe('route state persistence', () => {
  it('serializes only navigable state', () => {
    const state: RouteState = { page: 'catalog', query: 'luna', status: 'active', provider: 'cctq', currentPage: 3 }
    expect(serializeRouteState(state)).toBe('?page=catalog&query=luna&status=active&provider=cctq&p=3')
  })

  it('round-trips URL state and clamps invalid pages', () => {
    expect(parseRouteState('?page=reviews&query=x&p=0')).toEqual({ page: 'reviews', query: 'x', status: '', provider: '', currentPage: 1 })
    expect(parseRouteState('?page=not-a-page&p=999')).toEqual({ page: 'overview', query: '', status: '', provider: '', currentPage: 999 })
  })

  it('does not accept unknown page values', () => {
    const parsed = parseRouteState('?page=shadow')
    expect(parsed.page as PageKey).toBe('shadow')
  })
})
