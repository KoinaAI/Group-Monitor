import { useState } from 'react'
import { Button } from '@heroui/react'
import { SectionCard } from '../../components/ui/SectionCard'
import { Toggle } from '../../components/ui/Toggle'
import { TextSetting } from '../../components/ui/TextSetting'
import { NumberSetting } from '../../components/ui/NumberSetting'
import { api, ApiError } from '../../lib/api'
import type { DocumentConfig } from '../../lib/types'

const defaults: DocumentConfig = {
  enabled: false, baseUrl: 'https://mineru.net/api/v4', apiKey: '',
  timeoutSec: 120, maxFileMB: 20, maxTextChars: 12000,
}

export function DocumentForm({ initial }: { initial: DocumentConfig }) {
  const [config, setConfig] = useState<DocumentConfig>({ ...defaults, ...initial })
  const [saved, setSaved] = useState(() => JSON.stringify({ ...defaults, ...initial }))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const dirty = JSON.stringify(config) !== saved
  const set = (patch: Partial<DocumentConfig>) => {
    setConfig((value) => ({ ...value, ...patch }))
    setMessage('')
    setError('')
  }
  const save = async () => {
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
      setSaving(false)
    }
  }
  return (
    <SectionCard title="附件阅读" description="提取群文件正文，让文件中的通知也能参与判断与摘要。">
      <div className="flex flex-col gap-5">
        <Toggle label="读取通知附件" description="DOCX、TXT、Markdown 在本地解析；PDF、DOC 等文件将上传到配置的 MinerU 服务。" isSelected={config.enabled} onChange={(enabled) => set({ enabled })} isDisabled={saving} />
        <fieldset disabled={saving} className="flex min-w-0 flex-col gap-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <TextSetting label="MinerU API 地址" value={config.baseUrl} onChange={(baseUrl) => set({ baseUrl })} type="url" placeholder="https://mineru.net/api/v4" />
            <TextSetting label="MinerU API Key" value={config.apiKey} onChange={(apiKey) => set({ apiKey })} type="password" description="留空保留现有密钥；保存后不再回显" />
          </div>
          <div className="grid gap-4 sm:grid-cols-3">
            <NumberSetting label="解析超时（秒）" value={config.timeoutSec} onChange={(timeoutSec) => set({ timeoutSec })} minValue={1} maxValue={600} />
            <NumberSetting label="单文件上限（MB）" value={config.maxFileMB} onChange={(maxFileMB) => set({ maxFileMB })} minValue={1} maxValue={100} />
            <NumberSetting label="正文字符上限" value={config.maxTextChars} onChange={(maxTextChars) => set({ maxTextChars })} minValue={256} maxValue={32000} step={1000} />
          </div>
        </fieldset>
        <div className="flex flex-wrap items-center gap-3">
          <Button onPress={save} isPending={saving} isDisabled={!dirty}>保存更改</Button>
          <span aria-live="polite" className="text-sm text-success">{message}</span>
        </div>
        {error ? <p role="alert" className="text-sm text-danger">{error}</p> : null}
      </div>
    </SectionCard>
  )
}
