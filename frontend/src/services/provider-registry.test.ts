import { describe, expect, it } from 'vitest'
import { canValidateProvider, emptyProviderDraft, providerToDraft, validationSteps } from './provider-registry'

describe('provider registry wizard', () => {
  it('requires a safe id, URL, name and API key for creation', () => {
    const draft = emptyProviderDraft()
    expect(canValidateProvider(draft, false)).toBe(false)
    Object.assign(draft, { id: 'demo-provider', name: 'Demo', base_url: 'https://api.example.com', api_key: 'secret' })
    expect(canValidateProvider(draft, false)).toBe(true)
    draft.id = 'Bad ID'
    expect(canValidateProvider(draft, false)).toBe(false)
  })
  it('allows an existing provider to retain its encrypted key', () => {
    const draft = providerToDraft({ id: 'demo', name: 'Demo', kind: 'standard', models: 2, base_url: 'https://api.example.com/v1' })
    expect(draft.api_key).toBe('')
    expect(canValidateProvider(draft, true)).toBe(true)
  })
  it('shows every required validation step', () => {
    expect(validationSteps.map(step => step.key)).toEqual(['network', 'auth', 'models', 'chat'])
  })
})
