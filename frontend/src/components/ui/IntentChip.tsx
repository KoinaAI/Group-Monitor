import type { ReactNode } from 'react'
import { Chip } from '@heroui/react'
import type { Intent } from '../../lib/labels'

// labels.ts encodes intents in the older color vocabulary (primary/secondary…);
// HeroUI v3 Chip colors are default|accent|success|warning|danger. Translate
// in one place so every status/level tag stays on the v3 palette.
type ChipColor = 'default' | 'accent' | 'success' | 'warning' | 'danger'
const toColor: Record<Intent, ChipColor> = {
  default: 'default',
  primary: 'accent',
  secondary: 'default',
  success: 'success',
  warning: 'warning',
  danger: 'danger',
}

export function IntentChip({
  intent = 'default',
  children,
  size = 'sm',
  variant = 'soft',
}: {
  intent?: Intent
  children: ReactNode
  size?: 'sm' | 'md'
  variant?: 'soft' | 'primary' | 'secondary' | 'tertiary'
}) {
  return (
    <Chip color={toColor[intent]} variant={variant} size={size}>
      <Chip.Label>{children}</Chip.Label>
    </Chip>
  )
}
