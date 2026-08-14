import { describe, expect, it } from 'vitest'
import { useAsyncTask } from './async-task'

describe('useAsyncTask', () => {
  it('exposes loading and success states', async () => {
    const task = useAsyncTask<{ value: string }>(() => false)
    const pending = task.run(async () => ({ value: 'ready' }))
    expect(task.state.value.status).toBe('loading')
    await pending
    expect(task.state.value).toMatchObject({ status: 'success', data: { value: 'ready' } })
  })

  it('cancels the previous request when a new request starts', async () => {
    const task = useAsyncTask<string>(() => false)
    let firstAborted = false
    const first = task.run((_signal) => new Promise<string>((_resolve, reject) => {
      _signal.addEventListener('abort', () => { firstAborted = true; reject({ message: 'cancelled', kind: 'aborted' }) })
    })).catch(() => undefined)
    await task.run(async () => 'second')
    await first
    expect(firstAborted).toBe(true)
    expect(task.state.value.data).toBe('second')
  })
})
