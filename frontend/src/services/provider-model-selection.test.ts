import { describe, expect, it } from 'vitest'
import { currentProviderSelection, providerAdmissionChanges, selectableProviderModels } from './provider-model-selection'
import type { Model } from '../types'

const model = (id: string, extra: Partial<Model> = {}): Model => ({ id, provider: 'p1', status: 'unknown', auto_routable: false, structured_output: false, structured_output_known: false, capabilities: [], context_window: 0, tools: false, vision: false, ...extra })

describe('provider model selection', () => {
  const models = [model('pending'), model('active', { status: 'active', admitted: true, auto_routable: true }), model('stale', { status: 'stale' }), model('other', { provider: 'p2' })]

  it('lists only selectable models for one provider', () => {
    expect(selectableProviderModels(models, 'p1').map(item => item.id)).toEqual(['pending', 'active'])
  })

  it('initializes from current approvals and submits only differences', () => {
    expect(currentProviderSelection(models, 'p1')).toEqual(['p1/active'])
    expect(providerAdmissionChanges(models, 'p1', ['p1/pending'])).toEqual([
      { key: 'p1/pending', admitted: true },
      { key: 'p1/active', admitted: false },
    ])
  })
})
