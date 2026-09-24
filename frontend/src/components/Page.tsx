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
        'mx-auto w-full max-w-6xl px-4 py-6 sm:px-6 sm:py-8',
        className,
      )}
    >
      {children}
    </div>
  )
}
