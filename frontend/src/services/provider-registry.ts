import type { Provider } from '../types'

export type ProviderWizardStep = 'form' | 'validate' | 'save'
export interface ProviderDraft { id: string; name: string; base_url: string; api_key: string; kind: 'standard'; enabled: boolean }
export interface ValidationStep { ok: boolean; status?: number; latency_ms?: number; message?: string }
export interface ProviderValidation { ok: boolean; normalized_base_url?: string; model_count: number; sample_model?: string; error_type?: string; network: ValidationStep; auth: ValidationStep; models: ValidationStep; chat: ValidationStep }
export interface ProviderRegistryResponse { items: Provider[]; credential_key_configured: boolean }

export const emptyProviderDraft = (): ProviderDraft => ({ id: '', name: '', base_url: '', api_key: '', kind: 'standard', enabled: true })
export function providerToDraft(provider: Provider): ProviderDraft { return { id: provider.id, name: provider.name || provider.id, base_url: provider.base_url || '', api_key: '', kind: 'standard', enabled: provider.enabled !== false } }
export function canValidateProvider(draft: ProviderDraft, editing: boolean): boolean { return draft.name.trim().length > 0 && /^https?:\/\//.test(draft.base_url.trim()) && (editing || draft.api_key.trim().length > 0) }
export function providerDraftMissing(draft: ProviderDraft, editing: boolean): string[] { const missing: string[] = []; if (!draft.name.trim()) missing.push('名称'); if (!/^https?:\/\//.test(draft.base_url.trim())) missing.push('有效的 Base URL'); if (!editing && !draft.api_key.trim()) missing.push('API Key'); return missing }
export const validationSteps = [
  { key: 'network', label: '网络连接' },
  { key: 'auth', label: '鉴权验证' },
  { key: 'models', label: '模型目录' },
  { key: 'chat', label: 'Chat 协议' },
] as const
