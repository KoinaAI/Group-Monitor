import type { ReactNode } from 'react'
import { Description, Label, Switch } from '@heroui/react'

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
      <Switch.Content>
        <Switch.Control>
          <Switch.Thumb />
        </Switch.Control>
        <div className="flex flex-col gap-0.5">
          <Label>{label}</Label>
          {description ? <Description>{description}</Description> : null}
        </div>
      </Switch.Content>
    </Switch>
  )
}
