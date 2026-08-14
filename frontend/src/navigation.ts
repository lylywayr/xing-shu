import type { PageKey } from './types'

export interface NavItem { key: PageKey; label: string; icon: string }
export interface NavGroup { label: string; items: NavItem[] }

export const navGroups: NavGroup[] = [
  { label: '指挥台', items: [{ key: 'overview', label: '总览', icon: '⌂' }, { key: 'routing', label: '路由中心', icon: '◎' }] },
  { label: '资源与能力', items: [{ key: 'resources', label: '资源池', icon: '◫' }, { key: 'integrations', label: '外部集成', icon: '⇄' }, { key: 'catalog', label: '模型目录', icon: '▦' }, { key: 'governance', label: '能力治理', icon: '◇' }] },
  { label: '学习闭环', items: [{ key: 'reviews', label: '教师复盘', icon: '✦' }, { key: 'knowledge', label: '路由知识', icon: '⌁' }, { key: 'shadow', label: '影子验证', icon: '◌' }] },
  { label: '运维', items: [{ key: 'logs', label: '调用与告警', icon: '◒' }, { key: 'accounts', label: '账号额度', icon: '◷' }, { key: 'snapshots', label: '版本快照', icon: '◇' }, { key: 'operations', label: '运维控制', icon: '⚙' }, { key: 'system', label: '系统状态', icon: '⌁' }] }
]

export const mobilePrimary: NavItem[] = [
  { key: 'overview', label: '总览', icon: '⌂' },
  { key: 'resources', label: '资源', icon: '◫' },
  { key: 'catalog', label: '模型', icon: '▦' },
  { key: 'reviews', label: '复盘', icon: '✦' }
]

export const mobileMore: NavItem[] = navGroups.flatMap(group => group.items).filter(item => !mobilePrimary.some(primary => primary.key === item.key))
