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
  const initialConfig = { ...defaults, ...initial, mode: initial.mode ?? (initial.baseUrl?.includes('/api/v4') ? 'api' : defaults.mode) } as DocumentConfig
  const [config, setConfig] = useState<DocumentConfig>(initialConfig)
  const [saved, setSaved] = useState(() => JSON.stringify(initialConfig))
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
      description="提取群文件正文，参与意图判断与通知摘要"
      actions={<Button size="sm" onPress={save} isPending={saving} isDisabled={!dirty || saving}>保存更改</Button>}
      footer={
        message ? <span role="status" className="text-xs text-success">{message}</span> : undefined
      }
    >
      <fieldset disabled={saving} className="grid min-w-0 gap-3">
        <div className="grid items-start gap-3 lg:grid-cols-[minmax(0,1fr)_minmax(14rem,0.7fr)]">
          <EnableSetting label="读取通知附件" description="通过 MinerU 解析所有类型的通知附件。" isSelected={config.enabled} onChange={(enabled) => set({ enabled })} isDisabled={saving} />
          <div className="grid min-w-0 gap-1.5">
            <label htmlFor="mineru-mode" className="text-sm font-medium">MinerU 接入方式</label>
            <NativeSelect variant="secondary" fullWidth>
              <NativeSelect.Trigger id="mineru-mode" value={config.mode} onChange={(event) => {
                const mode = event.target.value as DocumentConfig['mode']
                const firstParty = config.baseUrl.includes('mineru.net')
                set({ mode, ...(firstParty ? { baseUrl: mode === 'agent' ? 'https://mineru.net/api/v1/agent' : 'https://mineru.net/api/v4' } : {}) })
              }}>
                <NativeSelect.Option value="agent">Agent API（免费）</NativeSelect.Option>
                <NativeSelect.Option value="api">按量 API（需要 Key）</NativeSelect.Option>
              </NativeSelect.Trigger>
            </NativeSelect>
          </div>
        </div>
          <div className={`grid gap-3 ${config.mode === 'api' ? 'sm:grid-cols-2' : ''}`}>
            <TextSetting label="MinerU API 地址" value={config.baseUrl} onChange={(baseUrl) => set({ baseUrl })} type="url" placeholder={config.mode === 'agent' ? 'https://mineru.net/api/v1/agent' : 'https://mineru.net/api/v4'} />
            {config.mode === 'api' ? <TextSetting label="MinerU API Key" value={config.apiKey} onChange={(apiKey) => set({ apiKey })} type="password" description="留空保留现有密钥；保存后不再回显" /> : null}
          </div>
          <div className="flex flex-wrap items-start justify-between gap-2">
            <details className="min-w-0 flex-1 text-sm">
              <summary className="cursor-pointer text-muted">解析限制 <span className="ml-2 text-xs tabular-nums">{config.maxFileMB} MB · {config.maxTextChars.toLocaleString()} 字符</span></summary>
              <div className="mt-3 grid gap-3 sm:grid-cols-3">
            <NumberSetting label="解析超时（秒）" value={config.timeoutSec} onChange={(timeoutSec) => set({ timeoutSec })} minValue={1} maxValue={600} />
            <NumberSetting label="单文件上限（MB）" value={config.maxFileMB} onChange={(maxFileMB) => set({ maxFileMB })} minValue={1} maxValue={100} />
            <NumberSetting label="正文字符上限" value={config.maxTextChars} onChange={(maxTextChars) => set({ maxTextChars })} minValue={256} maxValue={32000} step={1000} />
              </div>
            </details>
            {config.mode === 'agent' ? <p className="text-xs leading-5 text-muted">免费，无需 Key，按 IP 限流</p> : null}
          </div>
        </fieldset>
      {error ? <p role="alert" className="mt-3 text-sm text-danger">{error}</p> : null}
    </SectionCard>
  )
}
