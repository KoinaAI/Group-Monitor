import { useState } from 'react'
import { Button, Input, TextField } from '@heroui/react'
import { Page } from '../components/Page'
import { PageHeader } from '../components/PageHeader'
import { SectionCard } from '../components/ui/SectionCard'
import { EmptyState, InlineError } from '../components/ui/States'
import { Loader } from '../components/Loader'
import { api } from '../lib/api'
import { useApi } from '../lib/useApi'
import { GroupRowItem } from './groups/GroupRowItem'

// Watched-group management. Loads the full group list (GET /api/groups), edits
// the `watch` flag optimistically in local state, and persists the whole set via
// POST /api/groups/watch when the operator saves.
export default function Groups() {
  const { data, error, loading, reload, setData } = useApi(api.groups.list, [])
  const [q, setQ] = useState('')
  const [dirty, setDirty] = useState(false)
  const [saving, setSaving] = useState(false)
  const rows = data ?? []

  const s = q.trim().toLowerCase()
  const filtered = s
    ? rows.filter(
        (r) =>
          r.groupName?.toLowerCase().includes(s) ||
          r.groupRemark?.toLowerCase().includes(s) ||
          String(r.groupId).includes(s),
      )
    : rows
  const watchedCount = rows.filter((r) => r.watch).length

  const setWatch = (groupId: number, v: boolean) => {
    setData((prev) => (prev ?? []).map((r) => (r.groupId === groupId ? { ...r, watch: v } : r)))
    setDirty(true)
  }

  const save = async () => {
    setSaving(true)
    try {
      await api.groups.saveWatch(
        rows.map((r) => ({ groupId: r.groupId, groupName: r.groupName, watch: r.watch })),
      )
      setDirty(false)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Page>
      <PageHeader
        title="群组"
        description="选择需要监听并升级的群组"
        actions={
          dirty ? (
            <Button onPress={save} isPending={saving}>
              保存更改
            </Button>
          ) : null
        }
      />
      <SectionCard
        title="群组列表"
        description={`共 ${rows.length} 个群 · 已监听 ${watchedCount} 个`}
        actions={
          <TextField aria-label="搜索群组" value={q} onChange={setQ} className="w-44">
            <Input placeholder="搜索群名或群号" variant="secondary" />
          </TextField>
        }
      >
        {loading ? (
          <Loader label="正在加载群组…" />
        ) : error ? (
          <InlineError message={error} onRetry={reload} />
        ) : filtered.length === 0 ? (
          <EmptyState
            icon="groups"
            title={rows.length === 0 ? '暂无群组' : '未找到匹配的群组'}
            description={rows.length === 0 ? '请确认机器人已加入群聊' : undefined}
          />
        ) : (
          <ul className="divide-y divide-border">
            {filtered.map((r) => (
              <GroupRowItem
                key={r.groupId}
                row={r}
                watch={r.watch}
                onWatch={(v) => setWatch(r.groupId, v)}
              />
            ))}
          </ul>
        )}
      </SectionCard>
    </Page>
  )
}
