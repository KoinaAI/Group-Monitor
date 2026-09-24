import type { ReactNode } from 'react'
import { Card } from '@heroui/react'
import { AppIcon, type IconName } from '../../lib/icons'
import { cn } from '../../lib/cn'

type Tone = 'accent' | 'success' | 'warning' | 'danger' | 'default'
const iconTone: Record<Tone, string> = {
  accent: 'bg-accent-soft text-accent',
  success: 'bg-success-soft text-success-soft-foreground',
  warning: 'bg-warning-soft text-warning-soft-foreground',
  danger: 'bg-danger-soft text-danger-soft-foreground',
  default: 'bg-default text-muted',
}

// Compact KPI tile: icon badge + label + value (+ optional hint line).
export function Stat({
  label,
  value,
  icon,
  tone = 'default',
  hint,
}: {
  label: string
  value: ReactNode
  icon: IconName
  tone?: Tone
  hint?: ReactNode
}) {
  return (
    <Card variant="default">
      <div className="flex items-center gap-3">
        <div
          className={cn(
            'grid size-10 shrink-0 place-items-center rounded-xl',
            iconTone[tone],
          )}
        >
          <AppIcon name={icon} className="size-5" />
        </div>
        <div className="min-w-0">
          <p className="truncate text-xs text-muted">{label}</p>
          <p className="text-xl font-semibold tabular-nums text-foreground">{value}</p>
          {hint ? <p className="truncate text-xs text-muted">{hint}</p> : null}
        </div>
      </div>
    </Card>
  )
}
