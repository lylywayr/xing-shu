import type { Model, Provider, Row } from '../types'

export type GovernanceStatus = 'active' | 'unknown' | 'meta' | 'stale' | 'retired' | 'disabled'

export function governanceCounts(records: Row[]): Record<GovernanceStatus, number> {
  const counts: Record<GovernanceStatus, number> = { active: 0, unknown: 0, meta: 0, stale: 0, retired: 0, disabled: 0 }
  for (const record of records) {
    const status = String(record.status || 'unknown') as GovernanceStatus
    if (status in counts) counts[status] += 1
  }
  return counts
}

export function autoExclusionReasons(model: Model, provider?: Provider): string[] {
  const reasons: string[] = []
  if (model.status !== 'active') reasons.push(`状态为 ${model.status}`)
  if (!model.auto_routable) reasons.push('未通过 Auto 路由资格')
  if (model.structured_output_known && !model.structured_output) reasons.push('结构化输出不支持')
  if (!model.structured_output_known) reasons.push('结构化输出能力未知')
  if (provider?.disabled || provider?.status === 'disabled') reasons.push('Provider 已停用')
  if (provider?.cooldown_active || provider?.status === 'cooldown') reasons.push('Provider 冷却中')
  if (provider?.sync_error || provider?.status === 'error') reasons.push('Provider 同步失败')
  return reasons
}

export function capabilityVerificationComplete(model: Model): boolean {
  return ['structured_output', 'tools', 'vision'].every(capability => {
    const evidence = model.capability_evidence?.[capability]
    return evidence?.source === 'runtime_probe' && evidence?.level === 'protocol'
  })
}

export function evidenceGroups(model: Model): Array<{ capability: string; evidence: Row | null }> {
  const names = ['tools', 'vision', 'structured_output']
  return names.map(capability => ({ capability, evidence: model.capability_evidence?.[capability] as unknown as Row || null }))
}
