import { useEffect, useState } from 'react'
import { Button } from '@heroui/react'
import { Page } from '../components/Page'
import { PageHeader } from '../components/PageHeader'
import { SectionCard } from '../components/ui/SectionCard'
import { TextSetting } from '../components/ui/TextSetting'
import { NumberSetting } from '../components/ui/NumberSetting'
import { Toggle } from '../components/ui/Toggle'
import { api } from '../lib/api'
import type { NotificationTarget, SourceConfig } from '../lib/types'

export default function Notifications() {
  const [targets, setTargets] = useState<NotificationTarget[]>([])
  const [saved, setSaved] = useState('[]')
  const [sources, setSources] = useState<SourceConfig[]>([])
  const [error, setError] = useState('')
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    api.notifications.list().then((v) => { setTargets(v ?? []); setSaved(JSON.stringify(v ?? [])) }).catch((e) => setError(String(e)))
    api.sources.list().then(setSources).catch((e) => setError(String(e)))
  }, [])
  const change = (id: string, patch: Partial<NotificationTarget>) => setTargets(targets.map((t) => t.id === id ? { ...t, ...patch } : t))
  const save = async () => {
    setBusy(true); setError(''); setNote('')
    try { const v = await api.notifications.save(targets); setTargets(v); setSaved(JSON.stringify(v)); setNote('通知目标已保存') }
    catch (e) { setError(e instanceof Error ? e.message : '保存失败') }
    finally { setBusy(false) }
  }
  const test = async (id: string) => {
    setBusy(true); setError(''); setNote('')
    try { await api.notifications.test(id); setNote('测试通知已发送') }
    catch (e) { setError(e instanceof Error ? e.message : '发送失败') }
    finally { setBusy(false) }
  }
  const add = (kind: 'ntfy' | 'bark') => setTargets([...targets, { id: crypto.randomUUID(), name: kind === 'ntfy' ? 'ntfy 广播' : 'Bark 推送', kind, enabled: true, url: kind === 'ntfy' ? 'https://ntfy.sh' : 'https://api.day.app', minLevel: 1, group: '讯枢', accountIds: [] }])
  return <Page>
    <PageHeader title="广播通知" description="通过 ntfy 主题或 Bark 设备推送正式提醒。" actions={<Button isPending={busy} onPress={save}>保存通知目标</Button>} />
    {error && <p role="alert" className="mb-4 text-sm text-danger">{error}</p>}
    {note && <p role="status" className="mb-4 text-sm text-success">{note}</p>}
    <div className="flex flex-col gap-6">
      {targets.length === 0 && <p className="text-sm text-muted">添加通知目标后，即使没有 QQ 主人，也可接收群通知。</p>}
      {targets.map((target) => <SectionCard key={target.id} title={target.name} description={target.kind === 'ntfy' ? '主题广播 · 支持自托管服务器' : '设备推送 · 支持自托管服务器'} actions={<Button variant="danger-soft" onPress={() => setTargets(targets.filter((t) => t.id !== target.id))}>移除</Button>}>
        <div className="flex flex-col gap-4">
          <Toggle label="启用通知目标" isSelected={target.enabled} onChange={(enabled) => change(target.id, { enabled })} />
          <TextSetting label="名称" value={target.name} onChange={(name) => change(target.id, { name })} />
          <TextSetting label="服务器地址" value={target.url} onChange={(url) => change(target.id, { url })} />
          {target.kind === 'ntfy' ? <>
            <TextSetting label="主题" value={target.topic ?? ''} onChange={(topic) => change(target.id, { topic })} />
            <TextSetting label="访问令牌（可选）" type="password" value={target.token ?? ''} onChange={(token) => change(target.id, { token })} description="保存后不回显；留空保留原值。" />
          </> : <>
            <TextSetting label="设备 Key" type="password" value={target.deviceKey ?? ''} onChange={(deviceKey) => change(target.id, { deviceKey })} description="保存后不回显；留空保留原值。" />
            <TextSetting label="通知分组" value={target.group ?? ''} onChange={(group) => change(target.id, { group })} />
          </>}
          <NumberSetting label="最低通知等级" description="1 一般 · 2 重要 · 3 紧急" value={target.minLevel} onChange={(minLevel) => change(target.id, { minLevel })} minValue={1} maxValue={3} />
          <fieldset><legend className="mb-2 text-sm">适用账号（不选择时接收所有账号）</legend><div className="flex flex-wrap gap-4">
            {sources.flatMap((s) => s.accounts.map((a) => <label key={a.id} className="flex items-center gap-2 text-sm"><input type="checkbox" checked={target.accountIds?.includes(a.id) ?? false} onChange={(e) => change(target.id, { accountIds: e.target.checked ? [...(target.accountIds ?? []), a.id] : target.accountIds?.filter((id) => id !== a.id) })} />{s.name} / {a.name}</label>))}
          </div></fieldset>
          <Button className="self-start" variant="secondary" isDisabled={busy || !target.enabled || JSON.stringify(targets) !== saved} onPress={() => test(target.id)}>发送测试通知</Button>
        </div>
      </SectionCard>)}
      <div className="flex gap-3"><Button variant="secondary" onPress={() => add('ntfy')}>添加 ntfy</Button><Button variant="secondary" onPress={() => add('bark')}>添加 Bark</Button></div>
    </div>
  </Page>
}
