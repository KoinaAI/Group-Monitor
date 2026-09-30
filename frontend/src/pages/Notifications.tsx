import { useEffect, useState } from 'react'
import { Button } from '@heroui/react'
import { EmptyState, NativeSelect } from '@heroui-pro/react'
import { Page } from '../components/Page'
import { PageHeader } from '../components/PageHeader'
import { SectionCard } from '../components/ui/SectionCard'
import { TextSetting } from '../components/ui/TextSetting'
import { Toggle } from '../components/ui/Toggle'
import { IntentChip } from '../components/ui/IntentChip'
import { api } from '../lib/api'
import { uid } from '../lib/id'
import type { NotificationTarget, SourceConfig } from '../lib/types'

// Keep the presentation-only draft field local until it is persisted by the
// notifications endpoint. Older servers simply omit it when loading targets.
type NotificationDraft = NotificationTarget & { note?: string }

export default function Notifications() {
  const [targets, setTargets] = useState<NotificationDraft[]>([])
  const [saved, setSaved] = useState('[]')
  const [sources, setSources] = useState<SourceConfig[]>([])
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({})
  const [error, setError] = useState('')
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)
  const [loading, setLoading] = useState(true)
  useEffect(() => {
    api.notifications.list()
      .then((v) => { setTargets((v ?? []) as NotificationDraft[]); setSaved(JSON.stringify(v ?? [])) })
      .catch((e) => setError(e instanceof Error ? e.message : '通知目标加载失败'))
      .finally(() => setLoading(false))
    api.sources.list().then((v) => setSources(v ?? [])).catch((e) => setError(String(e)))
  }, [])
  const dirty = JSON.stringify(targets) !== saved
  const accounts = sources.flatMap((source) => source.accounts.map((account) => ({ ...account, sourceName: source.name })))
  const change = (id: string, patch: Partial<NotificationTarget>) => {
    setTargets((current) => current.map((target) => target.id === id ? { ...target, ...patch } : target))
    setNote('')
  }
  const save = async () => {
    setBusy(true); setError(''); setNote('')
    try { const v = await api.notifications.save(targets); setTargets(v as NotificationDraft[]); setSaved(JSON.stringify(v)); setNote('通知目标已保存') }
    catch (e) { setError(e instanceof Error ? e.message : '保存失败') }
    finally { setBusy(false) }
  }
  const test = async (id: string) => {
    setBusy(true); setError(''); setNote('')
    try { await api.notifications.test(id); setNote('测试通知已发送') }
    catch (e) { setError(e instanceof Error ? e.message : '发送失败') }
    finally { setBusy(false) }
  }
  const add = (kind: 'ntfy' | 'bark') => {
    const id = uid()
    setTargets((current) => [...current, { id, name: kind === 'ntfy' ? 'ntfy 广播' : 'Bark 推送', kind, enabled: true, url: kind === 'ntfy' ? 'https://ntfy.sh' : 'https://api.day.app', minLevel: 1, group: '讯枢', accountIds: [], note: '' }])
    setCollapsed((current) => ({ ...current, [id]: false }))
    setNote('')
  }
  return <Page>
    <PageHeader title="广播通知" description="配置主题与设备推送，按账号和通知等级分发。" actions={<Button size="sm" isPending={busy} isDisabled={loading || !dirty} onPress={save}>保存通知目标</Button>} />
    <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
      <div className="flex items-center gap-2 text-sm text-muted">
        <span className="tabular-nums">{targets.length} 个目标</span>
        <IntentChip intent="success">{targets.filter((target) => target.enabled).length} 个启用</IntentChip>
        {dirty && <span className="text-xs">有未保存的更改</span>}
      </div>
      <div className="flex gap-2">
        <Button size="sm" variant="secondary" isDisabled={loading || busy} onPress={() => add('ntfy')}>添加 ntfy</Button>
        <Button size="sm" variant="secondary" isDisabled={loading || busy} onPress={() => add('bark')}>添加 Bark</Button>
      </div>
    </div>
    {error && <p role="alert" className="mb-4 text-sm text-danger">{error}</p>}
    {note && <p role="status" className="mb-4 text-sm text-success">{note}</p>}
    {loading ? <p role="status" className="py-6 text-sm text-muted">正在加载通知目标…</p> : targets.length === 0 ? (
      <EmptyState className="py-10">
        <EmptyState.Header>
          <EmptyState.Title>还没有通知目标</EmptyState.Title>
          <EmptyState.Description>添加 ntfy 主题或 Bark 设备，即可在手机上接收群通知。</EmptyState.Description>
        </EmptyState.Header>
      </EmptyState>
    ) : <div className="grid items-start gap-4 xl:grid-cols-2">
      {targets.map((target) => {
        const isCollapsed = collapsed[target.id] ?? false
        return <SectionCard key={target.id} title={target.name || (target.kind === 'ntfy' ? 'ntfy 广播' : 'Bark 推送')} description={target.kind === 'ntfy' ? 'ntfy · 主题广播' : 'Bark · 设备推送'} actions={<>
          <Button size="sm" variant="secondary" isDisabled={busy || !target.enabled || dirty} onPress={() => test(target.id)}>发送测试通知</Button>
          <Button size="sm" variant="tertiary" aria-expanded={!isCollapsed} onPress={() => setCollapsed((current) => ({ ...current, [target.id]: !isCollapsed }))}>
            {isCollapsed ? '展开' : '收起'}
          </Button>
          <Button size="sm" variant="danger-soft" isDisabled={busy} onPress={() => { setTargets((current) => current.filter((item) => item.id !== target.id)); setCollapsed((current) => { const next = { ...current }; delete next[target.id]; return next }); setNote('') }}>移除</Button>
        </>}>
        {isCollapsed ? <p className="truncate text-xs text-muted">{target.note?.trim() || '配置已收起，点击展开查看详情。'}</p> : <fieldset disabled={busy} className="flex min-w-0 flex-col gap-4">
          <Toggle label="启用通知目标" isSelected={target.enabled} onChange={(enabled) => change(target.id, { enabled })} isDisabled={busy} size="sm" />
          <div className="grid min-w-0 gap-3 sm:grid-cols-2">
            <TextSetting label="名称" value={target.name} onChange={(name) => change(target.id, { name })} />
            <TextSetting label="服务器地址" type="url" value={target.url} onChange={(url) => change(target.id, { url })} />
            {target.kind === 'ntfy' ? <>
              <TextSetting label="主题" value={target.topic ?? ''} onChange={(topic) => change(target.id, { topic })} placeholder="school-notices" />
              <TextSetting label="访问令牌（可选）" type="password" value={target.token ?? ''} onChange={(token) => change(target.id, { token })} description="留空保留原值，保存后隐藏。" />
            </> : <>
              <TextSetting label="设备 Key" type="password" value={target.deviceKey ?? ''} onChange={(deviceKey) => change(target.id, { deviceKey })} description="留空保留原值，保存后隐藏。" />
              <TextSetting label="通知分组" value={target.group ?? ''} onChange={(group) => change(target.id, { group })} />
            </>}
            <TextSetting className="sm:col-span-2" label="备注" value={target.note ?? ''} onChange={(note) => change(target.id, { note } as Partial<NotificationDraft>)} placeholder="例如：值班手机、备用通道" description="仅用于识别通知目标，不会发送给接收者。" />
          </div>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <label htmlFor={`level-${target.id}`} className="text-sm font-medium">最低通知等级</label>
            <NativeSelect variant="secondary" className="w-44">
              <NativeSelect.Trigger id={`level-${target.id}`} value={target.minLevel} onChange={(event) => change(target.id, { minLevel: Number(event.target.value) })}>
                <NativeSelect.Option value={1}>1 · 一般及以上</NativeSelect.Option>
                <NativeSelect.Option value={2}>2 · 重要及以上</NativeSelect.Option>
                <NativeSelect.Option value={3}>3 · 仅紧急</NativeSelect.Option>
              </NativeSelect.Trigger>
            </NativeSelect>
          </div>
          <fieldset className="min-w-0">
            <legend className="mb-2 text-xs text-muted">适用账号（不选择时接收所有账号）</legend>
            <div className="flex flex-wrap gap-x-4 gap-y-2">
              {accounts.length ? accounts.map((account) => <label key={account.id} className="flex cursor-pointer items-center gap-2 text-sm">
                <input type="checkbox" className="size-4 accent-[var(--accent)]" checked={target.accountIds?.includes(account.id) ?? false} disabled={busy} onChange={(event) => change(target.id, { accountIds: event.target.checked ? [...(target.accountIds ?? []), account.id] : target.accountIds?.filter((id) => id !== account.id) })} />
                {account.sourceName} / {account.name}
              </label>) : <span className="text-xs text-muted">所有账号</span>}
            </div>
          </fieldset>
        </fieldset>}
      </SectionCard>
      })}
    </div>}
  </Page>
}
