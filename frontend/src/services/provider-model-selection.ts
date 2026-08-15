import type { Model } from '../types'

export interface ModelApprovalChange { key: string; allow: boolean }

export function modelKey(model: Model): string {
  return `${model.provider}/${model.id}`
}

export function selectableProviderModels(models: Model[], providerId: string): Model[] {
  return models.filter(model => model.provider === providerId && (model.status === 'unknown' || model.status === 'active'))
}

export function currentProviderSelection(models: Model[], providerId: string): string[] {
  return selectableProviderModels(models, providerId).filter(model => model.auto_routable).map(modelKey)
}

export function providerApprovalChanges(models: Model[], providerId: string, selected: string[]): ModelApprovalChange[] {
  const chosen = new Set(selected)
  return selectableProviderModels(models, providerId)
    .filter(model => chosen.has(modelKey(model)) !== model.auto_routable)
    .map(model => ({ key: modelKey(model), allow: chosen.has(modelKey(model)) }))
}
