import { Description, Switch } from '@heroui/react'

export function EnableSetting({
  label,
  description,
  isSelected,
  isDisabled,
  onChange,
}: {
  label: string
  description: string
  isSelected: boolean
  isDisabled?: boolean
  onChange: (enabled: boolean) => void
}) {
  return (
    <Switch
      aria-label={label}
      isSelected={isSelected}
      isDisabled={isDisabled}
      onChange={onChange}
      size="sm"
      className="w-full gap-1"
    >
      <Switch.Content className="w-full justify-between gap-3">
        <span className="text-sm font-medium text-foreground">{label}</span>
        <Switch.Control><Switch.Thumb /></Switch.Control>
      </Switch.Content>
      <Description className="max-w-none text-xs leading-5" style={{ paddingInlineStart: 0 }}>{description}</Description>
    </Switch>
  )
}
