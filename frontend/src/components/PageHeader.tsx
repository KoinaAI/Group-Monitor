import type { ReactNode } from 'react'

// Page title block with optional description and right-aligned actions slot.
// Plain tokens (no HeroUI component) so it stays layout-only; interactive
// controls belong in `actions`.
export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string
  description?: string
  actions?: ReactNode
}) {
  return (
    <header className="mb-5 flex flex-wrap items-start justify-between gap-3">
      <div className="min-w-0">
        <h1 className="text-xl font-semibold tracking-tight text-foreground">
          {title}
        </h1>
        {description && (
          <p className="mt-1 text-sm text-muted">{description}</p>
        )}
      </div>
      {actions && (
        <div className="flex max-w-full flex-wrap items-center gap-2">{actions}</div>
      )}
    </header>
  )
}
