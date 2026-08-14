import type { ApiError } from '../types'

export type AsyncStatus = 'idle' | 'loading' | 'success' | 'empty' | 'error' | 'timeout' | 'unauthorized'

export interface AsyncState<T> {
  status: AsyncStatus
  data: T | null
  error: ApiError | null
  updatedAt: number | null
}

export function idleState<T>(): AsyncState<T> {
  return { status: 'idle', data: null, error: null, updatedAt: null }
}

export function loadingState<T>(previous: AsyncState<T> | null = null): AsyncState<T> {
  return { status: 'loading', data: previous?.data ?? null, error: null, updatedAt: previous?.updatedAt ?? null }
}

export function successState<T>(data: T, isEmpty: (value: T) => boolean = value => Array.isArray(value) && value.length === 0): AsyncState<T> {
  return { status: isEmpty(data) ? 'empty' : 'success', data, error: null, updatedAt: Date.now() }
}

export function errorState<T>(error: ApiError, previous: AsyncState<T> | null = null): AsyncState<T> {
  const status: AsyncStatus = error.kind === 'timeout' ? 'timeout' : error.kind === 'unauthorized' ? 'unauthorized' : 'error'
  return { status, data: previous?.data ?? null, error, updatedAt: previous?.updatedAt ?? null }
}

export function isTerminal<T>(state: AsyncState<T>): boolean {
  return state.status === 'success' || state.status === 'empty' || state.status === 'error' || state.status === 'timeout' || state.status === 'unauthorized'
}

export function canRetry<T>(state: AsyncState<T>): boolean {
  return state.status === 'timeout' || state.status === 'error' && Boolean(state.error?.retryable)
}
