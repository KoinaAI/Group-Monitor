import { useState } from 'react'
import { Button } from '@heroui/react'
import { KPIGroup, Widget } from '@heroui-pro/react'
import { useNavigate } from 'react-router-dom'
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
  const streamConnected = useLive((s) => s.connected)
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
    <Page>
      <PageHeader
        title="总览"
        description="关注待办通知，掌握群消息处理进度。"
        actions={
          <>
            <IntentChip intent={streamConnected ? 'success' : 'warning'}>
              {streamConnected ? '实时同步' : '正在重连'}
            </IntentChip>
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

      <Widget className="my-4">
        <Widget.Content className="flex flex-wrap items-center justify-between gap-x-6 gap-y-3 !py-3">
          <div className="flex min-w-0 items-center gap-3">
            <AppIcon name="rules" className="size-4 shrink-0 text-muted" />
            <span className="text-sm font-medium">处理策略</span>
            <span className="hidden text-xs text-muted sm:inline">消息聚合 → 重要性判断 → 通知推送</span>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <IntentChip intent={jev ? 'primary' : 'default'}>Jev 门控 {jev ? '启用' : '关闭'}</IntentChip>
            <IntentChip intent={llm ? 'primary' : 'default'}>LLM 蒸馏 {llm ? '启用' : '关闭'}</IntentChip>
            <Button variant="ghost" size="sm" onPress={() => navigate('/rules')}>调整规则</Button>
          </div>
        </Widget.Content>
      </Widget>

      <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1.5fr)_minmax(19rem,1fr)]">
        <EscalationPanel />
        <div className="grid min-w-0 gap-4">
          <BufferPanel />
          <MessageFeed />
        </div>
      </div>
    </Page>
  )
}
