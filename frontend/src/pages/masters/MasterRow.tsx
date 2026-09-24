import { Avatar, Button, Label, ListBox, Select } from '@heroui/react'
import { AppIcon } from '../../lib/icons'
import { masterKindLabel, minLevelLabel } from '../../lib/labels'
import { IntentChip } from '../../components/ui/IntentChip'
import { userAvatar } from '../../lib/qlogo'
import type { Master, MinLevel } from '../../lib/types'

const LEVELS: MinLevel[] = [0, 1, 2, 3]

// One master: QQ avatar, identity, a notification-gate Select
// (0 all → 3 urgent only), and a remove control. A notify-only master (share
// target, no login authority) is tagged so the two kinds are told apart.
export function MasterRow({
  master,
  onLevel,
  onRemove,
}: {
  master: Master
  onLevel: (lv: MinLevel) => void
  onRemove: () => void
}) {
  const notify = master.kind === 'notify'
  return (
    <li className="flex items-center gap-3 py-3 first:pt-0 last:pb-0">
      <Avatar size="md" className="shrink-0">
        <Avatar.Image src={userAvatar(master.userId)} alt={master.nickname} loading="lazy" />
        <Avatar.Fallback>{(master.nickname || 'Q').slice(0, 1)}</Avatar.Fallback>
      </Avatar>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <p className="truncate text-sm font-medium text-foreground">
            {master.nickname || `用户 ${master.userId}`}
          </p>
          <IntentChip intent={notify ? 'secondary' : 'primary'}>
            {masterKindLabel[notify ? 'notify' : 'full']}
          </IntentChip>
        </div>
        <p className="text-xs text-muted tabular-nums">{master.userId}</p>
      </div>
      <Select
        aria-label="通知级别"
        variant="secondary"
        selectionMode="single"
        value={master.minLevel}
        onChange={(v) => {
          if (v != null) onLevel(Number(v) as MinLevel)
        }}
        className="w-36"
      >
        <Select.Trigger>
          <Select.Value />
          <Select.Indicator />
        </Select.Trigger>
        <Select.Popover>
          <ListBox>
            {LEVELS.map((lv) => (
              <ListBox.Item key={lv} id={lv} textValue={minLevelLabel[lv]}>
                <Label>{minLevelLabel[lv]}</Label>
              </ListBox.Item>
            ))}
          </ListBox>
        </Select.Popover>
      </Select>
      <Button size="sm" variant="danger-soft" isIconOnly aria-label="移除主人" onPress={onRemove}>
        <AppIcon name="remove" className="size-4" />
      </Button>
    </li>
  )
}
