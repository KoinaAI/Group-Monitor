import type { ReactNode } from 'react'
import { Button } from '@heroui/react'
import { AppIcon, type IconName } from '../../lib/icons'
import { cn } from '../../lib/cn'

// Neutral "nothing here yet" placeholder for empty lists/feeds.
export function EmptyState({
  icon = 'more',
  title,
  description,
  action,
  className,
}: {
  icon?: IconName
  title: string
  description?: string
  action?: ReactNode
  className?: string
}) {
  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center gap-2 px-6 py-12 text-center',
        className,
      )}
    >
      <div className="grid size-11 place-items-center rounded-full bg-default text-muted">
        <AppIcon name={icon} className="size-5" />
      </div>
      <p className="text-sm font-medium text-foreground">{title}</p>
      {description ? <p className="max-w-sm text-xs text-muted">{description}</p> : null}
      {action ? <div className="mt-1">{action}</div> : null}
    </div>
  )
}

// Load-failure placeholder with an optional retry affordance.
export function InlineError({
  message,
  onRetry,
  className,
}: {
  message: string
  onRetry?: () => void
  className?: string
}) {
  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center gap-3 px-6 py-10 text-center',
        className,
      )}
    >
      <div className="grid size-11 place-items-center rounded-full bg-danger-soft text-danger-soft-foreground">
        <AppIcon name="disconnected" className="size-5" />
      </div>
      <p className="text-sm text-foreground">{message}</p>
      {onRetry ? (
        <Button size="sm" variant="secondary" onPress={onRetry}>
          <AppIcon name="refresh" className="size-4" />
          重试
        </Button>
      ) : null}
    </div>
  )
}
