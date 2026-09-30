import type { ReactNode } from 'react'
import { Description, Switch } from '@heroui/react'

// Labeled toggle built on the confirmed HeroUI v3 Switch anatomy
// (Switch.Content > Switch.Control > Switch.Thumb, label/description after the
// control). Used for every boolean setting across the config pages.
export function Toggle({
  label,
  description,
  isSelected,
  onChange,
  isDisabled,
  size = 'md',
}: {
  label: ReactNode
  description?: ReactNode
  isSelected: boolean
  onChange: (v: boolean) => void
  isDisabled?: boolean
  size?: 'sm' | 'md' | 'lg'
}) {
  return (
    <Switch isSelected={isSelected} onChange={onChange} isDisabled={isDisabled} size={size}>
      <Switch.Content className="w-full justify-between gap-3">
        <div className="flex flex-col gap-0.5">
          <span className="text-sm font-medium text-foreground">{label}</span>
        </div>
        <Switch.Control><Switch.Thumb /></Switch.Control>
      </Switch.Content>
      {description ? <Description className="max-w-prose text-xs">{description}</Description> : null}
    </Switch>
  )
}
