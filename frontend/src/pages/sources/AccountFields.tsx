import { TextSetting } from '../../components/ui/TextSetting'
import { Toggle } from '../../components/ui/Toggle'
import type { SourceAccount } from '../../lib/types'

export function AccountFields({ account, onChange }: { account: SourceAccount; onChange: (value: SourceAccount) => void }) {
  const set = (value: Partial<SourceAccount>) => onChange({ ...account, ...value })
  return <div className="grid min-w-0 items-start gap-3 sm:grid-cols-2">
    <div className="flex flex-wrap items-end gap-3">
      <TextSetting className="min-w-40 flex-1" label="账号名称" value={account.name} onChange={(name) => set({ name })} placeholder="例如：校园账号、工作账号" />
      <div className="pb-2"><Toggle size="sm" label="启用账号" isSelected={account.enabled} onChange={(enabled) => set({ enabled })} /></div>
    </div>
    <TextSetting label="访问令牌" type="password" value={account.onebot.token} onChange={(token) => set({ onebot: { ...account.onebot, token } })} description="已保存的令牌不回显；留空保留原值。" />
      <TextSetting label="OneBot HTTP 地址" value={account.onebot.httpBase} onChange={(httpBase) => set({ onebot: { ...account.onebot, httpBase } })} placeholder="http://localhost:3000" />
      <TextSetting label="OneBot WebSocket 地址" value={account.onebot.wsUrl} onChange={(wsUrl) => set({ onebot: { ...account.onebot, wsUrl } })} placeholder="ws://localhost:3001" />
  </div>
}
