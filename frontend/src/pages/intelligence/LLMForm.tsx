import { useRef, useState } from 'react'
import { Button } from '@heroui/react'
import { SectionCard } from '../../components/ui/SectionCard'
import { EnableSetting } from './EnableSetting'
import { TextSetting } from '../../components/ui/TextSetting'
import { NumberSetting } from '../../components/ui/NumberSetting'
import { IntentChip } from '../../components/ui/IntentChip'
import { api, ApiError } from '../../lib/api'
import { urgencyIntent, urgencyLabel } from '../../lib/labels'
import type { LLMConfig, LLMTestResponse } from '../../lib/types'

// LLM distillation config + live test. Seeds a local draft from `initial`,
// persists via api.llm.save, and previews api.llm.test output inline.
export function LLMForm({ initial }: { initial: LLMConfig }) {
  const [llm, setLlm] = useState<LLMConfig>(initial)
  const [saved, setSaved] = useState(() => JSON.stringify(initial))
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [res, setRes] = useState<LLMTestResponse | null>(null)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const pending = useRef(false)
  const busy = saving || testing
  const dirty = JSON.stringify(llm) !== saved
  const set = (p: Partial<LLMConfig>) => {
    setLlm((s) => ({ ...s, ...p }))
    setError('')
    setMessage('')
    setRes(null)
  }

  const save = async () => {
    if (pending.current || !dirty) return
    pending.current = true
    setSaving(true)
    setError('')
    setMessage('')
    try {
      const r = await api.llm.save(llm)
      setLlm(r)
      setSaved(JSON.stringify(r))
      setMessage('LLM 配置已保存')
    } catch (e) {
      setError(e instanceof ApiError ? e.message : '保存失败，请重试')
    } finally {
      pending.current = false
      setSaving(false)
    }
  }
  const test = async () => {
    if (pending.current) return
    pending.current = true
    setTesting(true)
    setRes(null)
    setError('')
    try {
      setRes(await api.llm.test(llm))
    } catch (e) {
      setRes({ ok: false, error: e instanceof ApiError ? e.message : '测试失败' })
    } finally {
      pending.current = false
      setTesting(false)
    }
  }

  const r = res?.result
  return (
    <SectionCard
      title="LLM 蒸馏"
      description="将聚合消息压缩为结构化摘要后再推送"
      className="min-w-0"
      footer={
        <div className="flex w-full flex-wrap items-center gap-2">
          <Button size="sm" variant="secondary" onPress={test} isPending={testing} isDisabled={busy}>
            测试
          </Button>
          {dirty ? (
            <Button size="sm" onPress={save} isPending={saving} isDisabled={busy}>
              保存更改
            </Button>
          ) : null}
          <span role="status" className="text-xs text-success">{message}</span>
        </div>
      }
    >
      <div className="flex flex-col gap-4">
        <EnableSetting
          label="启用 LLM 蒸馏"
          description="关闭后仅推送紧急原文；正式通知仍可经 Jev 判断后归档"
          isSelected={llm.enabled}
          onChange={(v) => set({ enabled: v })}
          isDisabled={busy}
        />
        <fieldset disabled={busy} className="grid min-w-0 gap-3">
          <div className="grid gap-4 sm:grid-cols-2">
            <TextSetting
              label="Base URL"
              value={llm.baseUrl}
              onChange={(v) => set({ baseUrl: v })}
              placeholder="https://api.example.com/v1"
              type="url"
            />
            <TextSetting
              label="模型"
              value={llm.model}
              onChange={(v) => set({ model: v })}
              placeholder="claude-sonnet-5"
            />
          </div>
          <TextSetting
            label="API Key"
            description="仅在此更新；保存后不再回显"
            value={llm.apiKey}
            onChange={(v) => set({ apiKey: v })}
            placeholder="sk-…"
            type="password"
          />
          <div className="grid gap-4 sm:grid-cols-3">
            <NumberSetting
              label="超时（秒）"
              value={llm.timeoutSec}
              onChange={(v) => set({ timeoutSec: v })}
              minValue={1}
              maxValue={120}
            />
            <NumberSetting
              label="最大 Token"
              value={llm.maxTokens}
              onChange={(v) => set({ maxTokens: v })}
              minValue={64}
              maxValue={8192}
              step={64}
            />
            <NumberSetting
              label="温度"
              value={llm.temperature}
              onChange={(v) => set({ temperature: v })}
              minValue={0}
              maxValue={2}
              step={0.1}
              formatOptions={{ minimumFractionDigits: 1, maximumFractionDigits: 1 }}
            />
          </div>
        </fieldset>
        {error ? <p role="alert" className="text-sm text-danger">{error}</p> : null}
        {res ? (
          <div aria-live="polite" className="rounded-xl bg-surface-secondary p-3">
            {!res.ok ? (
              <p role="alert" className="text-xs text-danger">{res.error || '测试失败'}</p>
            ) : (
              <div className="flex flex-col gap-2">
                <div className="flex flex-wrap items-center gap-2">
                  <IntentChip intent={r ? urgencyIntent(r.level) : 'default'}>
                    {r ? urgencyLabel(r.level) : '无结果'}
                  </IntentChip>
                  {r ? (
                    <IntentChip intent={r.useful ? 'success' : 'secondary'}>
                      {r.useful ? '有用' : '无用'}
                    </IntentChip>
                  ) : null}
                  {r?.title ? (
                    <span className="text-sm font-medium text-foreground">{r.title}</span>
                  ) : null}
                </div>
                {r?.summary ? <p className="text-sm text-muted">{r.summary}</p> : null}
                {res.preview ? (
                  <pre className="max-h-48 overflow-y-auto whitespace-pre-wrap break-words text-xs text-muted">
                    {res.preview}
                  </pre>
                ) : null}
              </div>
            )}
          </div>
        ) : null}
      </div>
    </SectionCard>
  )
}
