import { useState } from 'react'
import { Button, Input, Label, ListBox, Select, TextField } from '@heroui/react'
import { AppIcon } from '../../lib/icons'
import { overrideLevelLabel } from '../../lib/labels'
import type { SenderOverride, SenderOverrideLevel } from '../../lib/types'

const LEVELS: SenderOverrideLevel[] = ['vip', 'normal', 'muted']

// Per-member level overrides: a QQ-number add row plus a list of editable rows
// (note field + level Select + remove). Forces 重点/正常/屏蔽 for that sender.
export function OverrideEditor({
  value,
  onChange,
}: {
  value: SenderOverride[]
  onChange: (v: SenderOverride[]) => void
}) {
  const [qid, setQid] = useState('')
  const add = () => {
    const id = Number(qid.trim())
    if (!id || value.some((o) => o.userId === id)) {
      setQid('')
      return
    }
    onChange([...value, { userId: id, note: '', level: 'vip' }])
    setQid('')
  }
  const update = (userId: number, p: Partial<SenderOverride>) =>
    onChange(value.map((o) => (o.userId === userId ? { ...o, ...p } : o)))
  const remove = (userId: number) => onChange(value.filter((o) => o.userId !== userId))

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-end gap-2">
        <TextField aria-label="QQ 号" value={qid} onChange={setQid} className="flex-1">
          <Input placeholder="QQ 号码" variant="secondary" inputMode="numeric" />
        </TextField>
        <Button variant="secondary" onPress={add} isDisabled={!qid.trim()}>
          <AppIcon name="add" className="size-4" />
          添加
        </Button>
      </div>
      {value.length ? (
        <ul className="flex flex-col divide-y divide-border">
          {value.map((o) => (
            <li key={o.userId} className="flex items-center gap-2 py-2.5 first:pt-0 last:pb-0">
              <span className="w-24 shrink-0 truncate text-sm tabular-nums text-foreground">
                {o.userId}
              </span>
              <TextField
                aria-label="备注"
                value={o.note}
                onChange={(v) => update(o.userId, { note: v })}
                className="flex-1"
              >
                <Input placeholder="备注（可选）" variant="secondary" />
              </TextField>
              <Select
                aria-label="级别"
                variant="secondary"
                selectionMode="single"
                value={o.level}
                onChange={(v) => {
                  if (v != null) update(o.userId, { level: String(v) as SenderOverrideLevel })
                }}
                className="w-28"
              >
                <Select.Trigger>
                  <Select.Value />
                  <Select.Indicator />
                </Select.Trigger>
                <Select.Popover>
                  <ListBox>
                    {LEVELS.map((lv) => (
                      <ListBox.Item key={lv} id={lv} textValue={overrideLevelLabel[lv]}>
                        <Label>{overrideLevelLabel[lv]}</Label>
                      </ListBox.Item>
                    ))}
                  </ListBox>
                </Select.Popover>
              </Select>
              <Button
                size="sm"
                variant="danger-soft"
                isIconOnly
                aria-label="移除"
                onPress={() => remove(o.userId)}
              >
                <AppIcon name="remove" className="size-4" />
              </Button>
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-xs text-muted">可为特定成员强制设定重点 / 正常 / 屏蔽级别。</p>
      )}
    </div>
  )
}
