import { useState } from 'react'
import { Button } from '@heroui/react'
import { SectionCard } from '../../components/ui/SectionCard'
import { Toggle } from '../../components/ui/Toggle'
import { TextSetting } from '../../components/ui/TextSetting'
import { NumberSetting } from '../../components/ui/NumberSetting'
import { IntentChip } from '../../components/ui/IntentChip'
import { api, ApiError } from '../../lib/api'
import type { JevConfig, JevTestResponse } from '../../lib/types'

// Jev intent-gate config + test. The gate is fail-open: on error the pipeline
// still escalates. Test previews per-sample scores against the threshold.
export function JevForm({ initial }: { initial: JevConfig }) {
  const [jev, setJev] = useState<JevConfig>(initial)
  const [saved, setSaved] = useState(() => JSON.stringify(initial))
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [res, setRes] = useState<JevTestResponse | null>(null)
  const dirty = JSON.stringify(jev) !== saved
  const set = (p: Partial<JevConfig>) => setJev((s) => ({ ...s, ...p }))

  const save = async () => {
    setSaving(true)
    try {
      const r = await api.jev.save(jev)
      setJev(r)
      setSaved(JSON.stringify(r))
    } finally {
      setSaving(false)
    }
  }
  const test = async () => {
    setTesting(true)
    setRes(null)
    try {
      setRes(await api.jev.test(jev))
    } catch (e) {
      setRes({ ok: false, error: e instanceof ApiError ? e.message : '测试失败' })
    } finally {
      setTesting(false)
    }
  }

  return (
    <SectionCard
      title="Jev 意图门控"
      description="在蒸馏前判定聚合内容是否值得升级（失败时放行）"
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
          label="启用意图门控"
          description="关闭后所有聚合内容都进入蒸馏"
          isSelected={jev.enabled}
          onChange={(v) => set({ enabled: v })}
        />
        <div className="grid gap-4 sm:grid-cols-2">
          <TextSetting
            label="Base URL"
            value={jev.baseUrl}
            onChange={(v) => set({ baseUrl: v })}
            placeholder="https://api.example.com"
            type="url"
          />
          <TextSetting
            label="模型"
            value={jev.model}
            onChange={(v) => set({ model: v })}
            placeholder="jev-intent"
          />
        </div>
        <TextSetting
          label="API Key"
          description="仅在此更新；保存后不再回显"
          value={jev.apiKey}
          onChange={(v) => set({ apiKey: v })}
          placeholder="sk-…"
          type="password"
        />
        <div className="grid gap-4 sm:grid-cols-3">
          <NumberSetting
            label="阈值"
            description="noul 高于阈值才升级"
            value={jev.threshold}
            onChange={(v) => set({ threshold: v })}
            minValue={0}
            maxValue={1}
            step={0.05}
            formatOptions={{ minimumFractionDigits: 2, maximumFractionDigits: 2 }}
          />
          <NumberSetting
            label="上下文条数"
            value={jev.contextN}
            onChange={(v) => set({ contextN: v })}
            minValue={0}
            maxValue={50}
          />
          <NumberSetting
            label="超时（秒）"
            value={jev.timeoutSec}
            onChange={(v) => set({ timeoutSec: v })}
            minValue={1}
            maxValue={120}
          />
        </div>
        {res ? (
          <div className="rounded-lg bg-surface-secondary p-3">
            {!res.ok ? (
              <p className="text-xs text-danger">{res.error || '测试失败'}</p>
            ) : (
              <ul className="flex flex-col gap-2">
                {(res.samples ?? []).map((s, i) => (
                  <li key={i} className="flex items-center gap-2 text-xs">
                    <IntentChip intent={s.important ? 'warning' : 'default'}>
                      {s.important ? '升级' : '忽略'}
                    </IntentChip>
                    <span className="w-12 shrink-0 tabular-nums text-muted">
                      {s.noul.toFixed(2)}
                    </span>
                    <span className="truncate text-muted">{s.label || s.text}</span>
                  </li>
                ))}
              </ul>
            )}
          </div>
        ) : null}
      </div>
    </SectionCard>
  )
}
