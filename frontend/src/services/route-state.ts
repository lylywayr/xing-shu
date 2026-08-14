import type { PageKey } from '../types'

const pages = new Set<PageKey>(['overview', 'resources', 'integrations', 'catalog', 'routing', 'governance', 'reviews', 'knowledge', 'shadow', 'logs', 'accounts', 'snapshots', 'operations', 'system'])

export interface RouteState {
  page: PageKey
  query: string
  status: string
  provider: string
  currentPage: number
}

export function parseRouteState(search: string): RouteState {
  const params = new URLSearchParams(search)
  const rawPage = params.get('page') as PageKey | null
  const page = rawPage && pages.has(rawPage) ? rawPage : 'overview'
  const rawCurrentPage = Number(params.get('p') || '1')
  return {
    page,
    query: params.get('query') || '',
    status: params.get('status') || '',
    provider: params.get('provider') || '',
    currentPage: Number.isFinite(rawCurrentPage) && rawCurrentPage > 0 ? Math.floor(rawCurrentPage) : 1,
  }
}

export function serializeRouteState(state: RouteState): string {
  const params = new URLSearchParams()
  if (state.page !== 'overview') params.set('page', state.page)
  if (state.query) params.set('query', state.query)
  if (state.status) params.set('status', state.status)
  if (state.provider) params.set('provider', state.provider)
  if (state.currentPage > 1) params.set('p', String(Math.floor(state.currentPage)))
  const value = params.toString()
  return value ? `?${value}` : ''
}
