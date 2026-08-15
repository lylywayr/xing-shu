import type { PageKey } from './types'

export interface NavItem { key: PageKey; label: string; icon: string; description?: string }
export interface NavGroup { label: string; items: NavItem[] }

export const navGroups: NavGroup[] = [
  { label: '工作台', items: [
    { key: 'overview', label: '总览', icon: '⌂', description: '健康与关键指标' },
  ] },
  { label: '模型服务', items: [
    { key: 'resources', label: '资源池', icon: '◫', description: 'Provider 状态' },
    { key: 'integrations', label: '外部集成', icon: '⇄', description: 'FreeLLMAPI' },
    { key: 'catalog', label: '模型目录', icon: '▦', description: '模型与能力' },
    { key: 'api-access', label: 'API 接入', icon: '⌁', description: '地址与客户端密钥' },
  ] },
  { label: '路由治理', items: [
    { key: 'routing', label: '路由中心', icon: '◎', description: 'Explain 与决策' },
    { key: 'governance', label: '能力治理', icon: '◇', description: '规则与证据' },
  ] },
  { label: '学习闭环', items: [
    { key: 'reviews', label: '教师复盘', icon: '✦', description: '复盘与教师' },
    { key: 'knowledge', label: '路由知识', icon: '⌁', description: '可信知识' },
    { key: 'shadow', label: '影子验证', icon: '◌', description: '候选验证' },
  ] },
  { label: '系统运维', items: [
    { key: 'logs', label: '调用与告警', icon: '◒', description: '调用审计' },
    { key: 'accounts', label: '账号额度', icon: '◷', description: '额度事实' },
    { key: 'snapshots', label: '版本快照', icon: '▱', description: '快照恢复' },
    { key: 'operations', label: '运维控制', icon: '⚙', description: '运行操作' },
    { key: 'system', label: '系统状态', icon: '⌘', description: '诊断与一致性' },
  ] },
]

export const allNavItems = navGroups.flatMap(group => group.items)
