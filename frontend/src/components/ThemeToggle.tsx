import { Segment } from '@heroui-pro/react'
import { AppIcon, type IconName } from '../lib/icons'
import { useTheme, type ThemePref } from '../lib/theme'

// Segmented light / dark / follow-system control for the navbar. Pro's Segment
// is HeroUI's own idiom for this (it ships a theme-switcher example); the
// selected key is the stored preference, not the resolved theme, so "system"
// stays selected regardless of the current OS mode.
const ITEMS: { id: ThemePref; label: string; icon: IconName }[] = [
  { id: 'light', label: '亮色', icon: 'sun' },
  { id: 'dark', label: '暗色', icon: 'moon' },
  { id: 'system', label: '跟随系统', icon: 'system' },
]

export function ThemeToggle() {
  const { pref, setPref } = useTheme()
  return (
    <Segment
      aria-label="主题"
      size="sm"
      className="gap-0"
      selectedKey={pref}
      onSelectionChange={(key) => setPref(key as ThemePref)}
    >
      {ITEMS.map((it) => (
        <Segment.Item key={it.id} id={it.id} aria-label={it.label} className="size-7 px-0">
          <AppIcon name={it.icon} className="size-4" />
        </Segment.Item>
      ))}
    </Segment>
  )
}
