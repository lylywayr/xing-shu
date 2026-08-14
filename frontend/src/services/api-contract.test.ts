import { describe, expect, it, vi } from 'vitest'
import { request } from './api'

describe('API reliability contract', () => {
  it('converts a slow request into a typed timeout error', async () => {
    vi.stubGlobal('fetch', vi.fn((_input: RequestInfo | URL, init?: RequestInit) => new Promise((_resolve, reject) => {
      init?.signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')))
    })))
    await expect(request('/slow', { timeoutMs: 5 })).rejects.toMatchObject({ kind: 'timeout', retryable: true })
  })

  it('preserves caller cancellation as an abort error', async () => {
    const controller = new AbortController()
    vi.stubGlobal('fetch', vi.fn((_input: RequestInfo | URL, init?: RequestInit) => new Promise((_resolve, reject) => {
      init?.signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')))
    })))
    const pending = request('/cancel', { signal: controller.signal })
    controller.abort()
    await expect(pending).rejects.toMatchObject({ kind: 'aborted', retryable: false })
  })

  it('classifies unauthorized responses separately', async () => {
    const response = { ok: false, status: 401, headers: { get: (name: string) => name === 'x-request-id' ? 'req-401' : null }, text: async () => 'unauthorized' } as unknown as Response
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response))
    await expect(request('/private')).rejects.toMatchObject({ kind: 'unauthorized', status: 401, requestId: 'req-401', retryable: false })
  })

  it('classifies network failures as retryable', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))
    await expect(request('/offline')).rejects.toMatchObject({ kind: 'network', retryable: true })
  })
})
