import { describe, expect, it } from 'vitest'
import type { Model, Provider, Row } from '../types'
import { autoExclusionReasons, evidenceGroups, governanceCounts } from './governance'

const model = (extra: Partial<Model> = {}): Model => ({ id: 'm1', provider: 'p1', status: 'unknown', auto_routable: false, structured_output: false, structured_output_known: false, capabilities: [], context_window: 0, tools: false, vision: false, ...extra })
const provider = (extra: Partial<Provider> = {}): Provider => ({ id: 'p1', kind: 'credit', models: 1, ...extra })

describe('governance traceability', () => {
  it('counts every supported governance state', () => {
    const counts = governanceCounts(['active', 'unknown', 'disabled', 'retired'].map(status => ({ status })) as Row[])
    expect(counts.active).toBe(1); expect(counts.unknown).toBe(1); expect(counts.disabled).toBe(1); expect(counts.retired).toBe(1)
  })
  it('explains model and provider auto exclusion reasons', () => {
    const reasons = autoExclusionReasons(model(), provider({ status: 'cooldown', cooldown_active: true }))
    expect(reasons).toEqual(expect.arrayContaining(['状态为 unknown', '未通过 Auto 路由资格', '结构化输出能力未知', 'Provider 冷却中']))
  })
  it('groups capability evidence without inventing missing evidence', () => {
    const groups = evidenceGroups(model({ structured_output_known: true, structured_output: true, capability_evidence: { structured_output: { source: 'probe' } as never } }))
    expect(groups.find(item => item.capability === 'structured_output')?.evidence?.source).toBe('probe')
    expect(groups.find(item => item.capability === 'vision')?.evidence).toBeNull()
  })
})
