import type { IconName } from '../lib/icons'

export interface NavItem {
  label: string
  to: string
  icon: IconName
  hint?: string
}
export interface NavSection { title: string; items: NavItem[] }

// Full route directory powers contextual tabs and the command palette.
export const NAV: NavSection[] = [
  { title: '工作台', items: [
    { label: '总览', to: '/', icon: 'overview', hint: '运行状态与实时动态' },
    { label: '群组', to: '/groups', icon: 'groups', hint: '监听群与聊天记录' },
    { label: '通知归档', to: '/notices', icon: 'fileText', hint: '检索已保留的正式通知' },
    { label: '运行日志', to: '/logs', icon: 'logs', hint: '流水与升级事件' },
  ] },
  { title: '通知与接收人', items: [
    { label: '广播通知', to: '/notifications', icon: 'send', hint: 'ntfy 与 Bark 通知目标' },
    { label: '主人', to: '/masters', icon: 'masters', hint: 'QQ 接收人与通知等级' },
  ] },
  { title: '处理策略', items: [
    { label: '规则', to: '/rules', icon: 'rules', hint: '聚合窗口与升级条件' },
    { label: '智能', to: '/intelligence', icon: 'intelligence', hint: '模型提炼、意图判断与附件阅读' },
  ] },
  { title: '连接与数据', items: [
    { label: '信息源', to: '/sources', icon: 'connection', hint: '渠道与账号连接' },
    { label: 'Agent 接入', to: '/agents', icon: 'lock', hint: 'Skill、MCP 与只读 API Key' },
    { label: '存储与备份', to: '/storage', icon: 'download', hint: '定期备份通知归档' },
  ] },
]

export const NAV_TITLE = Object.fromEntries(NAV.flatMap((section) => section.items).map((item) => [item.to, item.label]))
export function navSection(path: string) {
  if (path === '/connection') return NAV[3]
  if (path.startsWith('/groups/')) return NAV[0]
  return NAV.find((section) => section.items.some((item) => item.to === path))
}
export const SIDEBAR_NAV: NavSection[] = [NAV[0], { title: '配置中心', items: [
  { label: '通知与接收人', to: '/notifications', icon: 'send' },
  { label: '处理策略', to: '/rules', icon: 'rules' },
  { label: '连接与数据', to: '/sources', icon: 'connection' },
] }]
