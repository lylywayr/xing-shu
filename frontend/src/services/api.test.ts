import { describe, expect, it, vi } from 'vitest'
import { request } from './api'

describe('request', () => {
  it('throws a structured error on failed API responses', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('provider unavailable', { status: 503 })))
    await expect(request('/api/admin/status', { retries: 0 })).rejects.toMatchObject({ message: 'provider unavailable', status: 503 })
  })
})
