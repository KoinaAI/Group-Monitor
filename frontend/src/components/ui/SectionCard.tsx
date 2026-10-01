import type { ReactNode } from 'react'
import { Card } from '@heroui/react'
import { cn } from '../../lib/cn'

// Titled surface used across pages: header (title + description + right-aligned
// actions) over a content region. Thin wrapper on HeroUI Card so spacing and
// tokens stay consistent everywhere.
export function SectionCard({
  title,
  description,
  actions,
  children,
  footer,
  variant = 'default',
  className,
  contentClassName,
}: {
  title?: ReactNode
  description?: ReactNode
  actions?: ReactNode
  children?: ReactNode
  footer?: ReactNode
  variant?: 'transparent' | 'default' | 'secondary' | 'tertiary'
  className?: string
  contentClassName?: string
}) {
  return (
    <Card variant={variant} className={cn('workspace-card min-w-0 gap-0 rounded-2xl p-4', className)}>
      {title || actions || description ? (
        <Card.Header className="flex flex-row flex-wrap items-start justify-between gap-2">
          <div className="flex min-w-0 flex-col gap-0.5">
            {title ? <Card.Title className="text-base">{title}</Card.Title> : null}
            {description ? <Card.Description>{description}</Card.Description> : null}
          </div>
          {actions ? (
            <div className="flex max-w-full flex-wrap items-center gap-2">{actions}</div>
          ) : null}
        </Card.Header>
      ) : null}
      <Card.Content className={cn(title || actions || description ? 'mt-2.5 min-w-0' : 'min-w-0', contentClassName)}>{children}</Card.Content>
      {footer ? <Card.Footer className="mt-3">{footer}</Card.Footer> : null}
    </Card>
  )
}
