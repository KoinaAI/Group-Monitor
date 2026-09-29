import type { IconName } from '../lib/icons'

// Sidebar navigation as data. One source of truth for routes + labels + icons,
// so the router (App.tsx) and the Sidebar shell never drift apart. Labels are
// Chinese to match the operator-facing dashboard; `to` values are the exact
// route paths registered in App.tsx.
export interface NavItem {
  label: string
  to: string
  icon: IconName
  /** Optional one-line hint shown as a tooltip / description where supported. */
  hint?: string
}

export interface NavSection {
  /** Section heading rendered above its items. */
  title: string
  items: NavItem[]
}

export const NAV: NavSection[] = [
  {
    title: '监控',
    items: [
      { label: '总览', to: '/', icon: 'overview', hint: '运行状态与实时动态' },
      { label: '群组', to: '/groups', icon: 'groups', hint: '监听群与聊天记录' },
      { label: '通知归档', to: '/notices', icon: 'fileText', hint: '检索已保留的正式通知' },
      { label: '日志', to: '/logs', icon: 'logs', hint: '流水与升级事件' },
    ],
  },
  {
    title: '配置',
    items: [
      { label: '主人', to: '/masters', icon: 'masters', hint: '通知接收人与等级' },
      { label: '规则', to: '/rules', icon: 'rules', hint: '静默窗口与升级条件' },
      { label: '智能', to: '/intelligence', icon: 'intelligence', hint: 'LLM 提炼与意图闸门' },
      { label: '存储', to: '/storage', icon: 'download', hint: '定期备份通知归档' },
      { label: '连接', to: '/connection', icon: 'connection', hint: 'OneBot 接入' },
    ],
  },
]

// Flat lookup for the header title / breadcrumb: path → label.
export const NAV_TITLE: Record<string, string> = Object.fromEntries(
  NAV.flatMap((s) => s.items).map((i) => [i.to, i.label]),
)
