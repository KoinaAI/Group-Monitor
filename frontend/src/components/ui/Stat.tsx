import type { ReactNode } from 'react'
import { KPI } from '@heroui-pro/react/kpi'
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
    <KPI className="min-w-0">
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <p className="truncate text-xs text-muted">{label}</p>
          <p className="mt-1 text-xl font-semibold leading-tight tabular-nums tracking-tight text-foreground">{value}</p>
          {hint ? <p className="mt-1 truncate text-xs text-muted">{hint}</p> : null}
        </div>
        <div
          className={cn(
            'grid size-8 shrink-0 place-items-center rounded-lg',
            iconTone[tone],
          )}
        >
          <AppIcon name={icon} className="size-4" />
        </div>
      </div>
    </KPI>
  )
}
