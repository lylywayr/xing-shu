import type { ApiError, ApiErrorKind } from '../types'

export interface RequestOptions extends Omit<RequestInit, 'signal'> {
  timeoutMs?: number
  retries?: number
  signal?: AbortSignal
}

export class ApiRequestError extends Error implements ApiError {
  readonly status?: number
  readonly kind: ApiErrorKind
  readonly retryable: boolean
  readonly requestId?: string

  constructor(message: string, details: { kind: ApiErrorKind; status?: number; retryable?: boolean; requestId?: string }) {
    super(message)
    this.name = 'ApiRequestError'
    this.kind = details.kind
    this.status = details.status
    this.retryable = details.retryable ?? false
    this.requestId = details.requestId
  }
}

const DEFAULT_TIMEOUT_MS = 15_000
const DEFAULT_RETRIES = 1

function isAbortError(error: unknown): boolean {
  return error instanceof DOMException && error.name === 'AbortError'
}

function requestIdOf(response: Response): string | undefined {
  const headers = response.headers
  if (!headers || typeof headers.get !== 'function') return undefined
  return headers.get('x-request-id') || headers.get('x-correlation-id') || undefined
}

function parsePayload(text: string): unknown {
  if (!text) return null
  try { return JSON.parse(text) } catch { return text }
}

function errorMessage(payload: unknown, status: number): string {
  if (typeof payload === 'string' && payload.trim()) return payload
  if (payload && typeof payload === 'object') {
    const record = payload as Record<string, unknown>
    for (const key of ['message', 'error', 'detail']) {
      if (typeof record[key] === 'string' && record[key]) return record[key] as string
    }
  }
  return `请求失败（HTTP ${status}）`
}

function classifyHttp(response: Response, payload: unknown): ApiRequestError {
  const status = response.status
  const requestId = requestIdOf(response)
  if (status === 401) {
    return new ApiRequestError('登录状态已失效，请重新登录', { kind: 'unauthorized', status, requestId })
  }
  return new ApiRequestError(errorMessage(payload, status), {
    kind: 'http',
    status,
    requestId,
    retryable: status === 408 || status === 425 || status === 429 || status >= 500,
  })
}

function mergeSignals(caller: AbortSignal | undefined, timeoutMs: number): { signal: AbortSignal; cleanup: () => void } {
  const controller = new AbortController()
  let timeout: ReturnType<typeof setTimeout> | undefined
  const abort = () => controller.abort()
  if (caller) {
    if (caller.aborted) controller.abort()
    else caller.addEventListener('abort', abort, { once: true })
  }
  timeout = setTimeout(() => controller.abort(), timeoutMs)
  return {
    signal: controller.signal,
    cleanup: () => {
      if (timeout) clearTimeout(timeout)
      caller?.removeEventListener('abort', abort)
    },
  }
}

export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { timeoutMs = DEFAULT_TIMEOUT_MS, retries = DEFAULT_RETRIES, signal: callerSignal, ...init } = options
  let attempt = 0
  while (true) {
    const merged = mergeSignals(callerSignal, timeoutMs)
    try {
      const response = await fetch(path, { credentials: 'include', cache: 'no-store', ...init, signal: merged.signal })
      const payload = parsePayload(await response.text())
      if (!response.ok) {
        const error = classifyHttp(response, payload)
        if (error.retryable && attempt < retries) {
          attempt += 1
          continue
        }
        throw error
      }
      return payload as T
    } catch (error) {
      if (error instanceof ApiRequestError) throw error
      if (callerSignal?.aborted) {
        throw new ApiRequestError('请求已取消', { kind: 'aborted' })
      }
      if (merged.signal.aborted) {
        if (attempt < retries) {
          attempt += 1
          continue
        }
        throw new ApiRequestError(`请求超时（${timeoutMs}ms）`, { kind: 'timeout', retryable: true })
      }
      const networkError = new ApiRequestError('网络连接失败，请检查网络后重试', { kind: 'network', retryable: true })
      if (attempt < retries) {
        attempt += 1
        continue
      }
      throw networkError
    } finally {
      merged.cleanup()
    }
  }
}

export const get = <T>(path: string, options: RequestOptions = {}) => request<T>(path, options)

export const post = <T>(path: string, body?: unknown, options: RequestOptions = {}) => {
  const form = body instanceof URLSearchParams
  const headers = new Headers(options.headers)
  if (body !== undefined) headers.set('Content-Type', form ? 'application/x-www-form-urlencoded' : 'application/json')
  return request<T>(path, {
    ...options,
    method: 'POST',
    headers: body === undefined && !options.headers ? undefined : headers,
    body: body === undefined ? undefined : form ? body.toString() : JSON.stringify(body),
  })
}
