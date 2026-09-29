import { useEffect, useState } from 'react'
import { Button } from '@heroui/react'
import { Segment } from '@heroui-pro/react'
import { SectionCard } from '../../components/ui/SectionCard'
import { TextSetting } from '../../components/ui/TextSetting'
import { NumberSetting } from '../../components/ui/NumberSetting'
import { Toggle } from '../../components/ui/Toggle'
import { IntentChip } from '../../components/ui/IntentChip'
import { api, ApiError } from '../../lib/api'
import { useApi } from '../../lib/useApi'
import { fmtDateTime } from '../../lib/time'
import type { BackupConfig } from '../../lib/types'

const defaults: BackupConfig = {
  enabled: false, provider: 'r2', cron: '0 3 * * *', endpoint: '', bucket: '',
  prefix: 'group-monitor', region: 'auto', accessKey: '', secretKey: '', timeoutSec: 60,
}

export function BackupForm({ initial }: { initial: BackupConfig }) {
  const [config, setConfig] = useState<BackupConfig>({ ...defaults, ...initial })
  const [saved, setSaved] = useState(() => JSON.stringify({ ...defaults, ...initial }))
  const [account, setAccount] = useState('')
  const [saving, setSaving] = useState(false)
  const [running, setRunning] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const status = useApi(api.backup.status, [])
  const dirty = JSON.stringify(config) !== saved
  const set = (patch: Partial<BackupConfig>) => {
    setConfig((value) => ({ ...value, ...patch }))
    setMessage('')
    setError('')
  }

  useEffect(() => {
    const timer = window.setInterval(status.reload, 10000)
    return () => window.clearInterval(timer)
  }, [status.reload])

  const save = async () => {
    setSaving(true)
    setError('')
    setMessage('')
    try {
      const result = await api.backup.save(config)
      setConfig(result)
      setSaved(JSON.stringify(result))
      setMessage('配置已保存')
      status.reload()
    } catch (e) {
      setError(e instanceof ApiError ? e.message : '保存失败，请重试')
    } finally {
      setSaving(false)
    }
  }
  const run = async () => {
    setRunning(true)
    setError('')
    setMessage('')
    try {
      await api.backup.run()
      setMessage('备份已完成')
    } catch (e) {
      setError(e instanceof ApiError ? e.message : '备份失败，请重试')
    } finally {
      setRunning(false)
      status.reload()
    }
  }

  return (
    <SectionCard title="云端备份" description="归档文件将上传到 S3 兼容存储；备份不包含 API 密钥或连接配置。">
      <div className="flex flex-col gap-6">
        <Toggle label="启用定期备份" description="默认使用服务器时区，也可在 Cron 前指定 CRON_TZ=Asia/Shanghai" isSelected={config.enabled} onChange={(enabled) => set({ enabled })} isDisabled={saving || running} />
        <fieldset disabled={saving || running} className="flex min-w-0 flex-col gap-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <span className="text-sm font-medium">存储服务</span>
            <Segment aria-label="存储服务" selectedKey={config.provider} onSelectionChange={(key) => set({ provider: key as 'r2' | 's3', region: key === 'r2' ? 'auto' : 'us-east-1' })}>
              <Segment.Item id="r2">Cloudflare R2</Segment.Item>
              <Segment.Item id="s3">S3 兼容</Segment.Item>
            </Segment>
          </div>
          {config.provider === 'r2' ? (
            <div className="flex flex-col gap-2 sm:flex-row sm:items-end">
              <TextSetting label="Cloudflare Account ID" description="可自动生成 R2 的连接地址" value={account} onChange={setAccount} placeholder="32 位 Account ID" className="flex-1" />
              <Button variant="secondary" isDisabled={!/^[a-f\d]{32}$/i.test(account.trim())} onPress={() => set({ endpoint: `https://${account.trim()}.r2.cloudflarestorage.com`, region: 'auto', cron: '0 3 * * *' })}>应用 R2 预设</Button>
            </div>
          ) : null}
          <TextSetting label="Endpoint" value={config.endpoint} onChange={(endpoint) => set({ endpoint })} type="url" placeholder={config.provider === 'r2' ? 'https://<account>.r2.cloudflarestorage.com' : 'https://s3.us-east-1.amazonaws.com'} />
          <div className="grid gap-4 sm:grid-cols-2">
            <TextSetting label="Bucket" value={config.bucket} onChange={(bucket) => set({ bucket })} placeholder="group-monitor-backup" />
            <TextSetting label="Region" value={config.region} onChange={(region) => set({ region })} placeholder={config.provider === 'r2' ? 'auto' : 'us-east-1'} />
            <TextSetting label="Access Key ID" value={config.accessKey} onChange={(accessKey) => set({ accessKey })} type="password" description="留空保留已保存的凭据" />
            <TextSetting label="Secret Access Key" value={config.secretKey} onChange={(secretKey) => set({ secretKey })} type="password" description="保存后不再回显" />
            <TextSetting label="文件前缀" value={config.prefix} onChange={(prefix) => set({ prefix })} placeholder="group-monitor" />
            <TextSetting label="Cron 周期" value={config.cron} onChange={(cron) => set({ cron })} placeholder="0 3 * * *" description="分 时 日 月 周；默认每天 03:00" />
          </div>
          <NumberSetting label="备份超时（秒）" value={config.timeoutSec} onChange={(timeoutSec) => set({ timeoutSec })} minValue={1} maxValue={120} className="max-w-xs" />
        </fieldset>
        <div className="flex flex-wrap items-center gap-3">
          <Button onPress={save} isPending={saving} isDisabled={!dirty || running}>保存更改</Button>
          <Button variant="secondary" onPress={run} isPending={running} isDisabled={dirty || saving || !config.enabled || status.data?.running}>立即备份</Button>
          {dirty ? <span className="text-xs text-muted">保存后可立即备份</span> : null}
        </div>
        <div aria-live="polite" className="flex flex-col gap-2">
          {error ? <p role="alert" className="text-sm text-danger">{error}</p> : null}
          {message ? <p className="text-sm text-success">{message}</p> : null}
          {status.error ? <p role="alert" className="text-sm text-danger">备份状态加载失败：{status.error}</p> : null}
          {status.data ? (
            <>
              <div className="flex flex-wrap items-center gap-3 text-xs text-muted">
                <IntentChip intent={status.data.running ? 'primary' : 'default'}>{status.data.running ? '备份中' : config.enabled ? '计划已启用' : '计划已关闭'}</IntentChip>
                <span>最近成功：{status.data.lastSuccess ? fmtDateTime(status.data.lastSuccess) : '尚无记录'}</span>
                <span className="tabular-nums">最近上传 {status.data.filesUploaded} 个文件</span>
                {status.data.nextRun ? <span>下次运行：{fmtDateTime(status.data.nextRun)}</span> : null}
              </div>
              {status.data.lastError ? <p className="text-sm text-danger">最近失败：{status.data.lastError}</p> : null}
            </>
          ) : null}
        </div>
      </div>
    </SectionCard>
  )
}
