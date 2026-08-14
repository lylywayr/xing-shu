import { shallowRef, type Ref } from 'vue'
import type { ApiError } from '../types'
import { errorState, idleState, loadingState, successState, type AsyncState } from './async-state'

export type AsyncLoader<T> = (signal: AbortSignal) => Promise<T>

export function useAsyncTask<T>(empty: (value: T) => boolean = value => Array.isArray(value) && value.length === 0): {
  state: Ref<AsyncState<T>>
  run: (loader: AsyncLoader<T>) => Promise<T>
  cancel: () => void
} {
  const state = shallowRef<AsyncState<T>>(idleState<T>())
  let activeController: AbortController | null = null

  async function run(loader: AsyncLoader<T>): Promise<T> {
    activeController?.abort()
    const controller = new AbortController()
    activeController = controller
    state.value = loadingState(state.value)
    try {
      const data = await loader(controller.signal)
      if (activeController === controller) state.value = successState(data, empty)
      return data
    } catch (cause) {
      const error: ApiError = cause && typeof cause === 'object' && 'message' in cause
        ? cause as ApiError
        : { message: '请求失败', kind: 'network', retryable: true }
      if (activeController === controller) state.value = errorState(error, state.value)
      throw cause
    } finally {
      if (activeController === controller) activeController = null
    }
  }

  function cancel() {
    activeController?.abort()
    activeController = null
  }

  return { state, run, cancel }
}
