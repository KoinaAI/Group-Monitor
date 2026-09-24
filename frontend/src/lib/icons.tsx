import { addCollection, Icon } from '@iconify/react'
import gravitySubset from './gravity-subset.json'
import type { ComponentProps } from 'react'

// Register a trimmed Gravity UI set once, at module load, so icons resolve
// locally with no Iconify API round-trip — this dashboard is self-hosted and
// must render offline. The subset (src/lib/gravity-subset.json) is generated
// from the ICONS map below by scripts/gen-icons.mjs; re-run it after adding an
// icon here. Shipping only the ~40 icons we use instead of the full ~1500-icon
// collection keeps the bundle small.
addCollection(gravitySubset as Parameters<typeof addCollection>[0])

// Semantic name → Gravity UI icon id. Keeping every icon reference in one place
// makes a restyle or icon-set swap a single-file change. All ids are verified
// present in @iconify-json/gravity-ui.
export const ICONS = {
  // Navigation.
  overview: 'chart-column',
  groups: 'comments',
  logs: 'list-ul',
  masters: 'persons',
  rules: 'sliders-vertical',
  intelligence: 'sparkles',
  connection: 'plug-connection',

  // Status.
  connected: 'circle-check',
  disconnected: 'circle-xmark',
  running: 'circle-play',
  paused: 'circle-pause',
  enable: 'play',
  pause: 'pause',

  // Actions & affordances.
  escalation: 'triangle-exclamation',
  urgent: 'bell-dot',
  bell: 'bell',
  send: 'paper-plane',
  refresh: 'arrows-rotate-right',
  add: 'plus',
  remove: 'trash-bin',
  clock: 'clock',
  search: 'magnifier',
  menu: 'bars',
  image: 'picture',
  at: 'at',
  logout: 'arrow-right-from-square',
  key: 'key',
  lock: 'lock',
  mail: 'envelope',
  chevronRight: 'chevron-right',
  chevronDown: 'chevron-down',
  close: 'xmark',
  check: 'check',
  watch: 'eye',
  muted: 'volume-xmark',
  ban: 'ban',
  model: 'cpu',
  gate: 'shield-check',
  filter: 'funnel',
  linkOff: 'link-slash',
  tag: 'tag',
  more: 'ellipsis',
  file: 'file',
  fileText: 'file-text',
  fileDoc: 'file-letter-w',
  fileXls: 'file-letter-x',
  filePpt: 'file-letter-p',
  fileZip: 'file-zipper',
  fileCode: 'file-code',
  download: 'file-arrow-down',
  play: 'play-fill',
  back: 'arrow-left',

  // Theme switcher.
  sun: 'sun',
  moon: 'moon',
  system: 'display',
} as const

export type IconName = keyof typeof ICONS

type AppIconProps = Omit<ComponentProps<typeof Icon>, 'icon'> & { name: IconName }

// Semantic wrapper: <AppIcon name="urgent" className="size-5" />.
export function AppIcon({ name, ...props }: AppIconProps) {
  return <Icon icon={`gravity-ui:${ICONS[name]}`} {...props} />
}

// Escape hatch for a one-off raw Iconify id not worth a semantic key.
export { Icon }
