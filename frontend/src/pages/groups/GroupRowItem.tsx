import { Avatar, Button, Switch } from '@heroui/react'
import { useNavigate } from 'react-router-dom'
import { AppIcon } from '../../lib/icons'
import { groupAvatar } from '../../lib/qlogo'
import type { GroupRow } from '../../lib/types'

// One group row: avatar + identity, a watch toggle, and a 历史 button that opens
// the dedicated history route (/groups/:groupId/history). The group name/remark
// ride along in router state so the history page can render its header without
// re-fetching the whole group list.
export function GroupRowItem({
  row,
  watch,
  onWatch,
}: {
  row: GroupRow
  watch: boolean
  onWatch: (v: boolean) => void
}) {
  const navigate = useNavigate()

  const openHistory = () =>
    navigate(`/groups/${row.groupId}/history`, {
      state: {
        groupName: row.groupName,
        groupRemark: row.groupRemark,
        memberCount: row.memberCount,
      },
    })

  return (
    <li className="py-3 first:pt-0 last:pb-0">
      <div className="flex items-center gap-3">
        <Avatar size="md" className="shrink-0">
          <Avatar.Image
            src={groupAvatar(row.groupId)}
            alt={row.groupName || String(row.groupId)}
            loading="lazy"
          />
          <Avatar.Fallback>{(row.groupName || 'Q').slice(0, 1)}</Avatar.Fallback>
        </Avatar>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate text-sm font-medium text-foreground">
              {row.groupName || row.groupId}
            </span>
            {row.groupRemark ? (
              <span className="truncate text-xs text-muted">{row.groupRemark}</span>
            ) : null}
          </div>
          <p className="mt-0.5 text-xs text-muted tabular-nums">
            {row.memberCount} 名成员 · {row.groupId}
          </p>
        </div>
        <Button size="sm" variant="tertiary" onPress={openHistory}>
          <AppIcon name="clock" className="size-4" />
          历史
        </Button>
        <Switch isSelected={watch} onChange={onWatch} size="sm" aria-label="监听该群">
          <Switch.Content>
            <Switch.Control>
              <Switch.Thumb />
            </Switch.Control>
          </Switch.Content>
        </Switch>
      </div>
    </li>
  )
}
