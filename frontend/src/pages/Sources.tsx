import { useEffect, useState } from 'react'
import { Button } from '@heroui/react'
import { Page } from '../components/Page'
import { PageHeader } from '../components/PageHeader'
import { SectionCard } from '../components/ui/SectionCard'
import { TextSetting } from '../components/ui/TextSetting'
import { Loader } from '../components/Loader'
import { api } from '../lib/api'
import { activeAccount, selectAccount, newAccount } from '../lib/accounts'
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
  return <Page>
    <PageHeader title="信息源" description="按渠道组织账号；每个账号独立设置监听群、主人和规则。" actions={<Button onPress={save} isPending={saving}>保存信息源</Button>} />
    {error && <p role="alert" className="mb-4 text-sm text-danger">{error}</p>}
    {note && <p role="status" className="mb-4 text-sm text-success">{note}</p>}
    {!sources ? <Loader /> : <div className="flex flex-col gap-6">
      {sources.length === 0 && <SectionCard title="尚未添加信息源" description="当前使用密码登录。添加 NapCat 后，可接入一个或多个 QQ 账号。"><p className="text-sm text-muted">每个账号需提供独立的 OneBot 接口。</p></SectionCard>}
      {sources.map((source) => <SectionCard key={source.id} title={source.name || 'NapCat'} description="渠道：NapCat / OneBot v11" actions={<Button variant="danger-soft" onPress={() => setSources(sources.filter((s) => s.id !== source.id))}>移除信息源</Button>}>
        <TextSetting label="信息源名称" value={source.name} onChange={(name) => patch(source.id, { name })} />
        <div className="mt-6 flex flex-col gap-6">
          {source.accounts.map((account) => <section key={account.id} className="border-t border-separator pt-6">
            <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
              <p className="text-sm text-muted">{statuses.find((s) => s.accountId === account.id)?.connected ? '已连接' : account.enabled ? '等待连接' : '已停用'}</p>
              <div className="flex gap-2">
                <Button variant="secondary" isDisabled={!statuses.some((s) => s.accountId === account.id)} onPress={() => selectAccount(account.id)}>管理此账号</Button>
                <Button variant="danger-soft" onPress={() => patch(source.id, { accounts: source.accounts.filter((a) => a.id !== account.id) })}>移除账号</Button>
              </div>
            </div>
            <AccountFields account={account} onChange={(next) => patch(source.id, { accounts: source.accounts.map((a) => a.id === next.id ? next : a) })} />
          </section>)}
          <Button variant="secondary" className="self-start" onPress={() => patch(source.id, { accounts: [...source.accounts, newAccount()] })}>添加账号</Button>
        </div>
      </SectionCard>)}
      <Button variant="secondary" className="self-start" onPress={() => setSources([...sources, { id: crypto.randomUUID(), name: 'NapCat', kind: 'napcat', accounts: [] }])}>添加 NapCat 信息源</Button>
    </div>}
  </Page>
}
