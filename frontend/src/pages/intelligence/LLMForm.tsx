import { useState } from 'react'
import { Button } from '@heroui/react'
import { SectionCard } from '../../components/ui/SectionCard'
import { Toggle } from '../../components/ui/Toggle'
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
  const dirty = JSON.stringify(llm) !== saved
  const set = (p: Partial<LLMConfig>) => setLlm((s) => ({ ...s, ...p }))

  const save = async () => {
    setSaving(true)
    try {
      const r = await api.llm.save(llm)
      setLlm(r)
      setSaved(JSON.stringify(r))
    } finally {
      setSaving(false)
    }
  }
  const test = async () => {
    setTesting(true)
    setRes(null)
    try {
      setRes(await api.llm.test(llm))
    } catch (e) {
      setRes({ ok: false, error: e instanceof ApiError ? e.message : '测试失败' })
    } finally {
      setTesting(false)
    }
  }

  const r = res?.result
  return (
    <SectionCard
      title="LLM 蒸馏"
      description="将聚合消息压缩为结构化摘要后再推送"
      actions={
        <>
          <Button variant="secondary" onPress={test} isPending={testing}>
            测试
          </Button>
          {dirty ? (
            <Button onPress={save} isPending={saving}>
              保存更改
            </Button>
          ) : null}
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <Toggle
          label="启用 LLM 蒸馏"
          description="关闭后直接转发原始聚合内容"
          isSelected={llm.enabled}
          onChange={(v) => set({ enabled: v })}
        />
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
        {res ? (
          <div className="rounded-lg bg-surface-secondary p-3">
            {!res.ok ? (
              <p className="text-xs text-danger">{res.error || '测试失败'}</p>
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
                  <pre className="whitespace-pre-wrap rounded-md bg-default p-2 text-xs text-muted">
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
