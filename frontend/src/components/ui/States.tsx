import type { ReactNode } from 'react'
import { Button } from '@heroui/react'
import { EmptyState as ProEmptyState } from '@heroui-pro/react'
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
    <ProEmptyState size="sm" className={cn('px-4 py-6', className)}>
      <ProEmptyState.Header>
        <ProEmptyState.Media variant="icon"><AppIcon name={icon} className="size-5 text-muted" /></ProEmptyState.Media>
        <ProEmptyState.Title className="text-sm">{title}</ProEmptyState.Title>
        {description ? <ProEmptyState.Description className="max-w-sm text-xs">{description}</ProEmptyState.Description> : null}
      </ProEmptyState.Header>
      {action ? <ProEmptyState.Content>{action}</ProEmptyState.Content> : null}
    </ProEmptyState>
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
        'flex flex-col items-center justify-center gap-3 px-4 py-6 text-center',
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
