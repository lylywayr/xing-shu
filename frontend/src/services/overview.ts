import type { OverviewStatus } from '../types'

export type OverviewTone = 'blue' | 'green' | 'amber' | 'red' | 'muted'

export function overviewHealthLabel(status: OverviewStatus): string {
  if (status === 'ready') return '就绪'
  if (status === 'healthy') return '健康'
  if (status === 'unhealthy') return '异常'
  return '未知'
}

export function overviewTone(status: string | undefined): OverviewTone {
  if (status === 'ready' || status === 'healthy' || status === 'active') return 'green'
  if (status === 'unhealthy' || status === 'disabled') return 'red'
  return 'amber'
}
