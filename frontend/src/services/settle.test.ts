import { describe, expect, it } from 'vitest'
import { settleRecord } from './settle'

describe('settleRecord', () => {
  it('returns successful values while isolating failed requests', async () => {
    const result = await settleRecord({
      models: Promise.resolve(['model-a']),
      providers: Promise.reject(new Error('provider down')),
    })
    expect(result.values).toEqual({ models: ['model-a'] })
    expect(result.errors).toHaveLength(1)
    expect(result.errors[0].key).toBe('providers')
  })
})
