import { describe, expect, it } from 'vitest'
import { overviewTone, overviewHealthLabel } from './overview'
import type { Overview } from '../types'

describe('overview data contract', () => {
  it('maps live health states without hardcoded healthy values', () => {
    const overview: Overview = {
      health: {
        service: { status: 'ready' },
        overall: 'degraded',
      },
      consistency: { catalog: 1, governance: 1, missing_governance: 0, ok: true },
      providers: [{ id: 'p1', kind: 'credit', models: 1, status: 'disabled', disabled: true }],
      recent: { count: 2, items: [] },
      alerts: { count: 1, items: [] },
      generated_at: '2026-08-08T00:00:00Z',
    }
    expect(overviewHealthLabel(overview.health.service.status)).toBe('就绪')
    expect(overviewTone(overview.health.overall)).toBe('amber')
    expect(overviewTone(overview.providers[0].status || 'unknown')).toBe('red')
  })
})
