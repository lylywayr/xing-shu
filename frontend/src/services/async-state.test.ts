import { describe, expect, it } from 'vitest'
import { canRetry, errorState, idleState, loadingState, successState } from './async-state'

describe('async state contract', () => {
  it('keeps previous data visible during loading', () => {
    const previous = successState([{ id: 'one' }])
    expect(loadingState(previous)).toMatchObject({ status: 'loading', data: [{ id: 'one' }] })
  })

  it('distinguishes empty from successful data', () => {
    expect(successState([]).status).toBe('empty')
    expect(successState([{ id: 'one' }]).status).toBe('success')
  })

  it('maps timeout and unauthorized errors to dedicated states', () => {
    expect(errorState({ message: 'slow', kind: 'timeout', retryable: true }).status).toBe('timeout')
    expect(errorState({ message: 'login', kind: 'unauthorized', retryable: false }).status).toBe('unauthorized')
    expect(canRetry(errorState({ message: 'slow', kind: 'timeout', retryable: true }))).toBe(true)
  })

  it('starts in idle without stale data', () => {
    expect(idleState()).toEqual({ status: 'idle', data: null, error: null, updatedAt: null })
  })
})
