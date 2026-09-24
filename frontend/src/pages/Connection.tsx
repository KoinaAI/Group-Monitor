import { Page } from '../components/Page'
import { PageHeader } from '../components/PageHeader'
import { SectionCard } from '../components/ui/SectionCard'
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
      <div className="flex flex-col gap-4">
        <SectionCard title="连接状态" description="与 NapCat 的实时连接情况">
          <div className="flex items-center gap-3">
            <StatusDot tone={connected ? 'success' : 'danger'} pulse={connected} />
            <div className="min-w-0">
              <p className="text-sm font-medium text-foreground">
                {connected ? '已连接' : '未连接'}
              </p>
              {connected && (nickname || selfId) ? (
                <p className="text-xs text-muted tabular-nums">
                  {nickname ? `${nickname} · ` : ''}
                  {selfId ?? ''}
                </p>
              ) : (
                <p className="text-xs text-muted">检查 NapCat 是否在线以及端点配置是否正确</p>
              )}
            </div>
          </div>
        </SectionCard>
        {loading || !config ? (
          error ? (
            <InlineError message={error} onRetry={reload} />
          ) : (
            <Loader label="正在加载配置…" />
          )
        ) : (
          <OneBotForm initial={config.onebot} />
        )}
      </div>
    </Page>
  )
}
