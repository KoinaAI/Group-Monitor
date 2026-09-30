import { useState } from 'react'
import { Button, Input, TextField } from '@heroui/react'
import { ActionBar, ListView, Segment, Widget } from '@heroui-pro/react'
import { Page } from '../components/Page'
import { PageHeader } from '../components/PageHeader'
import { EmptyState, InlineError } from '../components/ui/States'
import { Loader } from '../components/Loader'
import { api, ApiError } from '../lib/api'
import { AppIcon } from '../lib/icons'
import { useApi } from '../lib/useApi'
import { GroupRowItem } from './groups/GroupRowItem'

type GroupFilter = 'all' | 'watched' | 'unwatched'

// Keep unsaved watch changes separate from server data so they can be discarded
// and a switch returned to its original value no longer counts as a change.
export default function Groups() {
  const { data, error, loading, reload, setData } = useApi(api.groups.list, [])
  const [q, setQ] = useState('')
  const [filter, setFilter] = useState<GroupFilter>('all')
  const [changes, setChanges] = useState<Record<number, boolean>>({})
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')
  const rows = (data ?? []).map((row) => ({ ...row, watch: changes[row.groupId] ?? row.watch }))
  const dirtyCount = Object.keys(changes).length
  const watchedCount = rows.filter((row) => row.watch).length
  const search = q.trim().toLowerCase()
  const filtered = rows.filter((row) => {
    const matchesSearch = !search || row.groupName?.toLowerCase().includes(search) || row.groupRemark?.toLowerCase().includes(search) || String(row.groupId).includes(search)
    return matchesSearch && (filter === 'all' || (filter === 'watched' ? row.watch : !row.watch))
  })

  const setWatch = (groupId: number, value: boolean) => {
    setChanges((previous) => {
      const next = { ...previous }
      if (data?.find((row) => row.groupId === groupId)?.watch === value) delete next[groupId]
      else next[groupId] = value
      return next
    })
    setSaveError('')
  }

  const save = async () => {
    setSaving(true)
    setSaveError('')
    try {
      await api.groups.saveWatch(rows.map((row) => ({ groupId: row.groupId, groupName: row.groupName, watch: row.watch })))
      setData(() => rows)
      setChanges({})
    } catch (e) {
      setSaveError(e instanceof ApiError ? e.message : '保存失败，请重试')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Page>
      <PageHeader
        title="群组"
        description="集中管理监听范围，查看群组历史消息。"
        actions={<Button variant="secondary" size="sm" onPress={reload} isDisabled={loading || saving || dirtyCount > 0}><AppIcon name="refresh" className="size-4" />刷新群组</Button>}
      />
      <Widget>
        <Widget.Header className="flex-wrap gap-y-2 py-2">
          <div className="flex items-center gap-3">
            <Widget.Title>群组列表</Widget.Title>
            <Widget.Description className="tabular-nums">共 {rows.length} 个群 · 已监听 {watchedCount} 个</Widget.Description>
          </div>
          <TextField aria-label="搜索群组" value={q} onChange={setQ} className="w-full sm:w-64">
            <Input placeholder="搜索群名、备注或群号" variant="secondary" />
          </TextField>
        </Widget.Header>
        <Widget.Content className="!p-0">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border/60 px-4 py-3">
            <Segment aria-label="群组监听筛选" size="sm" selectedKey={filter} onSelectionChange={(key) => setFilter(key as GroupFilter)}>
              <Segment.Item id="all">全部</Segment.Item>
              <Segment.Item id="watched">已监听 <span className="ml-1 tabular-nums text-muted">{watchedCount}</span></Segment.Item>
              <Segment.Item id="unwatched">未监听 <span className="ml-1 tabular-nums text-muted">{rows.length - watchedCount}</span></Segment.Item>
            </Segment>
            <span className="text-xs tabular-nums text-muted">显示 {filtered.length} 个群组</span>
          </div>
          {loading ? <Loader label="正在加载群组…" /> : error ? (
            <InlineError message={error} onRetry={reload} />
          ) : filtered.length === 0 ? (
            <EmptyState icon="groups" title={rows.length === 0 ? '暂无群组' : '未找到匹配的群组'} description={rows.length === 0 ? '请确认机器人已加入群聊' : '调整搜索内容或监听筛选后重试。'} />
          ) : (
            <ListView aria-label="群组列表" variant="secondary" className="pb-1">
              {filtered.map((row) => (
                <GroupRowItem key={row.groupId} row={row} watch={row.watch} onWatch={(value) => setWatch(row.groupId, value)} isDisabled={saving} />
              ))}
            </ListView>
          )}
        </Widget.Content>
      </Widget>
      {saveError ? <InlineError message={saveError} /> : null}
      {dirtyCount > 0 ? <div className="h-16" aria-hidden /> : null}
      <ActionBar isOpen={dirtyCount > 0} aria-label="群组监听更改">
        <ActionBar.Prefix><span className="text-sm tabular-nums">{dirtyCount} 项未保存</span></ActionBar.Prefix>
        <ActionBar.Content>
          <Button variant="ghost" size="sm" isDisabled={saving} onPress={() => { setChanges({}); setSaveError('') }}>放弃更改</Button>
          <Button size="sm" onPress={save} isPending={saving}>保存更改</Button>
        </ActionBar.Content>
      </ActionBar>
    </Page>
  )
}
