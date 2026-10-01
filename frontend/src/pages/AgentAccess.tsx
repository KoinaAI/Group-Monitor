import { useEffect, useState } from 'react'
import { Button, Chip, Description, Label } from '@heroui/react'
import { CheckboxButtonGroup, EmptyState } from '@heroui-pro/react'
import { Page } from '../components/Page'
import { PageHeader } from '../components/PageHeader'
import { SectionCard } from '../components/ui/SectionCard'
import { TextSetting } from '../components/ui/TextSetting'
import { api } from '../lib/api'
import { AppIcon } from '../lib/icons'
import type { AgentKey, SourceConfig } from '../lib/types'
import { SkillInstall } from './agents/SkillInstall'

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
  const availableAccounts = sources.flatMap((source) => source.accounts.map((account) => ({ ...account, sourceName: source.name })))
  return <Page>
    <PageHeader title="Agent 接入" description="授权 Agent 检索通知、近期消息与原文上下文。" actions={<Chip size="sm" variant="soft" color="accent"><Chip.Label>只读访问</Chip.Label></Chip>} />
    {error && <p role="alert" className="mb-4 text-sm text-danger">{error}</p>}
    <div className="grid items-start gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)]">
      <SectionCard title="创建 API Key" description="每个 Agent 使用独立密钥，便于分别撤销。">
        <div className="flex flex-col gap-3">
          <div className="flex flex-wrap items-end gap-2">
            <TextSetting className="min-w-44 flex-1" label="密钥名称" value={name} onChange={setName} placeholder="例如：个人助理" />
            <Button size="sm" isPending={busy} isDisabled={!name.trim()} onPress={create}>创建密钥</Button>
          </div>
          <CheckboxButtonGroup value={accounts} onChange={setAccounts} aria-label="可访问账号">
            <Label>可访问账号</Label>
            <Description>不选择时允许所有当前及未来账号。</Description>
            <div className="mt-2 flex flex-wrap gap-2">
              {availableAccounts.map((account) => <CheckboxButtonGroup.Item key={account.id} value={account.id} aria-label={`${account.sourceName} / ${account.name}`} className="flex-row items-center gap-2 px-3 py-2">
                <CheckboxButtonGroup.Indicator className="static" />
                <CheckboxButtonGroup.ItemContent><span className="text-sm">{account.sourceName} / {account.name}</span></CheckboxButtonGroup.ItemContent>
              </CheckboxButtonGroup.Item>)}
            </div>
          </CheckboxButtonGroup>
          {availableAccounts.length === 0 && <p className="text-xs text-muted">尚未添加信息源账号，密钥可用于后续接入的账号。</p>}
          {secret && <div role="status" className="rounded-xl border border-accent/20 bg-accent/5 p-3"><p className="mb-2 text-sm font-medium">密钥只显示一次，请立即保存。</p><code className="break-all text-sm" aria-label="新 API Key">{secret}</code></div>}
        </div>
        <div className="mt-4 flex items-center justify-between gap-3 border-t border-separator pt-3">
          <h3 className="text-sm font-medium">已授权 Agent</h3>
          <span className="text-xs tabular-nums text-muted">{keys.length} 个密钥</span>
        </div>
        {keys.length === 0 ? <EmptyState size="sm" className="py-4"><EmptyState.Header><EmptyState.Title>暂无 API Key</EmptyState.Title><EmptyState.Description>创建密钥后，在 Agent 运行环境中完成配置。</EmptyState.Description></EmptyState.Header></EmptyState> : <ul className="mt-2 flex max-h-64 flex-col overflow-y-auto">{keys.map((key) => <li key={key.id} className="flex min-w-0 items-center gap-3 py-2">
          <AppIcon name="key" className="size-4 shrink-0 text-muted" />
          <div className="min-w-0 flex-1"><p className="truncate text-sm font-medium">{key.name}</p><p className="break-words text-xs text-muted">{key.accountIds?.length ? key.accountIds.map(accountName).join('、') : '所有账号'} · {new Date(key.createdAt).toLocaleDateString()}</p></div>
          <Button size="sm" variant="danger-soft" isDisabled={busy} onPress={() => revoke(key.id)}>撤销</Button>
        </li>)}</ul>}
      </SectionCard>
      <SkillInstall />
        <SectionCard className="lg:col-span-2" title="接入地址" description="MCP 与直接 API 共用密钥，使用 Authorization: Bearer 鉴权。">
          <dl className="grid gap-4 text-sm md:grid-cols-3">{[
            ['MCP · Streamable HTTP', '/api/mcp'],
            ['直接 API · 工具目录', '/api/agent/v1/tools'],
            ['直接 API · 查询', '/api/agent/v1/query'],
          ].map(([label, path]) => <div key={path} className="flex min-w-0 flex-col gap-1"><dt className="text-xs text-muted">{label}</dt><dd className="break-all font-mono text-xs leading-5">{location.origin}{path}</dd></div>)}</dl>
        </SectionCard>
    </div>
  </Page>
}
