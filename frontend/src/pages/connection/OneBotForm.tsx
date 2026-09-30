import { useState } from 'react'
import { Button } from '@heroui/react'
import { SectionCard } from '../../components/ui/SectionCard'
import { TextSetting } from '../../components/ui/TextSetting'
import { api } from '../../lib/api'
import type { OneBotConfig } from '../../lib/types'

// NapCat OneBot endpoint config. Seeds a local draft from `initial` and persists
// via api.onebot.save. The token is a secret — shown masked, not echoed back.
export function OneBotForm({ initial }: { initial: OneBotConfig }) {
  const [ob, setOb] = useState<OneBotConfig>(initial)
  const [saved, setSaved] = useState(() => JSON.stringify(initial))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const dirty = JSON.stringify(ob) !== saved
  const set = (p: Partial<OneBotConfig>) => {
    setOb((s) => ({ ...s, ...p }))
    setMessage('')
    setError('')
  }

  const save = async () => {
    setSaving(true)
    setError('')
    setMessage('')
    try {
      const r = await api.onebot.save(ob)
      setOb(r)
      setSaved(JSON.stringify(r))
      setMessage('连接配置已保存')
    } catch (e) {
      setError(e instanceof Error ? e.message : '保存失败，请重试')
    } finally {
      setSaving(false)
    }
  }

  return (
    <SectionCard
      title="OneBot 配置"
      description="连接 NapCat 的 HTTP / WebSocket 端点与令牌"
      actions={
        dirty ? (
          <Button onPress={save} isPending={saving}>
            保存更改
          </Button>
        ) : null
      }
    >
      <fieldset disabled={saving} className="grid min-w-0 gap-4 md:grid-cols-3">
        <TextSetting
          label="HTTP Base"
          description="OneBot HTTP API 地址"
          value={ob.httpBase}
          onChange={(v) => set({ httpBase: v })}
          placeholder="http://172.17.0.2:3100"
          type="url"
        />
        <TextSetting
          label="WebSocket URL"
          description="事件上报的正向 WS 地址"
          value={ob.wsUrl}
          onChange={(v) => set({ wsUrl: v })}
          placeholder="ws://172.17.0.2:3101"
          type="url"
        />
        <TextSetting
          label="Access Token"
          description="仅在此更新；保存后不再回显"
          value={ob.token}
          onChange={(v) => set({ token: v })}
          placeholder="access token"
          type="password"
        />
      </fieldset>
      {error && <p role="alert" className="mt-3 text-sm text-danger">{error}</p>}
      {message && <p role="status" className="mt-3 text-sm text-success">{message}</p>}
    </SectionCard>
  )
}
