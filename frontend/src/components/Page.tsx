import type { ReactNode } from 'react'
import { cn } from '../lib/cn'

// Standard routed-page container: centered column with responsive gutters. Pages
// compose <Page><PageHeader/>…</Page> so spacing stays consistent across routes.
export function Page({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  return (
    <div
      className={cn(
        'workspace-page mx-auto w-full min-w-0 px-4 sm:px-7 lg:px-8',
        className,
      )}
    >
      {children}
    </div>
  )
}
