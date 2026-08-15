import type { Model } from '../types'

export interface ModelApprovalChange { key: string; allow: boolean }
export interface ModelAdmissionChange { key: string; admitted: boolean }

export function modelKey(model: Model): string {
  return `${model.provider}/${model.id}`
}

export function selectableProviderModels(models: Model[], providerId: string): Model[] {
  return models.filter(model => model.provider === providerId && (model.status === 'unknown' || model.status === 'active'))
}

export function currentProviderSelection(models: Model[], providerId: string): string[] {
  return selectableProviderModels(models, providerId).filter(model => model.admitted === true).map(modelKey)
}

export function providerAdmissionChanges(models: Model[], providerId: string, selected: string[]): ModelAdmissionChange[] {
  const chosen = new Set(selected)
  return selectableProviderModels(models, providerId)
    .filter(model => chosen.has(modelKey(model)) !== (model.admitted === true))
    .map(model => ({ key: modelKey(model), admitted: chosen.has(modelKey(model)) }))
}
