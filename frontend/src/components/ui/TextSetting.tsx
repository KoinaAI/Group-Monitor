import type { ReactNode } from 'react'
import { Description, Input, Label, TextField } from '@heroui/react'
import { cn } from '../../lib/cn'

// Labeled single-line text field. Wraps the HeroUI TextField anatomy
// (Label + secondary Input + optional Description) for use in config forms.
export function TextSetting({
  label,
  description,
  value,
  onChange,
  placeholder,
  type = 'text',
  inputMode,
  isDisabled,
  className,
}: {
  label: ReactNode
  description?: ReactNode
  value: string
  onChange: (v: string) => void
  placeholder?: string
  type?: 'text' | 'password' | 'url'
  inputMode?: 'text' | 'numeric' | 'url'
  isDisabled?: boolean
  className?: string
}) {
  return (
    <TextField
      value={value}
      onChange={onChange}
      type={type}
      inputMode={inputMode}
      isDisabled={isDisabled}
      className={cn('flex flex-col gap-1.5', className)}
    >
      <Label>{label}</Label>
      <Input variant="secondary" placeholder={placeholder} />
      {description ? <Description>{description}</Description> : null}
    </TextField>
  )
}
