import { describe, expect, it } from 'vitest'
import type { Model, Provider } from '../types'
import { capabilityMatches, filterModels, providerOperationalLabel, providerOperationalTone, selectableModelIds } from './catalog'

const model = (extra: Partial<Model> = {}): Model => ({ id: 'm1', provider: 'p1', status: 'active', auto_routable: true, structured_output: false, structured_output_known: false, capabilities: [], context_window: 0, tools: false, vision: false, ...extra })
const provider = (extra: Partial<Provider> = {}): Provider => ({ id: 'p1', kind: 'credit', models: 1, ...extra })

describe('provider operational facts', () => {
  it('distinguishes disabled, cooldown, unsynced and active states', () => {
    expect(providerOperationalLabel(provider({ disabled: true }))).toBe('已停用')
    expect(providerOperationalLabel(provider({ status: 'cooldown', cooldown_active: true }))).toBe('冷却中')
    expect(providerOperationalLabel(provider())).toBe('未同步')
    expect(providerOperationalTone(provider({ status: 'cooldown' }))).toBe('amber')
  })
})

describe('catalog capability filters', () => {
  it('does not treat unknown structured output as unsupported', () => {
    expect(capabilityMatches(model(), 'unknown')).toBe(true)
    expect(capabilityMatches(model({ structured_output: true, structured_output_known: true }), 'structured_output')).toBe(true)
    expect(capabilityMatches(model({ structured_output: true, structured_output_known: false }), 'structured_output')).toBe(false)
  })
  it('filters by query, provider, status and capability together', () => {
    const result = filterModels([model({ id: 'vision-a', vision: true }), model({ id: 'text-b', provider: 'p2' })], 'vision', 'p1', 'active', 'vision')
    expect(result.map(item => item.id)).toEqual(['vision-a'])
  })
  it('keeps only active models in batch selection', () => {
    const result = selectableModelIds([model({ id: 'a' }), model({ id: 'b', status: 'disabled' })], ['p1/a', 'p1/b', 'p1/missing'])
    expect(result).toEqual(['p1/a'])
  })
})


describe('provider sync errors', () => {
  it('prioritizes a visible sync error over an old successful sync', () => {
    expect(providerOperationalLabel(provider({ status: 'error', sync_error: 'timeout' }))).toBe('同步失败')
    expect(providerOperationalTone(provider({ status: 'error', sync_error: 'timeout' }))).toBe('red')
  })
})
