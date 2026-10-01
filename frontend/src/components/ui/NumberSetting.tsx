import type { ReactNode } from 'react'
import { Description, Label, NumberField } from '@heroui/react'
import { cn } from '../../lib/cn'

// Labeled numeric stepper (HeroUI v3 NumberField). The backend contract uses
// whole-number seconds/counts, so onChange coerces the possibly-undefined value
// back to a number.
export function NumberSetting({
  label,
  description,
  value,
  onChange,
  minValue,
  maxValue,
  step = 1,
  formatOptions,
  isDisabled,
  fullWidth = false,
  className,
}: {
  label: ReactNode
  description?: ReactNode
  value: number
  onChange: (v: number) => void
  minValue?: number
  maxValue?: number
  step?: number
  formatOptions?: Intl.NumberFormatOptions
  isDisabled?: boolean
  /** Stretch the stepper to its grid cell when it sits beside text inputs. */
  fullWidth?: boolean
  className?: string
}) {
  return (
    <NumberField
      variant="secondary"
      className={cn('flex min-w-0 flex-col gap-1.5', className)}
      value={value}
      onChange={(v) => onChange(typeof v === 'number' && Number.isFinite(v) ? v : 0)}
      minValue={minValue}
      maxValue={maxValue}
      step={step}
      formatOptions={formatOptions}
      isDisabled={isDisabled}
    >
      <Label>{label}</Label>
      <NumberField.Group className={cn('h-[38px] min-w-36', fullWidth ? 'w-full max-w-none' : 'w-fit max-w-48')}>
        <NumberField.DecrementButton className="w-9" />
        <NumberField.Input className={cn('px-2 text-center', fullWidth ? 'min-w-0 flex-1' : 'w-16')} />
        <NumberField.IncrementButton className="w-9" />
      </NumberField.Group>
      {description ? <Description className="text-xs leading-5">{description}</Description> : null}
    </NumberField>
  )
}
