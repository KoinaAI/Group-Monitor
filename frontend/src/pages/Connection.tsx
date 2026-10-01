import { Page } from '../components/Page'
import { PageHeader } from '../components/PageHeader'
import { StatusDot } from '../components/ui/StatusDot'
import { InlineError } from '../components/ui/States'
import { Loader } from '../components/Loader'
import { api } from '../lib/api'
import { useApi } from '../lib/useApi'
import { useLive } from '../lib/store'
import { OneBotForm } from './connection/OneBotForm'

// NapCat OneBot connection: a live status readout (SSE status, falling back to
// the one-shot /api/status) plus the endpoint config form.
export default function Connection() {
  const { data: config, error, loading, reload } = useApi(api.config, [])
  const { data: sys } = useApi(api.status, [])
  const live = useLive((s) => s.status)
  const connected = live?.onebotConnected ?? sys?.onebotConnected ?? false
  const selfId = live?.selfId ?? sys?.selfId
  const nickname = sys?.account?.nickname

  return (
    <Page>
      <PageHeader title="连接" description="NapCat OneBot 连接配置" />
      <div className="mb-3 flex flex-wrap items-center gap-2 text-xs text-muted" aria-label="连接状态">
        <StatusDot tone={connected ? 'success' : 'danger'} pulse={connected} />
        <span className="font-medium text-foreground">
          {connected ? '已连接' : '未连接'}
        </span>
        {connected && (nickname || selfId) ? (
          <span className="tabular-nums">
            {nickname ? `${nickname} · ` : ''}
            {selfId ?? ''}
          </span>
        ) : (
          <span>检查 NapCat 是否在线以及端点配置是否正确</span>
        )}
      </div>
      {loading || !config ? (
        error ? (
          <InlineError message={error} onRetry={reload} />
        ) : (
          <Loader label="正在加载配置…" />
        )
      ) : (
        <OneBotForm initial={config.onebot} />
      )}
    </Page>
  )
}
