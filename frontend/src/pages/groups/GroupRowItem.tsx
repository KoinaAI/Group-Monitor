import { Avatar, Button, Switch, Tooltip } from '@heroui/react'
import { ListView } from '@heroui-pro/react'
import { useNavigate } from 'react-router-dom'
import { AppIcon } from '../../lib/icons'
import { groupAvatar } from '../../lib/qlogo'
import type { GroupRow } from '../../lib/types'

export function GroupRowItem({ row, watch, onWatch, isDisabled }: {
  row: GroupRow
  watch: boolean
  onWatch: (value: boolean) => void
  isDisabled?: boolean
}) {
  const navigate = useNavigate()
  const openHistory = () => navigate(`/groups/${row.groupId}/history`, {
    state: { groupName: row.groupName, groupRemark: row.groupRemark, memberCount: row.memberCount },
  })

  return (
    <ListView.Item id={row.groupId} textValue={row.groupName || String(row.groupId)} className="gap-3 !py-2.5">
      <Avatar size="sm" className="shrink-0">
        <Avatar.Image src={groupAvatar(row.groupId)} alt={row.groupName || String(row.groupId)} loading="lazy" />
        <Avatar.Fallback>{(row.groupName || 'Q').slice(0, 1)}</Avatar.Fallback>
      </Avatar>
      <ListView.ItemContent className="!block">
        <div className="flex min-w-0 items-center gap-2">
          <ListView.Title>{row.groupName || row.groupId}</ListView.Title>
          {row.groupRemark ? <span className="hidden truncate text-xs text-muted sm:inline">{row.groupRemark}</span> : null}
        </div>
        <ListView.Description className="block tabular-nums">{row.groupId} · {row.memberCount} 名成员</ListView.Description>
      </ListView.ItemContent>
      <ListView.ItemAction className="flex items-center gap-2 sm:gap-3">
        <Tooltip>
          <Button size="sm" variant="secondary" onPress={openHistory} aria-label="历史" className="gap-1.5">
            <AppIcon name="clock" className="size-4" />
            <span className="hidden sm:inline">历史记录</span>
          </Button>
          <Tooltip.Content>查看历史记录</Tooltip.Content>
        </Tooltip>
        <Switch isSelected={watch} onChange={onWatch} isDisabled={isDisabled} size="sm" aria-label="监听该群">
          <Switch.Content><Switch.Control><Switch.Thumb /></Switch.Control></Switch.Content>
        </Switch>
      </ListView.ItemAction>
    </ListView.Item>
  )
}
