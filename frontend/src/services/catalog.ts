import type { Model, Provider } from '../types'

export type CapabilityFilter = 'all' | 'tools' | 'vision' | 'structured_output' | 'unknown'
export type ProviderOperationalTone = 'green' | 'amber' | 'red' | 'muted'

export function providerOperationalLabel(provider: Provider): string {
  if (provider.disabled || provider.status === 'disabled') return '已停用'
  if (provider.status === 'cooldown' || provider.cooldown_active) return '冷却中'
  if (provider.status === 'error' || provider.sync_error) return '同步失败'
  if (!provider.last_sync) return '未同步'
  return '运行中'
}

export function providerOperationalTone(provider: Provider): ProviderOperationalTone {
  if (provider.disabled || provider.status === 'disabled' || provider.status === 'error' || provider.sync_error) return 'red'
  if (provider.status === 'cooldown' || provider.cooldown_active) return 'amber'
  if (!provider.last_sync) return 'muted'
  return 'green'
}

export function capabilityMatches(model: Model, filter: CapabilityFilter): boolean {
  if (filter === 'all') return true
  if (filter === 'tools') return model.tools
  if (filter === 'vision') return model.vision
  if (filter === 'structured_output') return model.structured_output_known && model.structured_output
  if (filter === 'unknown') return !model.structured_output_known
  return false
}

export function filterModels(models: Model[], query: string, provider: string, status: string, capability: CapabilityFilter): Model[] {
  const needle = query.trim().toLowerCase()
  return models.filter(model => {
    const text = `${model.id} ${model.provider} ${(model.capabilities || []).join(' ')}`.toLowerCase()
    return (!needle || text.includes(needle)) && (!provider || model.provider === provider) && (!status || model.status === status) && capabilityMatches(model, capability)
  })
}

export function selectableModelIds(models: Model[], selected: string[]): string[] {
  const allowed = new Set(models.filter(model => model.status === 'active').map(model => `${model.provider}/${model.id}`))
  return selected.filter(id => allowed.has(id))
}
