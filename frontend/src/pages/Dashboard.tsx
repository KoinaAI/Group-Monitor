import { useState } from 'react'
import { Page } from '../components/Page'
import { PageHeader } from '../components/PageHeader'
import { Stat } from '../components/ui/Stat'
import { Toggle } from '../components/ui/Toggle'
import { IntentChip } from '../components/ui/IntentChip'
import { api } from '../lib/api'
import { useApi } from '../lib/useApi'
import { useLive } from '../lib/store'
import { BufferPanel, EscalationPanel, MessageFeed } from './dashboard/panels'

// Overview: merges the one-shot GET /api/status (extra fields like totalGroups /
// quietWindowSec) with the live SSE status, preferring live where both exist.
// The global enable switch uses optimistic local state since api.setEnabled does
// not necessarily echo a status event back over the stream.
export default function Dashboard() {
  const { data: sys } = useApi(api.status, [])
  const live = useLive((s) => s.status)
  const [enabledLocal, setEnabledLocal] = useState<boolean | null>(null)
  const [busy, setBusy] = useState(false)

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
    try {
      await api.setEnabled(v)
    } catch {
      setEnabledLocal(!v)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Page>
      <PageHeader
        title="总览"
        description="实时运行状态与最新升级事件"
        actions={
          <Toggle
            label={enabled ? '运行中' : '已暂停'}
            isSelected={enabled}
            onChange={toggle}
            isDisabled={busy}
          />
        }
      />
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Stat
          label="运行状态"
          value={enabled ? '运行中' : '已暂停'}
          icon={enabled ? 'running' : 'pause'}
          tone={enabled ? 'success' : 'default'}
        />
        <Stat label="监听群组" value={`${watched}/${totalGroups}`} icon="groups" tone="accent" />
        <Stat label="推送主人" value={masters} icon="masters" tone="accent" />
        <Stat
          label="静默窗口"
          value={`${quiet}s`}
          icon="clock"
          tone="default"
          hint={connected ? 'NapCat 已连接' : 'NapCat 未连接'}
        />
      </div>

      <div className="mt-4 flex flex-wrap items-center gap-2">
        <IntentChip intent={connected ? 'success' : 'default'}>
          {connected ? 'NapCat 已连接' : 'NapCat 未连接'}
        </IntentChip>
        <IntentChip intent={llm ? 'primary' : 'default'}>
          LLM 蒸馏 {llm ? '启用' : '关闭'}
        </IntentChip>
        <IntentChip intent={jev ? 'primary' : 'default'}>
          Jev 门控 {jev ? '启用' : '关闭'}
        </IntentChip>
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <MessageFeed />
        </div>
        <div className="flex flex-col gap-4">
          <BufferPanel />
          <EscalationPanel />
        </div>
      </div>
    </Page>
  )
}
