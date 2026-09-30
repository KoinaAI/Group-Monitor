import { useEffect, useState } from 'react'
import { Button } from '@heroui/react'
import { Page } from '../components/Page'
import { PageHeader } from '../components/PageHeader'
import { SectionCard } from '../components/ui/SectionCard'
import { TextSetting } from '../components/ui/TextSetting'
import { api } from '../lib/api'
import type { AgentKey, SourceConfig } from '../lib/types'

export default function AgentAccess() {
  const [keys, setKeys] = useState<AgentKey[]>([])
  const [sources, setSources] = useState<SourceConfig[]>([])
  const [name, setName] = useState('')
  const [accounts, setAccounts] = useState<string[]>([])
  const [secret, setSecret] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    api.agentKeys.list().then((v) => setKeys(v ?? [])).catch((e) => setError(String(e)))
    api.sources.list().then(setSources).catch((e) => setError(String(e)))
  }, [])
  const create = async () => {
    setBusy(true); setError(''); setSecret('')
    try { const r = await api.agentKeys.create(name, accounts); setKeys(r.keys); setSecret(r.apiKey); setName('') }
    catch (e) { setError(e instanceof Error ? e.message : '创建失败') }
    finally { setBusy(false) }
  }
  const revoke = async (id: string) => {
    setBusy(true); setError('')
    try { setKeys(await api.agentKeys.revoke(id)); setSecret('') }
    catch (e) { setError(e instanceof Error ? e.message : '撤销失败') }
    finally { setBusy(false) }
  }
  const accountName = (id: string) => sources.flatMap((s) => s.accounts.map((a) => ({ ...a, sourceName: s.name }))).find((a) => a.id === id)?.name ?? '已移除账号'
  return <Page>
    <PageHeader title="Agent 接入" description="让第三方 Agent 检索正式通知、近期消息及原文上下文。所有工具均为只读。" />
    {error && <p role="alert" className="mb-4 text-sm text-danger">{error}</p>}
    <div className="flex flex-col gap-6">
      <SectionCard title="接入地址" description="MCP 与直接 API 共用同一 API Key，均通过 Authorization: Bearer 鉴权。">
        <dl className="flex flex-col gap-3 text-sm"><div><dt className="text-muted">MCP · Streamable HTTP</dt><dd className="break-all">{location.origin}/api/mcp</dd></div><div><dt className="text-muted">直接 API · 工具目录</dt><dd className="break-all">{location.origin}/api/agent/v1/tools</dd></div><div><dt className="text-muted">直接 API · 查询</dt><dd className="break-all">{location.origin}/api/agent/v1/query</dd></div></dl>
      </SectionCard>
      <SectionCard title="创建 API Key" description="每个 Agent 使用独立密钥，便于分别撤销。">
        <div className="flex flex-col gap-4">
          <TextSetting label="密钥名称" value={name} onChange={setName} placeholder="例如：个人助理" />
          <fieldset><legend className="mb-2 text-sm">可访问账号（不选择时允许所有当前及未来账号）</legend><div className="flex flex-wrap gap-4">
            {sources.flatMap((s) => s.accounts.map((a) => <label key={a.id} className="flex items-center gap-2 text-sm"><input type="checkbox" checked={accounts.includes(a.id)} onChange={(e) => setAccounts(e.target.checked ? [...accounts, a.id] : accounts.filter((id) => id !== a.id))} />{s.name} / {a.name}</label>))}
          </div></fieldset>
          <Button className="self-start" isPending={busy} isDisabled={!name.trim()} onPress={create}>创建密钥</Button>
          {secret && <div role="status" className="rounded-xl bg-surface-secondary p-4"><p className="mb-2 text-sm font-medium">密钥只显示一次，请立即保存。</p><code className="break-all text-sm" aria-label="新 API Key">{secret}</code></div>}
        </div>
      </SectionCard>
      <SectionCard title="已授权 Agent">
        {keys.length === 0 ? <p className="text-sm text-muted">暂无 API Key</p> : <ul className="divide-y divide-separator">{keys.map((key) => <li key={key.id} className="flex items-center justify-between gap-4 py-4 first:pt-0 last:pb-0"><div className="min-w-0"><p className="font-medium">{key.name}</p><p className="mt-1 text-sm text-muted">{key.accountIds?.length ? key.accountIds.map(accountName).join('、') : '所有账号'} · {new Date(key.createdAt).toLocaleDateString()}</p></div><Button variant="danger-soft" isDisabled={busy} onPress={() => revoke(key.id)}>撤销</Button></li>)}</ul>}
      </SectionCard>
    </div>
  </Page>
}
