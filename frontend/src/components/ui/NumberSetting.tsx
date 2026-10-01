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
  className?: string
}) {
  return (
    <NumberField
      variant="secondary"
      className={cn('flex min-w-0 flex-col gap-1', className)}
      value={value}
      onChange={(v) => onChange(typeof v === 'number' && Number.isFinite(v) ? v : 0)}
      minValue={minValue}
      maxValue={maxValue}
      step={step}
      formatOptions={formatOptions}
      isDisabled={isDisabled}
    >
      <Label>{label}</Label>
      <NumberField.Group className="h-9 w-fit min-w-36 max-w-48">
        <NumberField.DecrementButton className="w-9" />
        <NumberField.Input className="w-16 px-2 text-center" />
        <NumberField.IncrementButton className="w-9" />
      </NumberField.Group>
      {description ? <Description className="text-xs leading-5">{description}</Description> : null}
    </NumberField>
  )
}
