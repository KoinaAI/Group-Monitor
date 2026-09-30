import { Description } from '@heroui/react'
import { CellSwitch } from '@heroui-pro/react'

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
    <CellSwitch
      aria-label={label}
      isSelected={isSelected}
      isDisabled={isDisabled}
      onChange={onChange}
      variant="secondary"
      className="w-full gap-1 bg-transparent shadow-none"
    >
      <CellSwitch.Trigger className="w-full justify-between rounded-none bg-transparent px-0 shadow-none">
        <CellSwitch.Label>{label}</CellSwitch.Label>
        <CellSwitch.Control />
      </CellSwitch.Trigger>
      <Description className="max-w-none text-xs leading-relaxed">{description}</Description>
    </CellSwitch>
  )
}
