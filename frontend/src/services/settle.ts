export interface SettledError { key: string; error: unknown }
export interface SettledRecord<T extends Record<string, unknown>> { values: Partial<T>; errors: SettledError[] }

export async function settleRecord<T extends Record<string, Promise<unknown>>>(tasks: T): Promise<SettledRecord<{ [K in keyof T]: Awaited<T[K]> }>> {
  const entries = Object.entries(tasks)
  const settled = await Promise.all(entries.map(async ([key, promise]) => {
    try { return { key, value: await promise, ok: true as const } }
    catch (error) { return { key, error, ok: false as const } }
  }))
  const values: Record<string, unknown> = {}
  const errors: SettledError[] = []
  for (const item of settled) {
    if (item.ok) values[item.key] = item.value
    else errors.push({ key: item.key, error: item.error })
  }
  return { values, errors } as SettledRecord<{ [K in keyof T]: Awaited<T[K]> }>
}
