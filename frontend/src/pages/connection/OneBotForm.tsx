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
  const dirty = JSON.stringify(ob) !== saved
  const set = (p: Partial<OneBotConfig>) => setOb((s) => ({ ...s, ...p }))

  const save = async () => {
    setSaving(true)
    try {
      const r = await api.onebot.save(ob)
      setOb(r)
      setSaved(JSON.stringify(r))
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
      <div className="flex flex-col gap-4">
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
      </div>
    </SectionCard>
  )
}
