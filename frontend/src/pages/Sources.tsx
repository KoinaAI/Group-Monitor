import { useEffect, useState } from 'react'
import { Button, Card, Chip } from '@heroui/react'
import { EmptyState } from '@heroui-pro/react'
import { Page } from '../components/Page'
import { PageHeader } from '../components/PageHeader'
import { SectionCard } from '../components/ui/SectionCard'
import { TextSetting } from '../components/ui/TextSetting'
import { Loader } from '../components/Loader'
import { api } from '../lib/api'
import { activeAccount, selectAccount, newAccount } from '../lib/accounts'
import { uid } from '../lib/id'
import { AppIcon } from '../lib/icons'
import type { SourceConfig, SourceStatus } from '../lib/types'
import { AccountFields } from './sources/AccountFields'

export default function Sources() {
  const [sources, setSources] = useState<SourceConfig[] | null>(null)
  const [statuses, setStatuses] = useState<SourceStatus[]>([])
  const [error, setError] = useState('')
  const [note, setNote] = useState('')
  const [saving, setSaving] = useState(false)
  useEffect(() => {
    api.sources.list().then(setSources).catch((e) => setError(String(e)))
    const refresh = () => api.sources.status().then(setStatuses).catch(() => {})
    void refresh()
    const timer = setInterval(refresh, 10000)
    return () => clearInterval(timer)
  }, [])
  const save = async () => {
    if (!sources) return
    setSaving(true); setError(''); setNote('')
    try {
      const next = await api.sources.save(sources)
      setSources(next); setNote('信息源已保存')
      if (activeAccount() && !next.some((s) => s.accounts.some((a) => a.id === activeAccount()))) selectAccount('')
    } catch (e) { setError(e instanceof Error ? e.message : '保存失败') }
    finally { setSaving(false) }
  }
  const patch = (id: string, change: Partial<SourceConfig>) => setSources((all) => all?.map((s) => s.id === id ? { ...s, ...change } : s) ?? [])
  const addSource = () => setSources((current) => [...(current ?? []), { id: uid(), name: 'NapCat', kind: 'napcat', accounts: [] }])
  return <Page>
    <PageHeader title="信息源" description="管理接入渠道与账号，每个账号独立配置监听群、主人和规则。" actions={<><Button variant="secondary" onPress={addSource}><AppIcon name="add" className="size-4" />添加 NapCat 信息源</Button><Button onPress={save} isPending={saving} isDisabled={!sources}>保存信息源</Button></>} />
    {error && <p role="alert" className="mb-4 text-sm text-danger">{error}</p>}
    {note && <p role="status" className="mb-4 text-sm text-success">{note}</p>}
    {!sources ? <Loader /> : <div className="flex flex-col gap-4">
      {sources.length === 0 && <Card><EmptyState size="sm">
        <EmptyState.Header><EmptyState.Media variant="icon"><AppIcon name="connection" className="size-6" /></EmptyState.Media><EmptyState.Title>尚未添加信息源</EmptyState.Title><EmptyState.Description>接入 NapCat，为每个 QQ 账号配置独立的 OneBot 接口。</EmptyState.Description></EmptyState.Header>
      </EmptyState></Card>}
      {sources.map((source) => <SectionCard key={source.id} title={<span className="flex items-center gap-2"><AppIcon name="connection" className="size-4 text-muted" />{source.name || 'NapCat'}<Chip size="sm" variant="soft">{source.accounts.length} 个账号</Chip></span>} description="NapCat / OneBot v11" actions={<Button size="sm" variant="danger-soft" onPress={() => setSources(sources.filter((s) => s.id !== source.id))}>移除信息源</Button>}>
        <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
          <TextSetting className="w-full sm:max-w-sm" label="信息源名称" value={source.name} onChange={(name) => patch(source.id, { name })} />
          <Button variant="secondary" onPress={() => patch(source.id, { accounts: [...source.accounts, newAccount()] })}><AppIcon name="add" className="size-4" />添加账号</Button>
        </div>
        <div className="grid items-start gap-4 [grid-template-columns:repeat(auto-fit,minmax(min(100%,32rem),1fr))]">
          {source.accounts.map((account) => {
            const connected = statuses.some((s) => s.accountId === account.id && s.connected)
            return <Card key={account.id} variant="secondary" className="min-w-0 gap-4 shadow-none">
              <Card.Header className="flex flex-row flex-wrap items-center justify-between gap-2">
                <Chip size="sm" variant="soft" color={connected ? 'success' : account.enabled ? 'warning' : 'default'}>{connected ? '已连接' : account.enabled ? '等待连接' : '已停用'}</Chip>
                <div className="flex gap-1">
                  <Button size="sm" variant="ghost" isDisabled={!statuses.some((s) => s.accountId === account.id)} onPress={() => selectAccount(account.id)}>管理此账号<AppIcon name="chevronRight" className="size-3" /></Button>
                  <Button size="sm" variant="danger-soft" onPress={() => patch(source.id, { accounts: source.accounts.filter((a) => a.id !== account.id) })}>移除账号</Button>
                </div>
              </Card.Header>
              <Card.Content><AccountFields account={account} onChange={(next) => patch(source.id, { accounts: source.accounts.map((a) => a.id === next.id ? next : a) })} /></Card.Content>
            </Card>
          })}
        </div>
        {source.accounts.length === 0 && <p className="rounded-xl bg-surface-secondary px-4 py-3 text-sm text-muted">尚无账号。添加账号并保存后开始连接。</p>}
      </SectionCard>)}
    </div>}
  </Page>
}
