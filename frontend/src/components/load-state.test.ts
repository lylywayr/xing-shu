import { describe, expect, it } from 'vitest'
import { loadStateContract } from './load-state'

describe('load state contract', () => {
  it('defines distinct skeleton, timeout, error, and empty states', () => {
    expect(loadStateContract.states).toEqual(['loading', 'timeout', 'error', 'empty'])
    expect(loadStateContract.timeoutAfterMs).toBe(3000)
    expect(loadStateContract.retryLabel).toBe('重试')
  })
})
