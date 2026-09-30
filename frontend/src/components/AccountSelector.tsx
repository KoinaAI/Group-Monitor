import { useEffect, useState } from 'react'
import { Label, ListBox, Select } from '@heroui/react'
import { activeAccount, selectAccount } from '../lib/accounts'
import { api } from '../lib/api'
import type { SourceConfig } from '../lib/types'

export function AccountSelector() {
  const [sources, setSources] = useState<SourceConfig[]>([])
  useEffect(() => {
    api.sources.list().then((value) => {
      setSources(value)
      if (activeAccount() && !value.some((s) => s.accounts.some((a) => a.id === activeAccount()))) selectAccount('')
    }).catch(() => {})
  }, [])
  return <Select aria-label="当前信息源账号" className="w-40 sm:w-56" variant="secondary" selectionMode="single" value={activeAccount() || '__none'} onChange={(id) => selectAccount(id === '__none' ? '' : String(id))}>
    <Select.Trigger><Select.Value className="truncate" /><Select.Indicator /></Select.Trigger>
    <Select.Popover><ListBox>
      <ListBox.Item id="__none" textValue="未选择账号"><Label>未选择账号</Label></ListBox.Item>
      {sources.flatMap((s) => s.accounts.map((a) => <ListBox.Item key={a.id} id={a.id} textValue={`${s.name} / ${a.name}`}><Label>{s.name} / {a.name}</Label></ListBox.Item>))}
    </ListBox></Select.Popover>
  </Select>
}
