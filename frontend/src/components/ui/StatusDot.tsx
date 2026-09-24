import { cn } from '../../lib/cn'

// Small connection/health indicator. `pulse` adds a ping halo for "live" states.
type Tone = 'success' | 'danger' | 'warning' | 'accent' | 'muted'

const dot: Record<Tone, string> = {
  success: 'bg-success',
  danger: 'bg-danger',
  warning: 'bg-warning',
  accent: 'bg-accent',
  muted: 'bg-muted',
}

export function StatusDot({
  tone = 'muted',
  pulse = false,
  className,
}: {
  tone?: Tone
  pulse?: boolean
  className?: string
}) {
  return (
    <span className={cn('relative inline-flex size-2 shrink-0', className)} aria-hidden>
      {pulse ? (
        <span
          className={cn(
            'absolute inline-flex size-full animate-ping rounded-full opacity-75',
            dot[tone],
          )}
        />
      ) : null}
      <span className={cn('relative inline-flex size-2 rounded-full', dot[tone])} />
    </span>
  )
}
