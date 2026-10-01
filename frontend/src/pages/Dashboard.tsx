import { useState } from 'react'
import { Button } from '@heroui/react'
import { KPIGroup, Widget } from '@heroui-pro/react'
import { Link, useNavigate } from 'react-router-dom'
import { Page } from '../components/Page'
import { PageHeader } from '../components/PageHeader'
import { Stat } from '../components/ui/Stat'
import { Toggle } from '../components/ui/Toggle'
import { IntentChip } from '../components/ui/IntentChip'
import { InlineError } from '../components/ui/States'
import { AppIcon } from '../lib/icons'
import { api } from '../lib/api'
import { useApi } from '../lib/useApi'
import { useLive } from '../lib/store'
import { BufferPanel, EscalationPanel, MessageFeed } from './dashboard/panels'

// Overview: merges the one-shot GET /api/status (extra fields like totalGroups /
// quietWindowSec) with the live SSE status, preferring live where both exist.
// The global enable switch uses optimistic local state since api.setEnabled does
// not necessarily echo a status event back over the stream.
export default function Dashboard() {
  const { data: sys, error, reload } = useApi(api.status, [])
  const navigate = useNavigate()
  const live = useLive((s) => s.status)
  const [enabledLocal, setEnabledLocal] = useState<boolean | null>(null)
  const [busy, setBusy] = useState(false)
  const [toggleError, setToggleError] = useState('')

  const enabled = enabledLocal ?? live?.enabled ?? sys?.enabled ?? false
  const connected = live?.onebotConnected ?? sys?.onebotConnected ?? false
  const watched = live?.watchedGroups ?? sys?.watchedGroups ?? 0
  const totalGroups = sys?.totalGroups ?? watched
  const masters = live?.masters ?? sys?.masters ?? 0
  const llm = live?.llmEnabled ?? sys?.llmEnabled ?? false
  const jev = live?.jevEnabled ?? sys?.jevEnabled ?? false
  const quiet = sys?.quietWindowSec ?? 0

  const toggle = async (v: boolean) => {
    setEnabledLocal(v)
    setBusy(true)
    setToggleError('')
    try {
      await api.setEnabled(v)
    } catch {
      setEnabledLocal(!v)
      setToggleError('运行状态更新失败，请重试')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Page className="dashboard-page">
      <PageHeader
        title="总览"
        description="关注待办通知，掌握群消息处理进度。"
        actions={
          <>
            <Toggle
              label={enabled ? '运行中' : '已暂停'}
              isSelected={enabled}
              onChange={toggle}
              isDisabled={busy}
            />
          </>
        }
      />
      {error ? <InlineError message={error} onRetry={reload} /> : null}
      {toggleError ? <InlineError message={toggleError} /> : null}
      <KPIGroup className="!grid grid-cols-2 lg:grid-cols-4">
        <Stat
          label="NapCat"
          value={connected ? '已连接' : '未连接'}
          icon={connected ? 'connected' : 'disconnected'}
          tone={connected ? 'success' : 'danger'}
        />
        <Stat label="监听群组" value={`${watched}/${totalGroups}`} icon="groups" tone="accent" />
        <Stat label="推送主人" value={masters} icon="masters" tone="accent" />
        <Stat label="静默窗口" value={`${quiet}s`} icon="clock" tone="default" />
      </KPIGroup>

      <div className="dashboard-strategy my-4">
        <AppIcon name="rules" className="size-4 shrink-0 text-accent" />
        <p className="text-xs text-muted">聚合消息 <span className="mx-1">→</span> 判断重要性 <span className="mx-1">→</span> 推送通知</p>
        <div className="ml-auto flex flex-wrap items-center gap-2">
          <IntentChip intent={jev ? 'primary' : 'default'}>Jev {jev ? '启用' : '关闭'}</IntentChip>
          <IntentChip intent={llm ? 'primary' : 'default'}>LLM {llm ? '启用' : '关闭'}</IntentChip>
          <Button variant="ghost" size="sm" onPress={() => navigate('/rules')}>调整规则</Button>
        </div>
      </div>
      <div className="dashboard-main">
        <div className="min-w-0">
          <EscalationPanel />
          <div className="dashboard-feed"><MessageFeed /></div>
        </div>
        <div className="grid min-w-0 gap-4">
          <BufferPanel />
          <Widget>
            <Widget.Header><Widget.Title>工作空间</Widget.Title><Widget.Description>常用管理</Widget.Description></Widget.Header>
            <Widget.Content>
              <div className="dashboard-shortcuts">
                {[
                  { to: '/groups', icon: 'groups' as const, title: '监听范围', description: '选择关注的群组，查看原始消息' },
                  { to: '/notifications', icon: 'send' as const, title: '通知通道', description: '管理 ntfy 与 Bark 推送目标' },
                  { to: '/intelligence', icon: 'intelligence' as const, title: '智能处理', description: '配置模型、意图判断和附件阅读' },
                ].map((item) => <Link key={item.to} to={item.to} className="dashboard-shortcut">
                  <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-surface-secondary text-accent"><AppIcon name={item.icon} className="size-4" /></span>
                  <span className="min-w-0 flex-1"><span className="block text-xs font-medium text-foreground">{item.title}</span><span className="mt-0.5 block text-[11px] text-muted">{item.description}</span></span>
                  <AppIcon name="chevronRight" className="size-3 shrink-0 text-muted" />
                </Link>)}
              </div>
            </Widget.Content>
          </Widget>
        </div>
      </div>
    </Page>
  )
}
