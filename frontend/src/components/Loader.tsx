import { Spinner } from '@heroui/react'
import { cn } from '../lib/cn'

// Transient centered loader used by route/auth gates and page suspense.
export function Loader({
  label,
  fullscreen = false,
  className,
}: {
  label?: string
  fullscreen?: boolean
  className?: string
}) {
  return (
    <div
      className={cn(
        'grid place-items-center',
        fullscreen ? 'min-h-dvh' : 'min-h-40 w-full',
        className,
      )}
    >
      <div className="flex flex-col items-center gap-3">
        <Spinner size="lg" color="accent" />
        {label ? <p className="text-sm text-muted">{label}</p> : null}
      </div>
    </div>
  )
}
