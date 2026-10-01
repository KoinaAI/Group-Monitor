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
        'workspace-page mx-auto w-full min-w-0 max-w-[1320px] px-4 py-5 sm:px-6 sm:py-6',
        className,
      )}
    >
      {children}
    </div>
  )
}
