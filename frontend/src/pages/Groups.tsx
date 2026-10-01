import { useMemo, useState } from 'react'
import { Button, Chip, Input, ListBox, TextField } from '@heroui/react'
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
  const [selectedGroupIds, setSelectedGroupIds] = useState<Set<string>>(new Set())
  const [searchOpen, setSearchOpen] = useState(false)
  const [filter, setFilter] = useState<GroupFilter>('all')
  const [changes, setChanges] = useState<Record<number, boolean>>({})
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')
  const rows = (data ?? []).map((row) => ({ ...row, watch: changes[row.groupId] ?? row.watch }))
  const dirtyCount = Object.keys(changes).length
  const watchedCount = rows.filter((row) => row.watch).length
  const search = q.trim().toLowerCase()
  const selectedRows = useMemo(
    () => rows.filter((row) => selectedGroupIds.has(String(row.groupId))),
    [rows, selectedGroupIds],
  )
  const suggestions = useMemo(() => {
    if (!search) return rows
    return rows.filter((row) => [row.groupName, row.groupRemark, String(row.groupId)].some((value) => value?.toLowerCase().includes(search)))
  }, [rows, search])
  const filtered = rows.filter((row) => {
    const matchesSearch = !search || row.groupName?.toLowerCase().includes(search) || row.groupRemark?.toLowerCase().includes(search) || String(row.groupId).includes(search)
    const matchesSelected = selectedGroupIds.size === 0 || selectedGroupIds.has(String(row.groupId))
    return matchesSearch && matchesSelected && (filter === 'all' || (filter === 'watched' ? row.watch : !row.watch))
  })
  const totalMembers = rows.reduce((sum, row) => sum + (row.memberCount || 0), 0)
  const watchRate = rows.length ? Math.round((watchedCount / rows.length) * 100) : 0

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
      <div className="mb-4 grid grid-cols-3 gap-2 sm:gap-3" aria-label="群组摘要">
        <div className="rounded-xl border border-separator/70 bg-surface/70 px-3 py-2.5 sm:px-4">
          <p className="text-[11px] font-medium text-muted">群组总数</p>
          <p className="mt-0.5 text-lg font-semibold tabular-nums text-foreground">{rows.length}</p>
        </div>
        <div className="rounded-xl border border-separator/70 bg-surface/70 px-3 py-2.5 sm:px-4">
          <p className="text-[11px] font-medium text-muted">监听覆盖</p>
          <p className="mt-0.5 text-lg font-semibold tabular-nums text-foreground">{watchRate}<span className="ml-0.5 text-sm font-medium text-muted">%</span></p>
        </div>
        <div className="rounded-xl border border-separator/70 bg-surface/70 px-3 py-2.5 sm:px-4">
          <p className="text-[11px] font-medium text-muted">成员规模</p>
          <p className="mt-0.5 text-lg font-semibold tabular-nums text-foreground">{totalMembers.toLocaleString()}<span className="ml-1 text-sm font-medium text-muted">人</span></p>
        </div>
      </div>
      <Widget>
        <Widget.Header className="flex-wrap gap-y-2 py-2">
          <div className="flex items-center gap-3">
            <Widget.Title>群组列表</Widget.Title>
            <Widget.Description className="tabular-nums">共 {rows.length} 个群 · 已监听 {watchedCount} 个</Widget.Description>
          </div>
          <div className="relative w-full sm:w-80">
            <TextField
              aria-label="搜索群组"
              value={q}
              onChange={(value) => { setQ(value); setSearchOpen(true) }}
              onFocus={() => setSearchOpen(true)}
              onBlur={() => window.setTimeout(() => setSearchOpen(false), 120)}
            >
              <Input placeholder="输入群名或群号，可多选" variant="secondary" />
            </TextField>
            {searchOpen ? <div className="workspace-popover absolute inset-x-0 top-full z-30 mt-1 overflow-hidden rounded-lg border border-border">
              <ListBox aria-label="群组搜索结果" className="max-h-64 overflow-y-auto py-1">
                {suggestions.length === 0 ? <ListBox.Item id="empty" isDisabled textValue="没有匹配的群组">没有匹配的群组</ListBox.Item> : suggestions.map((row) => {
                  const selected = selectedGroupIds.has(String(row.groupId))
                  return <ListBox.Item
                    key={row.groupId}
                    id={String(row.groupId)}
                    textValue={`${row.groupName} ${row.groupRemark} ${row.groupId}`}
                    aria-selected={selected}
                    onPointerDown={(event) => event.preventDefault()}
                    onAction={() => {
                      setSelectedGroupIds((previous) => {
                        const next = new Set(previous)
                        if (selected) next.delete(String(row.groupId))
                        else next.add(String(row.groupId))
                        return next
                      })
                      setQ('')
                    }}
                  >
                    <span className="min-w-0 flex-1">
                      <span className="block truncate">{row.groupName || `群 ${row.groupId}`}</span>
                      <span className="block text-xs text-muted tabular-nums">群号 {row.groupId}{row.groupRemark ? ` · ${row.groupRemark}` : ''}</span>
                    </span>
                    {selected ? <AppIcon name="check" className="size-4 shrink-0 text-accent" /> : null}
                  </ListBox.Item>
                })}
              </ListBox>
            </div> : null}
          </div>
        </Widget.Header>
        {selectedRows.length > 0 ? <div className="flex flex-wrap items-center gap-1.5 border-b border-border/60 px-4 py-2">
          <span className="mr-1 text-xs text-muted">已选群组</span>
          {selectedRows.map((row) => <span key={row.groupId} className="inline-flex items-center gap-0.5">
            <Chip size="sm" variant="soft" color="accent"><Chip.Label>{row.groupName || `群 ${row.groupId}`} · {row.groupId}</Chip.Label></Chip>
            <Button size="sm" variant="ghost" isIconOnly aria-label={`移除 ${row.groupName || row.groupId}`} onPress={() => setSelectedGroupIds((previous) => {
              const next = new Set(previous)
              next.delete(String(row.groupId))
              return next
            })} className="size-5 min-w-5 p-0 text-muted"><AppIcon name="close" className="size-3" /></Button>
          </span>)}
          <Button size="sm" variant="ghost" onPress={() => setSelectedGroupIds(new Set())}>清除选择</Button>
        </div> : null}
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
