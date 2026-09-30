import { useRef, useState } from 'react'
import { Button } from '@heroui/react'
import { NativeSelect } from '@heroui-pro/react'
import { SectionCard } from '../../components/ui/SectionCard'
import { EnableSetting } from './EnableSetting'
import { TextSetting } from '../../components/ui/TextSetting'
import { NumberSetting } from '../../components/ui/NumberSetting'
import { api, ApiError } from '../../lib/api'
import type { DocumentConfig } from '../../lib/types'

const defaults: DocumentConfig = {
  enabled: false, mode: 'agent', baseUrl: 'https://mineru.net/api/v1/agent', apiKey: '',
  timeoutSec: 120, maxFileMB: 20, maxTextChars: 12000,
}

export function DocumentForm({ initial }: { initial: DocumentConfig }) {
  const [config, setConfig] = useState<DocumentConfig>({ ...defaults, ...initial })
  const [saved, setSaved] = useState(() => JSON.stringify({ ...defaults, ...initial }))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const pending = useRef(false)
  const dirty = JSON.stringify(config) !== saved
  const set = (patch: Partial<DocumentConfig>) => {
    setConfig((value) => ({ ...value, ...patch }))
    setMessage('')
    setError('')
  }
  const save = async () => {
    if (pending.current || !dirty) return
    pending.current = true
    setSaving(true)
    setError('')
    setMessage('')
    try {
      const result = await api.documents.save(config)
      setConfig(result)
      setSaved(JSON.stringify(result))
      setMessage('附件阅读配置已保存')
    } catch (e) {
      setError(e instanceof ApiError ? e.message : '保存失败，请重试')
    } finally {
      pending.current = false
      setSaving(false)
    }
  }
  return (
    <SectionCard
      title="附件阅读"
      description="提取群文件正文，让文件中的通知也能参与判断与摘要。"
      footer={
        <div className="flex flex-wrap items-center gap-3">
          <Button size="sm" onPress={save} isPending={saving} isDisabled={!dirty || saving}>保存更改</Button>
          <span role="status" className="text-xs text-success">{message}</span>
        </div>
      }
    >
      <div className="grid items-start gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,2fr)] lg:gap-8">
        <EnableSetting label="读取通知附件" description="所有文件类型统一交给 MinerU 解析，正文会参与判断与摘要。" isSelected={config.enabled} onChange={(enabled) => set({ enabled })} isDisabled={saving} />
        <fieldset disabled={saving} className="grid min-w-0 gap-3">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <p className="text-sm font-medium">MinerU 接入方式</p>
              <p className="text-xs text-muted">Agent API 免费且默认启用；按量 API 需要 Token。</p>
            </div>
            <NativeSelect variant="secondary" className="w-full sm:w-56">
              <NativeSelect.Trigger value={config.mode} onChange={(event) => {
                const mode = event.target.value as DocumentConfig['mode']
                const firstParty = config.baseUrl.includes('mineru.net')
                set({ mode, ...(firstParty ? { baseUrl: mode === 'agent' ? 'https://mineru.net/api/v1/agent' : 'https://mineru.net/api/v4' } : {}) })
              }}>
                <NativeSelect.Option value="agent">Agent API（免费）</NativeSelect.Option>
                <NativeSelect.Option value="api">按量 API（需要 Key）</NativeSelect.Option>
              </NativeSelect.Trigger>
            </NativeSelect>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <TextSetting label="MinerU API 地址" value={config.baseUrl} onChange={(baseUrl) => set({ baseUrl })} type="url" placeholder={config.mode === 'agent' ? 'https://mineru.net/api/v1/agent' : 'https://mineru.net/api/v4'} />
            {config.mode === 'api' ? <TextSetting label="MinerU API Key" value={config.apiKey} onChange={(apiKey) => set({ apiKey })} type="password" description="留空保留现有密钥；保存后不再回显" /> : <div className="flex items-end pb-1 text-xs text-muted">Agent API 不需要 Key，按 IP 限流。</div>}
          </div>
          <div className="grid gap-4 sm:grid-cols-3">
            <NumberSetting label="解析超时（秒）" value={config.timeoutSec} onChange={(timeoutSec) => set({ timeoutSec })} minValue={1} maxValue={600} />
            <NumberSetting label="单文件上限（MB）" value={config.maxFileMB} onChange={(maxFileMB) => set({ maxFileMB })} minValue={1} maxValue={100} />
            <NumberSetting label="正文字符上限" value={config.maxTextChars} onChange={(maxTextChars) => set({ maxTextChars })} minValue={256} maxValue={32000} step={1000} />
          </div>
        </fieldset>
      </div>
      {error ? <p role="alert" className="mt-3 text-sm text-danger">{error}</p> : null}
    </SectionCard>
  )
}
