import { useRef, useState } from 'react'
import { Button } from '@heroui/react'
import { SectionCard } from '../../components/ui/SectionCard'
import { EnableSetting } from './EnableSetting'
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
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const pending = useRef(false)
  const busy = saving || testing
  const dirty = JSON.stringify(jev) !== saved
  const set = (p: Partial<JevConfig>) => {
    setJev((s) => ({ ...s, ...p }))
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
      const r = await api.jev.save(jev)
      setJev(r)
      setSaved(JSON.stringify(r))
      setMessage('Jev 配置已保存')
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
      setRes(await api.jev.test(jev))
    } catch (e) {
      setRes({ ok: false, error: e instanceof ApiError ? e.message : '测试失败' })
    } finally {
      pending.current = false
      setTesting(false)
    }
  }

  return (
    <SectionCard
      title="Jev 意图门控"
      description="逐条预筛消息，并在归档前确认正式通知；判定失败时不写入归档"
      className="min-w-0"
      actions={
        <Button size="sm" variant="secondary" onPress={test} isPending={testing} isDisabled={busy}>
          测试连接
        </Button>
      }
      footer={
        <div className="flex w-full flex-wrap items-center gap-2">
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
          label="启用意图门控"
          description="关闭或未配置密钥时可继续推送，但暂停通知归档"
          isSelected={jev.enabled}
          onChange={(v) => set({ enabled: v })}
          isDisabled={busy}
        />
        <fieldset disabled={busy} className="grid min-w-0 gap-3">
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
              maxValue={20}
            />
            <NumberSetting
              label="超时（秒）"
              value={jev.timeoutSec}
              onChange={(v) => set({ timeoutSec: v })}
              minValue={1}
              maxValue={120}
            />
          </div>
        </fieldset>
        {error ? <p role="alert" className="text-sm text-danger">{error}</p> : null}
        {res ? (
          <div aria-live="polite" className="rounded-xl bg-surface-secondary p-3">
            {!res.ok ? (
              <p role="alert" className="text-xs text-danger">{res.error || '测试失败'}</p>
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
